package asset_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/internal/paths"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/asset"
	"github.com/buildpacks/pack/internal/asset/testmocks"
	blob2 "github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/oci"
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
		subject            asset.PackageURIFetcher
		expectedAssetPackage *oci.LayoutPackage
		assert             = h.NewAssertionManager(t)

		expectedPackageBlob blob2.Blob
	)
	it.Before(func() {
		mockController = gomock.NewController(t)
		mockDownloader = testmocks.NewMockDownloader(mockController)
		mockFileFetcher = testmocks.NewMockFileFetcher(mockController)
		subject = asset.NewPackageURIFetcher(mockDownloader, mockFileFetcher)

		var err error
		expectedAssetPackage, err = oci.NewLayoutPackage(blob2.NewBlob(
			filepath.Join("testdata", "fake-asset-package.tar"), blob2.RawOption),
		)
		assert.Nil(err)

		expectedPackageBlob = blob2.NewBlob("testdata/fake-asset-package.tar", blob2.RawOption)
	})
	when("#FetchURIAssets", func() {
		when("url uses 'http' scheme", func() {
			it("downloads asset", func() {
				assetURI := "http://asset/uri"
				mockDownloader.EXPECT().Download(gomock.Any(), assetURI, gomock.Any()).
					Return(expectedPackageBlob, nil)

				ociAssets, err := subject.FetchURIAssets(context.Background(), assetURI)
				assert.Nil(err)

				assert.Equal(len(ociAssets), 1)

				assertSameAssetLayers(t, ociAssets[0], expectedAssetPackage)
			})
		})
		when("url uses 'https' scheme", func() {
			it("downloads asset", func() {
				assetURI := "https://asset/uri"
				mockDownloader.EXPECT().Download(gomock.Any(), assetURI, gomock.Any()).
					Return(expectedPackageBlob, nil)

				ociAssets, err := subject.FetchURIAssets(context.Background(), assetURI)
				assert.Nil(err)

				assert.Equal(len(ociAssets), 1)

				assertSameAssetLayers(t, ociAssets[0], expectedAssetPackage)
			})
		})
		when("url uses 'file' scheme", func() {
			it("opens local file asset", func() {
				absPath, err := filepath.Abs(filepath.Join("testdata", "fake-asset-package.tar"))
				assert.Nil(err)

				assetURI, err := paths.FilePathToURI(filepath.Join("testdata", "fake-asset-package.tar"), ".")
				assert.Nil(err)

				mockFileFetcher.EXPECT().FetchFileAssets(gomock.Any(), gomock.Any(), absPath).
					Return([]*oci.LayoutPackage{expectedAssetPackage}, nil)

				ociAssets, err := subject.FetchURIAssets(context.Background(), assetURI)
				assert.Nil(err)

				assert.Equal(len(ociAssets), 1)

				assertSameAssetLayers(t, ociAssets[0], expectedAssetPackage)
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
			when("file asset is unable to be opened", func() {
				it("errors with helpful message", func() {
					assetPath, err := filepath.Abs(filepath.Join("/some", "file"))
					assert.Nil(err)

					assetURI, err := paths.FilePathToURI(assetPath, "")
					assert.Nil(err)

					mockFileFetcher.EXPECT().FetchFileAssets(gomock.Any(), gomock.Any(), assetPath).
						Return(nil, errors.New("unable to open file"))

					_, err = subject.FetchURIAssets(context.Background(), assetURI)
					assert.ErrorContains(err, `unable to fetch local file asset: "unable to open file"`)
				})
			})
			when("unable to open asset as OCI package", func() {
				var mockBlob *testmocks.MockBlob
				it.Before(func() {
					mockBlob = testmocks.NewMockBlob(mockController)
				})
				it("errors with a helpful message", func() {
					assetURI := "http://asset/uri"
					mockDownloader.EXPECT().Download(gomock.Any(), assetURI, gomock.Any()).
						Return(mockBlob, nil)
					mockBlob.EXPECT().Open().Return(nil, errors.New("open blob error"))

					_, err := subject.FetchURIAssets(context.Background(), assetURI)
					assert.ErrorContains(err, "error opening asset package in OCI format")
				})
			})
		})
	})
}
