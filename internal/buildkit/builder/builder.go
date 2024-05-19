package builder

import (
	"fmt"
	"strings"

	"github.com/buildpacks/pack/internal/buildkit/packerfile"
	"github.com/buildpacks/pack/internal/buildkit/packerfile/options"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

// FIXME: T better way to isolate both Cmd and Cmdf?

// Dockerfile: CMD
func (b *Builder[T]) Cmd(c ...string) *Builder[T] {
	for _, v := range c {
		b.cmd = append(b.cmd, CMD(v))
	}
	b.Packerfile.Cmd(c...)
	return b
}

// Dockerfile: CMD
func (b *Builder[T]) Cmdf(cmd ...cmd) *Builder[T] {
	b.cmd = append(b.cmd, cmd...)
	// b.Packerfile.Cmd(cmd...) // TODO: support cmd
	return b
}
// Dockerfile: ENTRYPOINT
func (b *Builder[T]) Entrypoint(ep ...string) *Builder[T] {
	b.entrypoint = append(b.entrypoint, ep...)
	b.Packerfile.Entrypoint(ep...)
	return b
}

// the name of the exported image
func (b *Builder[T]) Name(name string) *Builder[T] {
	b.ref = name
	return b
}

// Dockerfile: USER
func (b *Builder[T]) User(user string) *Builder[T] {
	b.user = user
	b.Packerfile.User(user)
	return b
}

// Dockerfile: ENV
func (b *Builder[T]) AddEnv(env ...string) *Builder[T] {
	b.envs = append(b.envs, env...)
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		b.Packerfile.AddEnv(k, v)
	}
	return b
}

// Attach STDIN
func (b *Builder[T]) Stdin() *Builder[T] {
	b.attachStdin = true
	return b
}

// Attach STDOUT
func (b *Builder[T]) Stdout() *Builder[T] {
	b.attachStdout = true
	return b
}

// Attach STDERR
func (b *Builder[T]) Stderr() *Builder[T] {
	b.attachStderr = true
	return b
}

// list of platforms builder targeting
func (b *Builder[T]) Platforms() []ocispecs.Platform {
	return b.platforms
}

// list of platforms builder targeting
func (b *Builder[T]) AddPlatform(platform ...ocispecs.Platform) *Builder[T] {
	b.platforms = append(b.platforms, platform...)
	return b
}

// list of platforms builder targeting
func (b *Builder[T]) SetPlatform(platform ...ocispecs.Platform) *Builder[T] {
	b.platforms = platform
	return b
}

// Set the base image
func (b *Builder[T]) PrevImage(prevImage packerfile.Packerfile[*T]) *Builder[T] {
	b.prevImage = prevImage
	return b
}

func (b *Builder[T]) Workdir(dir string) *Builder[T] {
	b.workdir = dir
	b.Packerfile.State().Dir(dir)
	return b
}

func (b *Builder[T]) AddVolume(volumes ...string) *Builder[T] {
	b.Packerfile.AddVolume(volumes...)
	b.mounts = append(b.mounts, volumes...)
	return b
}

func (b *Builder[T]) AppSource(src, dest string) *Builder[T] {
	b.Packerfile.Add([]string{src}, dest, options.ADD{
		Link: true,
		Chown: fmt.Sprintf("%s:%s", b.uid, b.gid),
		Chmod: fmt.Sprintf("%s:%s", b.uid, b.gid),
	})
	return b
}

func (b *Builder[T]) UID(uid string) *Builder[T] {
	b.uid = uid
	return b
}

func (b *Builder[T]) GID(gid string) *Builder[T] {
	b.gid = gid
	return b
}
