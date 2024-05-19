package builder

import (
	"strings"

	"github.com/buildpacks/pack/internal/buildkit/packerfile"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

// FIXME: any better way to isolate both Cmd and Cmdf?

// Dockerfile: CMD
func (b *Builder[any]) Cmd(c ...string) *Builder[any] {
	for _, v := range c {
		b.cmd = append(b.cmd, CMD(v))
	}
	b.Packerfile.Cmd(c...)
	return b
}

// Dockerfile: CMD
func (b *Builder[any]) Cmdf(cmd ...cmd) *Builder[any] {
	b.cmd = append(b.cmd, cmd...)
	// b.Packerfile.Cmd(cmd...) // TODO: support cmd
	return b
}
// Dockerfile: ENTRYPOINT
func (b *Builder[any]) Entrypoint(ep ...string) *Builder[any] {
	b.entrypoint = append(b.entrypoint, ep...)
	b.Packerfile.Entrypoint(ep...)
	return b
}

// the name of the exported image
func (b *Builder[any]) Name(name string) *Builder[any] {
	b.ref = name
	return b
}

// Dockerfile: USER
func (b *Builder[any]) User(user string) *Builder[any] {
	b.user = user
	b.Packerfile.User(user)
	return b
}

// Dockerfile: ENV
func (b *Builder[any]) AddEnv(env ...string) *Builder[any] {
	b.envs = append(b.envs, env...)
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		b.Packerfile.AddEnv(k, v)
	}
	return b
}

// Attach STDIN
func (b *Builder[any]) Stdin() *Builder[any] {
	b.attachStdin = true
	return b
}

// Attach STDOUT
func (b *Builder[any]) Stdout() *Builder[any] {
	b.attachStdout = true
	return b
}

// Attach STDERR
func (b *Builder[any]) Stderr() *Builder[any] {
	b.attachStderr = true
	return b
}

// list of platforms builder targeting
func (b *Builder[any]) Platforms() []ocispecs.Platform {
	return b.platforms
}

// list of platforms builder targeting
func (b *Builder[any]) AddPlatform(platform ...ocispecs.Platform) *Builder[any] {
	b.platforms = append(b.platforms, platform...)
	return b
}

// list of platforms builder targeting
func (b *Builder[any]) SetPlatform(platform ...ocispecs.Platform) *Builder[any] {
	b.platforms = platform
	return b
}

// Set the base image
func (b *Builder[T]) PrevImage(prevImage packerfile.Packerfile[*T]) *Builder[T] {
	b.prevImage = prevImage
	return b
}

func (b *Builder[any]) Workdir(dir string) *Builder[any] {
	b.workdir = dir
	b.Packerfile.State().Dir(dir)
	return b
}

func (b *Builder[any]) AddVolume(volumes ...string) *Builder[any] {
	b.Packerfile.AddVolume(volumes...)
	b.mounts = append(b.mounts, volumes...)
	return b
}
