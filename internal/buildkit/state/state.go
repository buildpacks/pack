package state

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerui"
	"github.com/moby/buildkit/identity"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/gitutil"
	"github.com/moby/buildkit/util/system"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/buildkit/packerfile/options"
)

// Similar to Dockerfile [VOLUME]
func (s State) AddVolumes(volumes ...string) State {
	for _, volume := range volumes {
		s.Config.Volumes[volume] = struct{}{}
	}
	// TODO: add current lifecycle version
	commitToHistory(s.ConfigFile, fmt.Sprintf("VOLUME %v", volumes), false, nil, time.Now(), s.version)
	return s
}

// Similar to Dockerfile [ENV]
func (s State) AddEnv(key, value string) State {
	prevState := s.State.AddEnv(key, value)
	commitToHistory(s.ConfigFile, fmt.Sprintf("ENV %s=%s", key, value), false, nil, time.Now(), s.version)
	s.State = &prevState
	return s
}

// similar to Dockerfile [ARG]
func (s State) AddArgs(args ...string) State {
	prevState := s.State
	commitMsg := make([]string, len(args))
	for _, arg := range args {
		result := strings.Split(arg, "=")
		switch len(result) {
		case 0:
		case 1:
			ps := prevState.AddEnv(result[0], "")
			prevState = &ps
			s.buildArgs[result[0]] = ""
			commitMsg = append(commitMsg, result...)
		default:
			ps := prevState.AddEnv(result[0], result[1])
			prevState = &ps
			s.buildArgs[result[0]] = result[1]
			commitMsg = append(commitMsg, fmt.Sprintf("%s=%s", result[0], result[1]))
		}
	}
	s.State = prevState
	s.ConfigFile.Config.Cmd = append(s.ConfigFile.Config.Cmd, args...)
	commitToHistory(s.ConfigFile, strings.Join(commitMsg, " "), false, &s, time.Now(), s.version)
	return s
}

// Set the network of the Container.
// Supported Values ['host', 'none', 'default']
func (s State) WithNetwork(network string) (_ State, err error) {
	var op llb.StateOption
	switch network {
	case "host":
		op = llb.Network(pb.NetMode_HOST)
	case "none":
		op = llb.Network(pb.NetMode_NONE)
	case "":
		fallthrough
	case "default":
		op = llb.Network(pb.NetMode_UNSET)
	default:
		return s, fmt.Errorf("unknown network: %s. supported ['none', 'host', 'default']", network)
	}
	prevState := op(*s.State)
	s.State = &prevState
	return s, nil
}

func (s State) AddCMD(args []string, prependShell bool) State {
	if prependShell {
		args = withShell(*s.ConfigFile, args)
	}
	s.ConfigFile.Config.Cmd = args
	s.ConfigFile.Config.ArgsEscaped = true //nolint:staticcheck // ignore SA1019: field is deprecated in OCI Image spec, but used for backward-compatibility with Docker image spec.
	s.cmdSet = true
	commitToHistory(s.ConfigFile, fmt.Sprintf("CMD %q", args), false, nil, time.Now(), s.version)
	return s
}

