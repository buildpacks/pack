package asset

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"net/url"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/ocipackage"
	"github.com/buildpacks/pack/internal/paths"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_downloader.go github.com/buildpacks/pack/internal/asset Downloader
type Downloader interface {
	Download(ctx context.Context, pathOrURI string, options ...blob.DownloadOption) (blob.Blob, error)
}

//go:generate mockgen -package testmocks -destination testmocks/mock_file_fetcher.go github.com/buildpacks/pack/internal/asset FileCacheFetcher
type FileCacheFetcher interface {
	FetchFileAssets(ctx context.Context, workingDir string, fileAssets ...string) ([]*ocipackage.OciLayoutPackage, error)
}

type URIFetcher struct {
	Downloader
	localFileFetcher FileCacheFetcher
}

func NewAssetURLFetcher(downloader Downloader, localFileFetcher FileCacheFetcher) URIFetcher {
	return URIFetcher{
		Downloader:       downloader,
		localFileFetcher: localFileFetcher,
	}
}

func (a URIFetcher) FetchURIAssets(ctx context.Context, uriAssets ...string) ([]*ocipackage.OciLayoutPackage, error) {
	result := []*ocipackage.OciLayoutPackage{}
	for _, assetFile := range uriAssets {
		uri, err := url.Parse(assetFile)
		if err != nil {
			return result, fmt.Errorf("unable to parse asset url: %s", err)
		}
		switch uri.Scheme {
		case "http", "https":
			assetBlob, err := a.Download(ctx, uri.String(), blob.RawDownload)
			if err != nil {
				return result, fmt.Errorf("unable to download asset: %q", err)
			}
			p, err := ocipackage.NewOCILayoutPackage(assetBlob)
			if err != nil { // TODO -Dan- handle error
				return result, errors.Wrap(err, "error opening asset package in OCI format")
			}
			result = append(result, p)
		case "file":
			assetFilePath, err := paths.URIToFilePath(uri.String())
			if err != nil {
				return result, fmt.Errorf("unable to get asset filepath: %q", err)
			}
			assetsFromFile, err := a.localFileFetcher.FetchFileAssets(ctx, "", assetFilePath)
			if err != nil {
				return result, fmt.Errorf("unable to fetch local file asset: %q", err)
			}

			result = append(result, assetsFromFile...)
		default:
			return result, fmt.Errorf("unable to handle url scheme: %q", uri.Scheme)
		}
	}

	return result, nil
}
