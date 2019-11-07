package auth

import (
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpack/lifecycle/cmd"
)

type LegacyEnvKeychain struct{}

func (LegacyEnvKeychain) Resolve(name.Registry) (authn.Authenticator, error) {
	env := os.Getenv(cmd.EnvLegacyRegistryAuth)
	if env == "" {
		return authn.Anonymous, nil
	}
	return &providedAuth{auth: env}, nil
}
