package pack

import (
	"context"

	"github.com/buildpack/pack/blob"

	"github.com/buildpack/imgutil"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_image_fetcher.go github.com/buildpack/pack ImageFetcher

type ImageFetcher interface {
	Fetch(ctx context.Context, name string, daemon, pull bool) (imgutil.Image, error)
}

//go:generate mockgen -package testmocks -destination testmocks/mock_downloader.go github.com/buildpack/pack Downloader

type Downloader interface {
	Download(ctx context.Context, pathOrURI string) (blob.Blob, error)
}
