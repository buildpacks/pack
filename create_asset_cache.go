package pack

import (
	"context"
	"fmt"
	pubcfg "github.com/buildpacks/pack/config"
	"io/ioutil"
	"strings"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/layer"
)

const assetDownloadWorkers = 4

type CreateAssetCacheOptions struct {
	ImageName string
	Assets    dist.Assets
	Publish   bool
	OS        string
}

func (c *Client) CreateAssetCache(ctx context.Context, opts CreateAssetCacheOptions) error {
	img, err := c.imageFactory.NewImage(opts.ImageName, !opts.Publish)
	if err != nil {
		return fmt.Errorf("unable to create asset cache image: %q", err)
	}
	if opts.OS == pubcfg.WindowsOS {
		err := img.SetOS(pubcfg.WindowsOS)
		if err != nil {
			panic(err)
		}
		windowsTmpDir, err := ioutil.TempDir("", "windows-base-layer")
		if err != nil {
			panic(err)
		}
		err = buildpackage.AddWindowsShimBaseLayer(img, windowsTmpDir)
		if err != nil {
			panic(err)
		}
	}

	blobMap, err := c.downloadAssets(opts.Assets)
	if err != nil {
		return err
	}

	// TODO -Dan- Do we use these results???
	assetMap := opts.Assets.ToIncompleteAssetMap()
	assetMap.Filter(blobMap.Keys())

	tarWriterFactory, err := layer.NewWriterFactory(opts.OS)
	if err != nil {
		panic(err)
	}
	assetCacheImage := dist.NewAssetCacheImage(img, blobMap, tarWriterFactory)
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

