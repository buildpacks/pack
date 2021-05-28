package pack

import (
	"context"
	"sort"

	"github.com/google/go-containerregistry/pkg/v1/empty"

	"github.com/buildpacks/pack/internal/asset"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/layer"
)

const downloadWorkerCount = 4

// CreateAssetPackageOptions is a configuration object passed to CreateAssetPackage
type CreateAssetPackageOptions struct {
	// Name of the output image, this may be a local filepath, or valid OCI image name
	ImageName string
	// List of Assets to appear in the final asset package.
	// assets with no URL will be omitted.
	Assets []dist.AssetInfo
	// publish resulting asset cache to registry.
	// option only used when Format = "image".
	Publish bool
	// OS type for the image, valid options are:
	// - windows
	// - linux
	OS string
	// Format to write output Asset Package, valid options are:
	// - file
	// - image
	Format string
}

// Minimum interface needed to successfully write an Asset Package in some format.
type AssetPackage interface {
	Save(additionalNames ...string) error
	AddAssetBlobs(pairs ...asset.Blob)
}

// CreateAssetPackage writes a new Asset Package image using options specified in opts.
// This image can be used to add assets to builds.
func (c *Client) CreateAssetPackage(ctx context.Context, opts CreateAssetPackageOptions) error {
	var assetPackage AssetPackage
	tarWriterFactory, err := layer.NewWriterFactory(opts.OS)
	if err != nil {
		return errors.Wrap(err, "unable to create layer tar writer")
	}
	assetLayerWriter := asset.NewLayerWriter(tarWriterFactory)
	switch {
	case opts.Format == FormatFile:
		eImg := empty.Image
		assetPackage = asset.NewFile(opts.ImageName, opts.OS, eImg, assetLayerWriter)
	default:
		img, err := newImageWithOS(opts.ImageName, opts.OS, !opts.Publish, c.imageFactory)
		if err != nil {
			return errors.Wrap(err, "unable to create asset package base image")
		}
		assetPackage = asset.NewImage(img, assetLayerWriter)
	}
	downloadManager := blob.NewDownloadManager(c.downloader, downloadWorkerCount)

	assets := simplifyAssets(opts.Assets)
	downloadJobs := assetToDownloadJob(assets)
	downloadResults, err := downloadManager.DownloadAndValidate(ctx, downloadJobs...)
	if err != nil {
		return errors.Wrap(err, "unable to download assets")
	}

	err = addAssetsToImage(assetPackage, assets, downloadResults)
	if err != nil {
		return errors.Wrapf(err, "unable to add asset blobs to assets package")
	}
	return assetPackage.Save()
}

func newImageWithOS(imgName, os string, local bool, imgFactory ImageFactory) (imgutil.Image, error) {
	img, err := imgFactory.NewImage(imgName, local)
	if err != nil {
		return nil, err
	}
	if err := img.SetOS(os); err != nil {
		return nil, err
	}
	return img, nil
}

func assetToDownloadJob(assetList []dist.AssetInfo) []blob.DownloadJob {
	result := []blob.DownloadJob{}
	for _, asset := range assetList {
		result = append(result, blob.DownloadJob{URI: asset.URI, Sha256: asset.Sha256})
	}

	return result
}

// simplifyAssets sorts assets by Sha256, and if multiple assets have the same
// sha256 value, we keep only the last one in the assets array.
func simplifyAssets(assets []dist.AssetInfo) []dist.AssetInfo {
	result := []dist.AssetInfo{}
	sort.SliceStable(assets, func(i, j int) bool {
		return assets[i].Sha256 < assets[j].Sha256
	})

	prevAssetSha := ""
	for _, asset := range assets {
		switch {
		case asset.Sha256 == prevAssetSha:
			result[len(result)-1] = asset
		default:
			result = append(result, asset)
			prevAssetSha = asset.Sha256
		}
	}

	return result
}

// addAssetsToImage takes a list of assets in assetList, checks to see if they have a corresponding blob in
// the download map and if so adds the blob to our assetImg.
func addAssetsToImage(assetImg AssetPackage, assetList []dist.AssetInfo, downloadMap map[blob.DownloadJob]blob.DownloadResult) error {
	for _, curAsset := range assetList {
		b, ok := downloadMap[blob.DownloadJob{URI: curAsset.URI, Sha256: curAsset.Sha256}]
		if !ok || b.Blob == nil {
			continue
		}
		aBlob, err := asset.FromRawBlob(curAsset, b.Blob)
		if err != nil {
			return err
		}
		assetImg.AddAssetBlobs(aBlob)
	}
	return nil
}
