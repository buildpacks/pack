package pack

import (
	"context"
	"io"

	"github.com/buildpack/lifecycle/image"
)

//go:generate mockgen -package mocks -destination mocks/fetcher.go github.com/buildpack/pack Fetcher
type Fetcher interface {
	FetchUpdatedLocalImage(context.Context, string, io.Writer) (image.Image, error)
	FetchLocalImage(string) (image.Image, error)
	FetchRemoteImage(string) (image.Image, error)
}

type ImageFetcher struct {
	Docker  Docker
	Factory ImageFactory
}

func (f *ImageFetcher) FetchUpdatedLocalImage(ctx context.Context, imageName string, stdout io.Writer) (image.Image, error) {
	expectedImage, err := f.FetchRemoteImage(imageName)
	if err != nil {
		return nil, err
	}

	if found, err := expectedImage.Found(); err != nil {
		return nil, err
	} else if found {
		err = f.Docker.PullImage(ctx, imageName, stdout)
		if err != nil {
			return nil, err
		}
	}

	return f.FetchLocalImage(imageName)
}

func (f *ImageFetcher) FetchLocalImage(imageName string) (image.Image, error) {
	return f.Factory.NewLocal(imageName)
}

func (f *ImageFetcher) FetchRemoteImage(imageName string) (image.Image, error) {
	return f.Factory.NewRemote(imageName)
}
