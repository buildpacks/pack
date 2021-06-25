package asset

import (
	"context"
	"fmt"
	"net/url"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/oci"
	"github.com/buildpacks/pack/internal/paths"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_downloader.go github.com/buildpacks/pack/internal/asset Downloader
// A Downloader is responsible for taking either a local filepath, or a URI
// and returning a Blob, which may be read from multiple times.
type Downloader interface {
	Download(ctx context.Context, pathOrURI string, options ...blob.DownloadOption) (blob.Blob, error)
}

//go:generate mockgen -package testmocks -destination testmocks/mock_file_fetcher.go github.com/buildpacks/pack/internal/asset FileFetcher

// A FileFetcher is responsible for taking a workingDir, and a list of filepaths to
// and returning a LayoutPackage for querying info about OCI image.
type FileFetcher interface {
	FetchFileAssets(ctx context.Context, workingDir string, fileAssetsPaths ...string) ([]*oci.LayoutPackage, error)
}

// PackageURIFetcher holds the internals for how to download remote files & fetch
// local files, this struct implements the URIFetcher interface.
type PackageURIFetcher struct {
	Downloader
	localFileFetcher FileFetcher
}

// NewPackageURIFetcher is a constructor and should be used to
// create new instances of PackageURIFetcher
func NewPackageURIFetcher(downloader Downloader, localFileFetcher FileFetcher) PackageURIFetcher {
	return PackageURIFetcher{
		Downloader:       downloader,
		localFileFetcher: localFileFetcher,
	}
}

// FetchURIAssets takes a list of URI's the use either the 'file' or 'http/https' protocol
// and returns an oci LayoutPackage for each such URI.
func (a PackageURIFetcher) FetchURIAssets(ctx context.Context, uriAssets ...string) ([]*oci.LayoutPackage, error) {
	result := []*oci.LayoutPackage{}
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
			p, err := oci.NewLayoutPackage(assetBlob)
			if err != nil {
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
