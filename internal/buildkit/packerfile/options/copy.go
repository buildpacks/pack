package options

import "github.com/moby/buildkit/client/llb"

// NOTE: All the Options provided here might not work!

// COPY Options used by [Dockerfile COPY] instruction.
type COPYOptions struct {
	// By default, the COPY instruction copies files from the build context.
	// The COPY --from flag lets you copy files from an image, a build stage, or a named context instead.
	From llb.State

	// The --chown and --chmod features are only supported on Dockerfiles
	// used to build Linux containers, and doesn't work on Windows containers.
	Chown, Chmod string

	// When --link is used your source files are copied into an empty destination directory.
	// That directory is turned into a layer that is linked on top of your previous state.
	Link string

	// The --parents flag preserves parent directories for src entries. This flag defaults to false.
	//
	// This behavior is similar to the Linux cp utility's --parents or rsync --relative flag.
	Parents bool

	// The --exclude flag lets you specify a path expression for files to be excluded.
	Exclude []string
}
