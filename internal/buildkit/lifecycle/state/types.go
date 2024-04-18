package state

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
)

type State struct {
	*llb.State
	*v1.ConfigFile
	version   string
	buildArgs map[string]string
}
