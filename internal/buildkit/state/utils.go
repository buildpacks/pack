package state

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/containerd/platforms"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerfile/shell"
	"github.com/moby/buildkit/identity"
	"github.com/moby/buildkit/util/system"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

func pathRelativeToWorkingDir(s llb.State, p string, platform ocispecs.Platform) (string, error) {
	dir, err := s.GetDir(context.TODO(), llb.Platform(platform))
	if err != nil {
		return "", err
	}

	if len(p) == 0 {
		return dir, nil
	}
	p, err = system.CheckSystemDriveAndRemoveDriveLetter(p, platform.OS)
	if err != nil {
		return "", errors.Wrap(err, "removing drive letter")
	}

	if system.IsAbs(p, platform.OS) {
		return system.NormalizePath("/", p, platform.OS, true)
	}
	return system.NormalizePath(dir, p, platform.OS, true)
}

func withShell(img v1.ConfigFile, args []string) []string {
	var shell []string
	if len(img.Config.Shell) > 0 {
		shell = append([]string{}, img.Config.Shell...)
	} else {
		shell = defaultShell(img.OS)
	}
	return append(shell, strings.Join(args, " "))
}

func defaultShell(os string) []string {
	if os == "windows" {
		return []string{"cmd", "/S", "/C"}
	}
	return []string{"/bin/sh", "-c"}
}

// Adds History for the [State]
func commitToHistory(cfg *v1.ConfigFile, msg string, withLayer bool, st *llb.State, tm time.Time, lifecycleVersion string) error {
	if st != nil {
		msg += " # buildpacks.io"
	}

	cfg.History = append(cfg.History, v1.History{
		CreatedBy:  msg,
		Comment:    fmt.Sprintf("lifecycle %s", lifecycleVersion),
		EmptyLayer: !withLayer,
		Created:    v1.Time{Time: tm},
	})
	return nil
}

func addEnv(env []string, k, v string) []string {
	gotOne := false
	for i, envVar := range env {
		key, _ := parseKeyValue(envVar)
		if shell.EqualEnvKeys(key, k) {
			env[i] = k + "=" + v
			gotOne = true
			break
		}
	}
	if !gotOne {
		env = append(env, k+"="+v)
	}
	return env
}

func parseKeyValue(env string) (string, string) {
	parts := strings.SplitN(env, "=", 2)
	v := ""
	if len(parts) > 1 {
		v = parts[1]
	}

	return parts[0], v
}

// Fork from: https://github.com/moby/buildkit/blob/9c8832ff46cc805f37537f0540aa4ac99be568d3/frontend/dockerfile/dockerfile2llb/convert.go#L1149
func dispatchCopy(s *State, srcPaths []string, destPath string, cfg CopyOptions) error {
	dest, err := pathRelativeToWorkingDir(s.state, destPath, *s.platform)
	if err != nil {
		return err
	}

	var copyOpt []llb.CopyOption

	if cfg.chown != "" {
		copyOpt = append(copyOpt, llb.WithUser(cfg.chown))
	}

	if len(cfg.exclude) > 0 {
		// In upcoming buildkit versions buildkit provides llb.WithExcludePatterns
		// replace this functionality with that
		copyOpt = append(copyOpt, &llb.CopyInfo{ExcludePatterns: cfg.exclude})
	}

	var mode *os.FileMode
	if cfg.chmod != "" {
		p, err := strconv.ParseUint(cfg.chmod, 8, 32)
		if err == nil {
			perm := os.FileMode(p)
			mode = &perm
		}
	}

	commitMessage := bytes.NewBufferString("")
	if cfg.AddCommand {
		commitMessage.WriteString("ADD")
	} else {
		commitMessage.WriteString("COPY")
	}

	if cfg.parents {
		commitMessage.WriteString(" " + "--parents")
	}
	if cfg.chown != "" {
		commitMessage.WriteString(" " + "--chown=" + cfg.chown)
	}
	if cfg.chmod != "" {
		commitMessage.WriteString(" " + "--chmod=" + cfg.chmod)
	}

	platform := cfg.targetPlatform
	if s.platform != nil {
		platform = *s.platform
	}

	env, err := s.state.Env(context.TODO())
	if err != nil {
		return err
	}

	var cmd = "COPY "
	if cfg.AddCommand {
		cmd = "ADD "
	}
	name := uppercaseCmd(cmd + strings.Join(env, " "))
	pgName := prefixCommand(s, name, s.multiArch, &platform, env)

	var a *llb.FileAction

	for _, src := range srcPaths {
		commitMessage.WriteString(" " + src)
		// Add git and http/https support when needed
		var patterns []string
		if cfg.parents {
			// detect optional pivot point
			parent, pattern, ok := strings.Cut(src, "/./")
			if !ok {
				pattern = src
				src = "/"
			} else {
				src = parent
			}

			pattern, err = system.NormalizePath("/", pattern, s.platform.OS, false)
			if err != nil {
				return errors.Wrap(err, "removing drive letter")
			}

			patterns = []string{strings.TrimPrefix(pattern, "/")}
		}

		src, err = system.NormalizePath("/", src, s.platform.OS, false)
		if err != nil {
			return errors.Wrap(err, "removing drive letter")
		}

		opts := append([]llb.CopyOption{&llb.CopyInfo{
			Mode:                mode,
			FollowSymlinks:      true,
			CopyDirContentsOnly: true,
			IncludePatterns:     patterns,
			AttemptUnpack:       cfg.AddCommand,
			CreateDestPath:      true,
			AllowWildcard:       true,
			AllowEmptyWildcard:  true,
		}}, copyOpt...)

		if a == nil {
			a = llb.Copy(cfg.source, src, dest, opts...)
		} else {
			a = a.Copy(cfg.source, src, dest, opts...)
		}
	}

	commitMessage.WriteString(" " + destPath)

	fileOpt := []llb.ConstraintsOpt{
		llb.WithCustomName(pgName),
	}
	if cfg.ignoreCache {
		fileOpt = append(fileOpt, llb.IgnoreCache)
	}

	if cfg.link && cfg.chmod == "" {
		pgID := identity.NewID()
		s.cmdIndex-- // prefixCommand increases it
		pgName := prefixCommand(s, name, s.multiArch, &platform, env)

		copyOpts := []llb.ConstraintsOpt{
			llb.Platform(*s.platform),
		}
		copyOpts = append(copyOpts, fileOpt...)
		copyOpts = append(copyOpts, llb.ProgressGroup(pgID, pgName, true))

		mergeOpts := append([]llb.ConstraintsOpt{}, fileOpt...)
		s.cmdIndex--
		mergeOpts = append(mergeOpts, llb.ProgressGroup(pgID, pgName, false), llb.WithCustomName(prefixCommand(s, "LINK "+name, s.multiArch, &platform, env)))

		s.state = s.state.WithOutput(llb.Merge([]llb.State{s.state, llb.Scratch().File(a, copyOpts...)}, mergeOpts...).Output())
	} else {
		s.state = s.state.File(a, fileOpt...)
	}

	return commitToHistory(s.config, commitMessage.String(), true, &s.state, time.Now(), s.version)
}

