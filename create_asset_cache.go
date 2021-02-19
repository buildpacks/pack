package pack

import (
	"context"
	"fmt"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/google/go-containerregistry/pkg/name"
)

type CreateAssetCacheOptions struct {
	ImageName string
	Assets    dist.Assets
	Publish bool
}

func (c *Client) CreateAssetCache(ctx context.Context, opts CreateAssetCacheOptions) error {
	validOpts, err := validateConfig(opts)
	if err != nil {
		return err
	}

	img, err := c.imageFactory.NewImage(validOpts.ImageName, !opts.Publish)
	if err != nil {
		return fmt.Errorf("unable to create asset cache image: %q", err)
	}

	blobMap, err := c.downloadAssets(opts.Assets)
	if err != nil {
		return err
	}

	assetMap := opts.Assets.ToIncompleteAssetMap()
	assetMap.Filter(blobMap.Keys())

	assetCacheImage := dist.NewAssetCacheImage(img, blobMap)
	return assetCacheImage.Save()
}

// TODO -Dan- downloads should be concurrent.
func (c *Client) downloadAssets(assets []dist.Asset) (dist.BlobMap, error) {
	result := make(dist.BlobMap)
	for _, asset := range assets {
		if asset.URI == "" {
			continue
		}
		b, err := c.downloader.Download(context.Background(), asset.URI, blob.RawDownload, blob.ValidateDownload(asset.Sha256))
		if err != nil {
			return dist.BlobMap{}, err
		}
		result[asset.Sha256] = dist.NewBlobAssetPair(b,asset.ToAssetValue(""))
	}
	return result, nil
}

func validateConfig(cfg CreateAssetCacheOptions) (CreateAssetCacheOptions, error) {
	tag, err := name.NewTag(cfg.ImageName, name.WeakValidation)
	if err != nil {
		return CreateAssetCacheOptions{}, fmt.Errorf("invalid asset cache image name: %q", err)
	}
	return CreateAssetCacheOptions{
		ImageName: tag.String(),
	}, nil
}