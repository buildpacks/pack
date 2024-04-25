package state

import (
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"

	"github.com/buildpacks/pack/internal/buildkit/packerfile"
	"github.com/buildpacks/pack/internal/buildkit/packerfile/options"
)

var _ packerfile.Packerfile[State] = (*State)(nil)

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
	commitToHistory(s.config, fmt.Sprintf("VOLUME %v", volumes), false, nil, time.Now(), s.version)
	return s
}

func (s *State) AddEnv(key, value string) *State {
	s.state = s.state.AddEnv(key, value)
	s.config.Config.Env = addEnv(s.config.Config.Env, key, value)
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
	s.config.Config.Entrypoint = args
	commitToHistory(s.config, fmt.Sprintf("ENTRYPOINT %q", args), false, nil, time.Now(), s.version)
	return s
}

func (s *State) Network(mode string) *State {
	switch mode {
	case "host":
		s.state = s.state.Network(pb.NetMode_HOST)
	case "none":
		s.state = s.state.Network(pb.NetMode_NONE)
	default:
		s.state = s.state.Network(pb.NetMode_UNSET)
	}
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
	})

	if err != nil {
		panic(err)
	}

	return s
}

func (s *State) Root() *State {
	// TODO: Add root access
	return s
}
