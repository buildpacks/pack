package state

import (
	"fmt"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
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

// Prepends v1.ConfigFile.Config.Cmd with given [flags]
func (s State) WithFlags(flags ...string) State {
	s.ConfigFile.Config.Cmd = append(flags, s.ConfigFile.Config.Cmd...)
	return s
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
