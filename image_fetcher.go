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

	found, err := expectedImage.Found()
	if err != nil {
		return nil, err
	}

	if found {
		err = f.Docker.PullImage(ctx, imageName, stdout)
		if err != nil {
			return nil, err
		}

	}
	expectedImage, err = f.FetchLocalImage(imageName)
	if err != nil {
		return nil, err
	}

	return expectedImage, nil
}

func (f *ImageFetcher) FetchLocalImage(imageName string) (image.Image, error) {
	expectedImage, err := f.Factory.NewLocal(imageName)

	if err != nil {
		return nil, err
	}

	found, err := expectedImage.Found()

	if err != nil {
		return nil, err
	}

	if !found {
		return nil, nil
	}

	return expectedImage, nil
}

func (f *ImageFetcher) FetchRemoteImage(imageName string) (image.Image, error) {
	expectedImage, err := f.Factory.NewRemote(imageName)

	if err != nil {
		return nil, err
	}

	return expectedImage, nil
}