func WithInternalName(name string) llb.ConstraintsOpt {
	return llb.WithCustomName("[internal] " + name)
}

func uppercaseCmd(str string) string {
	p := strings.SplitN(str, " ", 2)
	p[0] = strings.ToUpper(p[0])
	return strings.Join(p, " ")
}

// fork from: https://github.com/moby/buildkit/blob/9c8832ff46cc805f37537f0540aa4ac99be568d3/frontend/dockerfile/dockerfile2llb/convert.go#L1826C1-L1840C2
func prefixCommand(s *State, str string, prefixPlatform bool, platform *ocispecs.Platform, env []string) string {
	if s.cmdTotal == 0 {
		return str
	}
	out := "["
	if prefixPlatform && platform != nil {
		out += platforms.Format(*platform) + formatTargetPlatform(*platform, platformFromEnv(env)) + " "
	}
	if s.stageName != "" {
		out += s.stageName + " "
	}
	s.cmdIndex++
	out += fmt.Sprintf("%*d/%d] ", int(1+math.Log10(float64(s.cmdTotal))), s.cmdIndex, s.cmdTotal)
	return out + str
}

// fork from: https://github.com/moby/buildkit/blob/9c8832ff46cc805f37537f0540aa4ac99be568d3/frontend/dockerfile/dockerfile2llb/convert.go#L1843
func formatTargetPlatform(base ocispecs.Platform, target *ocispecs.Platform) string {
	if target == nil {
		return ""
	}
	if target.OS == "" {
		target.OS = base.OS
	}
	if target.Architecture == "" {
		target.Architecture = base.Architecture
	}
	p := platforms.Normalize(*target)

	if p.OS == base.OS && p.Architecture != base.Architecture {
		archVariant := p.Architecture
		if p.Variant != "" {
			archVariant += "/" + p.Variant
		}
		return "->" + archVariant
	}
	if p.OS != base.OS {
		return "->" + platforms.Format(p)
	}
	return ""
}

// fork from: https://github.com/moby/buildkit/blob/9c8832ff46cc805f37537f0540aa4ac99be568d3/frontend/dockerfile/dockerfile2llb/convert.go#L1869
func platformFromEnv(env []string) *ocispecs.Platform {
	var p ocispecs.Platform
	var set bool
	for _, v := range env {
		parts := strings.SplitN(v, "=", 2)
		switch parts[0] {
		case "TARGETPLATFORM":
			p, err := platforms.Parse(parts[1])
			if err != nil {
				continue
			}
			return &p
		case "TARGETOS":
			p.OS = parts[1]
			set = true
		case "TARGETARCH":
			p.Architecture = parts[1]
			set = true
		case "TARGETVARIANT":
			p.Variant = parts[1]
			set = true
		}
	}
	if !set {
		return nil
	}
	return &p
}
