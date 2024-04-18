package state

import (
	"encoding/json"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/util/system"
)

func NewState(os string) (_ *State, err error) {
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

	state, err := llb.Scratch().WithImageConfig(cfgBytes)
	return &State{
		State:      &state,
		ConfigFile: config,
	}, err
}
