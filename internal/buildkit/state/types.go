package state

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

type State struct {
	state               llb.State
	config              *v1.ConfigFile
	version             string
	buildArgs           map[string]string
	cmdSet, ignoreCache bool
	platform            *ocispecs.Platform
	cmdIndex, cmdTotal  int
	multiArch, shelx    bool
	// From ... AS stageName
	stageName string
}

type CopyOptions struct {
	dest           string
	targetPlatform ocispecs.Platform
	exclude        []string
	source         llb.State
	AddCommand     bool
	chmod, chown   string
	link, parents  bool
	ignoreCache    bool
}
