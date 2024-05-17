package builder

import (
	"github.com/buildpacks/pack/internal/buildkit/packerfile"
	"github.com/moby/buildkit/client"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

type Builder[T any] struct { // let's make the [builder] private so that no one annoyingly changes builder's embaded [state.State]
	ref string // name of the builder
	packerfile.Packerfile[T] // state of the builder
	client *client.Client // client to solve the state
	mounts []string // mounts to mount to the container
	entrypoint []string // entrypoint of the container
	cmd []cmd
	envs []string
	user string
	attachStdin, attachStdout, attachStderr bool
	platforms []ocispecs.Platform
	prevImage packerfile.Packerfile[*T]
	workdir string
}

type Stringifier interface {
	String() string
}

type cmd struct {
	os string
	path string
	wd string
	Stringifier
}