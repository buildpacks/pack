package pack

import (
	"context"

	"github.com/buildpack/lifecycle/image"

	"github.com/buildpack/pack/buildpack"
)

//go:generate mockgen -package mocks -destination mocks/image_fetcher.go github.com/buildpack/pack ImageFetcher

type ImageFetcher interface {
	Fetch(ctx context.Context, name string, daemon, pull bool) (image.Image, error)
}

type BuildpackFetcher interface {
	FetchBuildpack(localSearchPath string, bp buildpack.Buildpack) (buildpack.Buildpack, error)
}
