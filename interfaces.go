package pack

import (
	"context"

	"github.com/buildpack/pack/blob"

	"github.com/buildpack/imgutil"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_image_fetcher.go github.com/buildpack/pack ImageFetcher

type ImageFetcher interface {
	// Fetch fetches an image by resolving it both remotely and locally depending on provided parameters.
	// If daemon is true, it will look return a `local.Image`. Pull, applicable only when daemon is true, will
	// attempt to pull a remote image first.
	Fetch(ctx context.Context, name string, daemon, pull bool) (imgutil.Image, error)
}

//go:generate mockgen -package testmocks -destination testmocks/mock_downloader.go github.com/buildpack/pack Downloader

type Downloader interface {
	Download(ctx context.Context, pathOrURI string) (blob.Blob, error)
}

//go:generate mockgen -package testmocks -destination testmocks/mock_image_factory.go github.com/buildpack/pack ImageFactory

type ImageFactory interface {
	NewImage(repoName string, local bool) (imgutil.Image, error)
}
