package options

import (
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// NOTE: All the Options provided here might not work!

// The FROM instruction initializes a new build stage and sets the base image for subsequent instructions.
// As such, a valid Dockerfile must start with a FROM instruction.
type FROMOptions struct {
	Platform v1.Platform
	Ref      name.Reference
}
