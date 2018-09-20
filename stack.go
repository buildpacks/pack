package pack

import (
	"github.com/buildpack/pack/docker"
	"github.com/google/go-containerregistry/pkg/v1"
)

type Stack struct {
	BuildImage v1.Image
}

const defaultBuildRepo = "packs/build"

func DefaultStack(noPull bool) (Stack, error) {
	if !noPull {
		docker, err := docker.New()
		if err != nil {
			return Stack{}, err
		}
		err = docker.PullImage(defaultBuildRepo)
		if err != nil {
			return Stack{}, err
		}
	}

	buildImage, err := readImage(defaultBuildRepo, true)
	if err != nil {
		return Stack{}, err
	}
	return Stack{
		BuildImage: buildImage,
	}, nil
}
