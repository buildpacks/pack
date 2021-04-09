package asset_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/asset"
	"github.com/buildpacks/pack/internal/asset/testmocks"
	blob2 "github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/ocipackage"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestURLFetcher(t *testing.T) {
	spec.Run(t, "URLFetcher", testURLFetcher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testURLFetcher(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController     *gomock.Controller
		mockDownloader     *testmocks.MockDownloader
		mockFileFetcher    *testmocks.MockFileFetcher
		subject            asset.AssetURIFetcher
		expectedAssetCache *ocipackage.OciLayoutPackage
		assert             = h.NewAssertionManager(t)

		expectedCacheBlob blob2.Blob
	)
	it.Before(func() {
		mockController = gomock.NewController(t)
		mockDownloader = testmocks.NewMockDownloader(mockController)
		mockFileFetcher = testmocks.NewMockFileFetcher(mockController)
		subject = asset.NewAssetURLFetcher(mockDownloader, mockFileFetcher)

		var err error
		expectedAssetCache, err = ocipackage.NewOCILayoutPackage(blob2.NewBlob(
			filepath.Join("testdata", "fake-asset-cache.tar"), blob2.RawOption),
		)
		assert.Nil(err)

		expectedCacheBlob = blob2.NewBlob("testdata/fake-asset-cache.tar", blob2.RawOption)
	})
	when("#FetchURIAssets", func() {
		when("url uses 'http' scheme", func() {
			it("downloads asset", func() {
				assetURI := "http://asset/uri"
				mockDownloader.EXPECT().Download(gomock.Any(), assetURI, gomock.Any()).
					Return(expectedCacheBlob, nil)

				ociAssets, err := subject.FetchURIAssets(context.Background(), assetURI)
				assert.Nil(err)

				assert.Equal(len(ociAssets), 1)

				assertSameAssetLayers(t, ociAssets[0], expectedAssetCache)
			})
		})
		when("url uses 'https' scheme", func() {
			it("downloads asset", func() {
				assetURI := "https://asset/uri"
				mockDownloader.EXPECT().Download(gomock.Any(), assetURI, gomock.Any()).
					Return(expectedCacheBlob, nil)

				ociAssets, err := subject.FetchURIAssets(context.Background(), assetURI)
				assert.Nil(err)

				assert.Equal(len(ociAssets), 1)

				assertSameAssetLayers(t, ociAssets[0], expectedAssetCache)
			})
		})
		when("url uses 'file' scheme", func() {
			it("opens local file asset", func() {
				absPath, err := filepath.Abs("testdata/fake-asset-cache.tar")
				assert.Nil(err)

				assetURI := fmt.Sprintf("file://%s", absPath)
				mockFileFetcher.EXPECT().FetchFileAssets(gomock.Any(), gomock.Any(), absPath).
					Return([]*ocipackage.OciLayoutPackage{expectedAssetCache}, nil)

				ociAssets, err := subject.FetchURIAssets(context.Background(), assetURI)
				assert.Nil(err)

				assert.Equal(len(ociAssets), 1)

				assertSameAssetLayers(t, ociAssets[0], expectedAssetCache)
			})
		})
		when("error cases", func() {
			when("unknown uri scheme", func() {
				it("errors with helpful message", func() {
					_, err := subject.FetchURIAssets(context.Background(), "scheme://some-path")
					assert.ErrorContains(err, `unable to handle url scheme: "scheme"`)
				})
			})
			when("unable to parse uri", func() {
				it("errors with helpful message", func() {
					_, err := subject.FetchURIAssets(context.Background(), "::::")
					assert.ErrorContains(err, "unable to parse asset url")
				})
			})
			when("http asset fails to download", func() {
				it("errors with helpful message", func() {
					assetURI := "http://asset/uri"
					mockDownloader.EXPECT().Download(gomock.Any(), assetURI, gomock.Any()).
						Return(nil, errors.New("error downloading asset"))

					_, err := subject.FetchURIAssets(context.Background(), assetURI)
					assert.ErrorContains(err, `unable to download asset: "error downloading asset"`)
				})
			})
			when("file asset is able to be opened", func() {
				it("errors with helpful message", func() {
					assetURI := "file:///some/file"
					mockFileFetcher.EXPECT().FetchFileAssets(gomock.Any(), gomock.Any(), "/some/file").
						Return(nil, errors.New("unable to open file"))

					_, err := subject.FetchURIAssets(context.Background(), assetURI)
					assert.ErrorContains(err, `unable to fetch local file asset: "unable to open file"`)
				})
			})
		})
	})
}
