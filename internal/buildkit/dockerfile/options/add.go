package options

import v1 "github.com/google/go-containerregistry/pkg/v1"

// NOTE: All the Options provided here might not work!

// AddOptions are a set of Options supported by [Dockerfile] ADD command
type ADDOptions struct {
	// When <src> is the HTTP or SSH address of a remote Git repository,
	// BuildKit adds the contents of the Git repository to the image excluding the .git directory by default.
	//
	// The --keep-git-dir=true flag lets you preserve the .git directory.
	KeepGitDir bool

	// The --checksum flag lets you verify the checksum of a remote resource.
	CheckSum v1.Hash

	// The --chown and --chmod features are only supported on Dockerfiles
	// used to build Linux containers, and doesn't work on Windows containers.
	Chown, Chmod string

	// When --link is used your source files are copied into an empty destination directory.
	// That directory is turned into a layer that is linked on top of your previous state.
	Link bool

	// The --exclude flag lets you specify a path expression for files to be excluded.
	Exclude string
}
