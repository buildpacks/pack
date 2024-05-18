package state

import (
	"fmt"
	"io/fs"
	"runtime"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/entitlements"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/buildpacks/pack/internal/buildkit/packerfile/options"
)

// A key value pair with optional delim `=`. Used for Buildtime only environmental variables.
func (s *State) AddArg(args ...string) *State {
	commitStrs := make([]string, 0, len(args))
	for _, arg := range args {
		k, v, _ := strings.Cut(arg, "=")
		commitStr := k
		if v != "" {
			commitStr += "=" + v
		}
		commitStrs = append(commitStrs, commitStr)
		s.state = s.state.AddEnv(k, v) // we are adding env with we are not defining this env in configFile, so it is only available at buildtime!
	}
	s.options.BuildArgs = append(s.options.BuildArgs, args...)
	commitToHistory(s.config, "ARG "+strings.Join(commitStrs, " "), false, nil, time.Now(), s.version)
	return s
}

func (s *State) AddVolume(volumes ...string) *State {
	if s.config.Config.Volumes == nil {
		s.config.Config.Volumes = map[string]struct{}{}
	}
	for _, v := range volumes {
		if v == "" {
			panic("VOLUME specified can not be an empty string")
		}
		s.config.Config.Volumes[v] = struct{}{}
	}
	s.options.Volumes = append(s.options.Volumes, volumes...)
	commitToHistory(s.config, fmt.Sprintf("VOLUME %v", volumes), false, nil, time.Now(), s.version)
	return s
}

func (s *State) AddEnv(key, value string) *State {
	s.state = s.state.AddEnv(key, value)
	s.config.Config.Env = addEnv(s.config.Config.Env, key, value)
	s.options.Envs = append(s.options.Envs, fmt.Sprintf("%s=%s", key, value))
	commitToHistory(s.config, fmt.Sprintf("ENV %s=%s", key, value), false, nil, time.Now(), s.version)
	return s
}

// Set Entrypoint of CMD.
// when length of args equals 1 use shell form
// else Exec form
func (s *State) Entrypoint(args ...string) *State {
	useShell := len(args) == 1
	if useShell {
		args = strings.Split(args[0], " ")
		args = withShell(*s.config, args)
	}
	if !s.options.cmdSet {
		s.config.Config.Cmd = nil
	}
	s.config.Config.Entrypoint = args
	commitToHistory(s.config, fmt.Sprintf("ENTRYPOINT %q", args), false, nil, time.Now(), s.version)
	return s
}

func (s *State) Network(mode string) *State {
	switch mode {
	case "host", "HOST":
		s.state = s.state.Network(pb.NetMode_HOST)
		s.options.entitlement = entitlements.EntitlementNetworkHost
	case "none", "NONE":
		s.state = s.state.Network(pb.NetMode_NONE)
	case "", "default", "UNSET":
		s.state = s.state.Network(pb.NetMode_UNSET)
	}
	return s
}

func (s *State) Mkdir(path string, mode fs.FileMode, ops ...llb.MkdirOption) *State {
	s.state = s.state.File(llb.Mkdir(path, mode, ops...))
	return s
}

func (s *State) MkFile(path string, mode fs.FileMode, data []byte, ops ...llb.MkfileOption) *State {
	s.state = s.state.File(llb.Mkfile(path, mode, data, ops...))
	return s
}

func (s *State) Add(src []string, dest string, opt options.ADD) *State {
	err := dispatchCopy(s, src, dest, CopyOptions{
		exclude:    opt.Exclude,
		AddCommand: true,
		chown:      opt.Chown,
		chmod:      opt.Chmod,
		link:       opt.Link,
		dest: dest,
	})

	if err != nil {
		panic(err)
	}

	return s
}

func (s *State) User(user string) *State {
	s.state = s.state.User(user)
	s.config.Config.User = user
	s.options.User = user
	commitToHistory(s.config, fmt.Sprintf("USER %v", user), false, nil, time.Now(), s.version)
	return s
}

func (s *State) Cmd(cmd ...string) *State {
	if len(cmd) == 1 {
		cmd = withShell(*s.config, strings.Split(cmd[0], " "))
	}

	s.config.Config.Cmd = cmd
	s.config.Config.ArgsEscaped = true
	s.options.cmdSet = true
	commitToHistory(s.config, fmt.Sprintf("CMD %q", cmd), false, nil, time.Now(), s.version)
	return s
}

func (s *State) Run(cmd []string, execState func(state llb.ExecState) llb.State) *State {
	exec := s.state.Run(WithInternalName("running cmd"), llb.Args(cmd), llb.Network(llb.NetModeHost))
	s.state = exec.Root()
	s.state = execState(exec)
	return s
}

// shall we allow buildpack authors like paketo, heroku, kpack, dokku, GCP etc...,
// to modify [State] by passing pointer?
func (s *State) State() *llb.State {
	return &s.state
}

// shall we allow buildpack authors like paketo, heroku, kpack, dokku, GCP etc...,
// to modify [State] by passing pointer?
func (s *State) ConfigFile() *v1.ConfigFile {
	return s.config
}

func (s *State) Platform() *ocispecs.Platform {
	if s.platform != nil {
		return s.platform
	}

	return &ocispecs.Platform{
		OS: runtime.GOOS,
		Architecture: runtime.GOARCH,
	}
}

func (s *State) Options() Options {
	return s.options
}
