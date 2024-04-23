package state

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

type State struct {
	*llb.State
	*v1.ConfigFile
	version             string
	buildArgs           map[string]string
	cmdSet, ignoreCache bool
	platform            *ocispecs.Platform
	cmdIndex            int
}