func (s State) Copy(cfg options.COPY) (err error) {
	dest, err := pathRelativeToWorkingDir(d.state, cfg.params.DestPath, *d.platform)
	if err != nil {
		return err
	}

	if cfg.params.DestPath == "." || cfg.params.DestPath == "" || cfg.params.DestPath[len(cfg.params.DestPath)-1] == filepath.Separator {
		dest += string(filepath.Separator)
	}

	var copyOpt []llb.CopyOption

	if cfg.chown != "" {
		copyOpt = append(copyOpt, llb.WithUser(cfg.chown))
	}

	var mode *os.FileMode
	if cfg.chmod != "" {
		p, err := strconv.ParseUint(cfg.chmod, 8, 32)
		if err == nil {
			perm := os.FileMode(p)
			mode = &perm
		}
	}

	if cfg.checksum != "" {
		if !cfg.isAddCommand {
			return errors.New("checksum can't be specified for COPY")
		}
		if len(cfg.params.SourcePaths) != 1 {
			return errors.New("checksum can't be specified for multiple sources")
		}
		if !isHTTPSource(cfg.params.SourcePaths[0]) {
			return errors.New("checksum can't be specified for non-HTTP sources")
		}
	}

	commitMessage := bytes.NewBufferString("")
	if cfg.isAddCommand {
		commitMessage.WriteString("ADD")
	} else {
		commitMessage.WriteString("COPY")
	}

	var a *llb.FileAction

	for _, src := range cfg.params.SourcePaths {
		commitMessage.WriteString(" " + src)
		gitRef, gitRefErr := gitutil.ParseGitRef(src)
		if gitRefErr == nil && !gitRef.IndistinguishableFromLocal {
			if !cfg.isAddCommand {
				return errors.New("source can't be a git ref for COPY")
			}
			// TODO: print a warning (not an error) if gitRef.UnencryptedTCP is true
			commit := gitRef.Commit
			if gitRef.SubDir != "" {
				commit += ":" + gitRef.SubDir
			}
			var gitOptions []llb.GitOption
			if cfg.keepGitDir {
				gitOptions = append(gitOptions, llb.KeepGitDir())
			}
			st := llb.Git(gitRef.Remote, commit, gitOptions...)
			opts := append([]llb.CopyOption{&llb.CopyInfo{
				Mode:           mode,
				CreateDestPath: true,
			}}, copyOpt...)
			if a == nil {
				a = llb.Copy(st, "/", dest, opts...)
			} else {
				a = a.Copy(st, "/", dest, opts...)
			}
		} else if isHTTPSource(src) {
			if !cfg.isAddCommand {
				return errors.New("source can't be a URL for COPY")
			}

			// Resources from remote URLs are not decompressed.
			// https://docs.docker.com/engine/reference/builder/#add
			//
			// Note: mixing up remote archives and local archives in a single ADD instruction
			// would result in undefined behavior: https://github.com/moby/buildkit/pull/387#discussion_r189494717
			u, err := url.Parse(src)
			f := "__unnamed__"
			if err == nil {
				if base := path.Base(u.Path); base != "." && base != "/" {
					f = base
				}
			}

			st := llb.HTTP(src, llb.Filename(f), llb.Checksum(cfg.checksum), dfCmd(cfg.params))

			opts := append([]llb.CopyOption{&llb.CopyInfo{
				Mode:           mode,
				CreateDestPath: true,
			}}, copyOpt...)

			if a == nil {
				a = llb.Copy(st, f, dest, opts...)
			} else {
				a = a.Copy(st, f, dest, opts...)
			}
		} else {
			src, err = system.NormalizePath("/", src, d.platform.OS, false)
			if err != nil {
				return errors.Wrap(err, "removing drive letter")
			}

			opts := append([]llb.CopyOption{&llb.CopyInfo{
				Mode:                mode,
				FollowSymlinks:      true,
				CopyDirContentsOnly: true,
				AttemptUnpack:       cfg.isAddCommand,
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
	}

	for _, src := range cfg.params.SourceContents {
		commitMessage.WriteString(" <<" + src.Path)

		data := src.Data
		f, err := system.CheckSystemDriveAndRemoveDriveLetter(src.Path, d.platform.OS)
		if err != nil {
			return errors.Wrap(err, "removing drive letter")
		}
		st := llb.Scratch().File(
			llb.Mkfile(f, 0644, []byte(data)),
			dockerui.WithInternalName("preparing inline document"),
			llb.Platform(*d.platform),
		)

		opts := append([]llb.CopyOption{&llb.CopyInfo{
			Mode:           mode,
			CreateDestPath: true,
		}}, copyOpt...)

		if a == nil {
			a = llb.Copy(st, system.ToSlash(f, d.platform.OS), dest, opts...)
		} else {
			a = a.Copy(st, filepath.ToSlash(f), dest, opts...)
		}
	}

	commitMessage.WriteString(" " + cfg.params.DestPath)

	platform := cfg.opt.targetPlatform
	if d.platform != nil {
		platform = *d.platform
	}

	env, err := d.state.Env(context.TODO())
	if err != nil {
		return err
	}

	name := uppercaseCmd(processCmdEnv(cfg.opt.shlex, cfg.cmdToPrint.String(), env))
	fileOpt := []llb.ConstraintsOpt{
		llb.WithCustomName(prefixCommand(d, name, d.prefixPlatform, &platform, env)),
		location(cfg.opt.sourceMap, cfg.location),
	}
	if d.ignoreCache {
		fileOpt = append(fileOpt, llb.IgnoreCache)
	}

	// cfg.opt.llbCaps can be nil in unit tests
	if cfg.opt.llbCaps != nil && cfg.opt.llbCaps.Supports(pb.CapMergeOp) == nil && cfg.link && cfg.chmod == "" {
		pgID := identity.NewID()
		d.cmdIndex-- // prefixCommand increases it
		pgName := prefixCommand(d, name, d.prefixPlatform, &platform, env)

		copyOpts := []llb.ConstraintsOpt{
			llb.Platform(*d.platform),
		}
		copy(copyOpts, fileOpt)
		copyOpts = append(copyOpts, llb.ProgressGroup(pgID, pgName, true))

		var mergeOpts []llb.ConstraintsOpt
		copy(mergeOpts, fileOpt)
		d.cmdIndex--
		mergeOpts = append(mergeOpts, llb.ProgressGroup(pgID, pgName, false), llb.WithCustomName(prefixCommand(d, "LINK "+name, d.prefixPlatform, &platform, env)))

		d.state = d.state.WithOutput(llb.Merge([]llb.State{d.state, llb.Scratch().File(a, copyOpts...)}, mergeOpts...).Output())
	} else {
		d.state = d.state.File(a, fileOpt...)
	}

	return commitToHistory(&d.image, commitMessage.String(), true, &d.state, d.epoch)
}

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
func commitToHistory(cfg *v1.ConfigFile, msg string, withLayer bool, st *State, tm time.Time, lifecycleVersion string) error {
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
