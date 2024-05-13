package state

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/util/entitlements"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

type State struct {
	state               llb.State
	// the configFile used for storing run Options that cannot be set on [llb.State]
	config *v1.ConfigFile
	// which one should we consider
	// version of buildpacks api?
	// version of lifecycle?
	version string
	// platform of the current state
	platform *ocispecs.Platform
	options Options
}

type Options struct {
	multiArch bool
	cmdIndex int64
	cmdTotal int64
	stageName string
	cmdSet bool
	entitlement entitlements.Entitlement
	BuildArgs []string
	Envs []string
	User string
	Workdir string
	Volumes []string
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
