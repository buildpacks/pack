package pack

import (
	"context"
	"sort"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/layer"
)

const downloadWorkerCount = 4

type CreateAssetCacheOptions struct {
	ImageName string
	Assets    dist.Assets
	Publish   bool
	OS        string
}

func (c *Client) CreateAssetCache(ctx context.Context, opts CreateAssetCacheOptions) error {
	img, err := newImageWithOS(opts.ImageName, opts.OS, !opts.Publish, c.imageFactory)
	if err != nil {
		return errors.Wrap(err, "unable to create asset cache image")
	}

	downloadManager := blob.NewDownloadManager(c.downloader, downloadWorkerCount)

	assets := simplifyAssets(opts.Assets)
	downloadResults, err := downloadManager.DownloadAndValidate(assetToDownloadJob(assets)...)
	if err != nil {
		return errors.Wrap(err, "unable to download assets")
	}

	tarWriterFactory, err := layer.NewWriterFactory(opts.OS)
	if err != nil {
		panic(err)
	}
	assetCacheImage, err := dist.NewAssetCacheImage(img, tarWriterFactory)
	if err != nil {
		panic(err)
	}

	addAssetsToImage(assetCacheImage, assets, downloadResults)
	return assetCacheImage.Save()
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

func assetToDownloadJob(assetList []dist.Asset) []blob.DownloadJob {
	result := []blob.DownloadJob{}
	for _, asset := range assetList {
		result = append(result, blob.DownloadJob{URI: asset.URI, Sha256: asset.Sha256})
	}

	return result
}

// simplifyAssets sorts assets by Sha256, and if multiple assets have the same
// sha256 value, we keep only the last one in the assets array.
func simplifyAssets(assets dist.Assets) dist.Assets {
	result := dist.Assets{}
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

// this method mutates the given assetImg
func addAssetsToImage(assetImg *dist.AssetCacheImage, assets dist.Assets, downloadMap map[blob.DownloadJob]blob.DownloadResult) {
	for _, asset := range assets {
		b, ok := downloadMap[blob.DownloadJob{URI: asset.URI, Sha256: asset.Sha256}]
		if !ok {
			continue
		}
		assetImg.AddAssetLayers(dist.BlobAssetPair{
			Blob:  b.Blob,
			Asset: asset,
		})
	}
}
