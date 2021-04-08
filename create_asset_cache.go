package pack

import (
	"context"
	"github.com/buildpacks/pack/internal/asset"
	"github.com/google/go-containerregistry/pkg/v1/empty"
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
	Format    string
}

type AssetCache interface {
	Save(additionalNames ...string) error
	AddAssetBlobs(pairs ...asset.Blob)
}

func (c *Client) CreateAssetCache(ctx context.Context, opts CreateAssetCacheOptions) error {
	var assetCache AssetCache
	tarWriterFactory, err := layer.NewWriterFactory(opts.OS)
	if err != nil {
		return errors.Wrap(err, "unable to create layer tar writer")
	}
	assetLayerWriter := asset.NewLayerWriter(tarWriterFactory)
	switch {
	case opts.Format == FormatFile:
		eImg := empty.Image
		assetCache = asset.NewFile(opts.ImageName, opts.OS, eImg, assetLayerWriter)
	default:
		img, err := newImageWithOS(opts.ImageName, opts.OS, !opts.Publish, c.imageFactory)
		if err != nil {
			return errors.Wrap(err, "unable to create asset cache base image")
		}
		assetCache = asset.NewImage(img, assetLayerWriter)
	}
	downloadManager := blob.NewDownloadManager(c.downloader, downloadWorkerCount)

	assets := simplifyAssets(opts.Assets)
	downloadJobs := assetToDownloadJob(assets)
	downloadResults, err := downloadManager.DownloadAndValidate(ctx, downloadJobs...)
	if err != nil {
		return errors.Wrap(err, "unable to download assets")
	}

	addAssetsToImage(assetCache, assets, downloadResults)
	return assetCache.Save()
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
func addAssetsToImage(assetImg AssetCache, assets dist.Assets, downloadMap map[blob.DownloadJob]blob.DownloadResult) {
	for _, curAsset := range assets {
		b, ok := downloadMap[blob.DownloadJob{URI: curAsset.URI, Sha256: curAsset.Sha256}]
		if !ok || b.Blob == nil{
			continue
		}
		assetImg.AddAssetBlobs(asset.FromRawBlob(curAsset, b.Blob))
	}
}
