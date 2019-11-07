package image

import (
	"github.com/buildpack/imgutil"
	"github.com/buildpack/imgutil/local"
	"github.com/buildpack/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
)

type DefaultImageFactory struct {
	dockerClient *client.Client
	keychain     authn.Keychain
}

func NewFactory(dockerClient *client.Client, keychain authn.Keychain) *DefaultImageFactory {
	return &DefaultImageFactory{
		dockerClient: dockerClient,
		keychain:     keychain,
	}
}

func (f *DefaultImageFactory) NewImage(repoName string, daemon bool) (imgutil.Image, error) {
	if daemon {
		return local.NewImage(repoName, f.dockerClient)
	}

	return remote.NewImage(repoName, f.keychain)
}
