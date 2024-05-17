package state

import (
	"encoding/json"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/util/system"
)

// NewState creates an empty [State] with system specific default PATH env set
func Scratch(os string) (_ *State, err error) {
	config := &v1.ConfigFile{
		RootFS: v1.RootFS{
			Type: "layers",
		},
		Config: v1.Config{
			WorkingDir: "/",
			Env:        []string{"PATH=" + system.DefaultPathEnv(os)},
		},
	}

	cfgBytes, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	state, err := llb.Scratch().Network(llb.NetModeHost).WithImageConfig(cfgBytes)
	return &State{
		state:  state,
		config: config,
	}, err
}

func Remote(ref string, opts ...llb.ImageOption) *State {
	state := llb.Image(ref, opts...).Network(llb.NetModeHost)
	return &State{
		state: state,
		config: &v1.ConfigFile{}, // update with the current [llb.state]
	}
}

func Local(name string, opts ...llb.LocalOption) *State {
	return &State{
		state: llb.Local(name, opts...).Network(llb.NetModeHost),
		config: &v1.ConfigFile{}, // update with the current [llb.state]
	}
}

func OCILayout(ref string, opts ...llb.OCILayoutOption) *State {
	return &State{
		state: llb.OCILayout(ref, opts...).Network(llb.NetModeHost),
		config: &v1.ConfigFile{}, // update with the current [llb.state]
	}
}