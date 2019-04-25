package pack

import (
	"context"

	"github.com/buildpack/imgutil"

	"github.com/buildpack/pack/buildpack"
)

//go:generate mockgen -package mocks -destination mocks/image_fetcher.go github.com/buildpack/pack ImageFetcher

type ImageFetcher interface {
	Fetch(ctx context.Context, name string, daemon, pull bool) (imgutil.Image, error)
}

//go:generate mockgen -package mocks -destination mocks/buildpack_fetcher.go github.com/buildpack/pack BuildpackFetcher

type BuildpackFetcher interface {
	FetchBuildpack(uri string) (buildpack.Buildpack, error)
}
