package asset_test

import (
	"context"
	"errors"
	"github.com/buildpacks/imgutil/fakes"
	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/asset"
	"github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"testing"
)

func TestAssetFetcher(t *testing.T) {
	spec.Run(t, "Asset Fetcher", testAssetFetcher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAssetFetcher(t *testing.T, when spec.G, it spec.S) {
	var (
		assert         = testhelpers.NewAssertionManager(t)
		mockController *gomock.Controller
		mockFetcher    *testmocks.MockImageFetcher
		subject        asset.AssetImageFetcher
	)
	it.Before(func() {
		mockController = gomock.NewController(t)
		mockFetcher = testmocks.NewMockImageFetcher(mockController)
		subject = asset.NewImageFetcher(mockFetcher)
	})
	when("fetching asset cache images", func() {
		var (
			assetAImage = fakes.NewImage("assetAImage", "", nil)
			assetBImage = fakes.NewImage("assetBImage", "", nil)
		)
		it("fetches all assets with the passed pull policy", func() {
			mockFetcher.EXPECT().Fetch(gomock.Any(), "assetAImage", true, pubcfg.PullIfNotPresent).Return(assetAImage, nil)
			mockFetcher.EXPECT().Fetch(gomock.Any(), "assetBImage", true, pubcfg.PullIfNotPresent).Return(assetBImage, nil)

			imageAssets, err := subject.FetchImageAssets(context.Background(), pubcfg.PullIfNotPresent, "assetAImage", "assetBImage")
			assert.Nil(err)

			assert.Equal(len(imageAssets), 2)
			firstImageAsset, ok := (imageAssets[0]).(*fakes.Image)
			assert.Equal(ok, true)

			assert.Equal(firstImageAsset.Name(), "assetAImage")

			secondImageAsset, ok := (imageAssets[1]).(*fakes.Image)
			assert.Equal(ok, true)

			assert.Equal(secondImageAsset.Name(), "assetBImage")
		})
	})

	when("failure cases", func() {
		when("unable to fetch image", func() {
			it.Before(func() {
				subject = asset.NewImageFetcher(mockFetcher)
			})
			it("errors with a helpful message", func() {
				mockFetcher.EXPECT().Fetch(gomock.Any(), "errorImage", true, pubcfg.PullIfNotPresent).Return(nil, errors.New("a bad error"))

				_, err := subject.FetchImageAssets(context.Background(), pubcfg.PullIfNotPresent, "errorImage")
				assert.ErrorContains(err, `unable to fetch asset image: "a bad error"`)
			})
		})
	})
}
