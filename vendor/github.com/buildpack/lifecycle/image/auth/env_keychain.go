package auth

import (
	"encoding/json"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/cmd"
)

type EnvKeychain struct{}

func DefaultEnvKeychain() authn.Keychain {
	return authn.NewMultiKeychain(&EnvKeychain{}, authn.DefaultKeychain)
}

func (EnvKeychain) Resolve(registry name.Registry) (authn.Authenticator, error) {
	env := os.Getenv(cmd.EnvRegistryAuth)
	if env == "" {
		return authn.Anonymous, nil
	}
	authMap := map[string]string{}
	err := json.Unmarshal([]byte(env), &authMap)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s value", cmd.EnvRegistryAuth)
	}
	auth, ok := authMap[registry.Name()]
	if ok {
		return &providedAuth{auth: auth}, nil
	}

	return authn.Anonymous, nil
}

type providedAuth struct {
	auth string
}

func (p *providedAuth) Authorization() (string, error) {
	return p.auth, nil
}

func BuildEnvVar(keychain authn.Keychain, images ...string) (string, error) {
	registryAuths := map[string]string{}

	for _, image := range images {
		reference, authenticator, err := ReferenceForRepoName(keychain, image)
		if err != nil {
			return "", nil
		}
		if authenticator == authn.Anonymous {
			continue
		}

		registryAuths[reference.Context().Registry.Name()], err = authenticator.Authorization()
		if err != nil {
			return "", nil
		}
	}
	authData, err := json.Marshal(registryAuths)
	if err != nil {
		return "", err
	}
	return string(authData), nil
}
