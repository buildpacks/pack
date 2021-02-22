package pack

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
)

const assetDownloadWorkers = 4

type CreateAssetCacheOptions struct {
	ImageName string
	Assets    dist.Assets
	Publish   bool
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

type downloadResult struct {
	index  int
	pair   dist.BlobAssetPair
	sha256 string
	err    error
}

type downloadJob struct {
	asset dist.Asset
	index int
}

// TODO -Dan- parallel downloads should cleanly exit with Ctrl-C
// TODO -Dan- parallel download output is a bit messed up.
// existing behavior is a bit dangerous and can poison the cache.
func (c *Client) downloadAssets(assets []dist.Asset) (dist.BlobMap, error) {
	resultMap := make(dist.BlobMap)
	results := make(chan downloadResult, len(assets))
	jobs := make(chan downloadJob, len(assets))
	for workerCount := 0; workerCount < assetDownloadWorkers; workerCount++ {
		go downloadWorker(c.downloader, jobs, results)
	}

	for assetIdx, asset := range assets {
		jobs <- downloadJob{
			asset: asset,
			index: assetIdx,
		}
	}
	close(jobs)

	resultIndiciesMap := map[string]int{}
	errors := []error{}
	for i := 0; i < len(assets); i++ {
		r := <-results
		prevIdx, ok := resultIndiciesMap[r.sha256]
		switch {
		case r.err != nil:
			errors = append(errors, r.err)
		case !ok || prevIdx < r.index:
			resultIndiciesMap[r.sha256] = r.index
			resultMap[r.sha256] = r.pair
		}
	}

	joinedErrs := fmt.Errorf("the following errors occurred during download: %q", errorJoin(errors, ", "))
	if len(errors) > 0 {
		return resultMap, joinedErrs
	}
	return resultMap, nil
}

func errorJoin(elems []error, sep string) string {
	strArr := []string{}
	for _, elem := range elems {
		strArr = append(strArr, elem.Error())
	}

	return strings.Join(strArr, sep)
}

func downloadWorker(downloader Downloader, jobs <-chan downloadJob, results chan<- downloadResult) {
	for j := range jobs {
		var b blob.Blob = nil
		var err error
		if j.asset.URI != "" {
			b, err = downloader.Download(context.Background(), j.asset.URI, blob.RawDownload, blob.ValidateDownload(j.asset.Sha256))
		}
		results <- downloadResult{
			index: j.index,
			pair: dist.BlobAssetPair{
				Blob:     b,
				AssetVal: j.asset.ToAssetValue(""),
			},
			sha256: j.asset.Sha256,
			err:    err,
		}
	}
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
