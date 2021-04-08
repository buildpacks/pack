package asset_test

import (
	"errors"
	"github.com/buildpacks/pack/internal/asset"
	fakes2 "github.com/buildpacks/pack/internal/asset/fakes"
	"github.com/buildpacks/pack/internal/asset/testmocks"
	"github.com/buildpacks/pack/internal/dist"
	testmocks2 "github.com/buildpacks/pack/testmocks"
	"github.com/golang/mock/gomock"
	"testing"

	"github.com/buildpacks/imgutil/fakes"

	"github.com/sclevine/spec"

	h "github.com/buildpacks/pack/testhelpers"
)

func TestAssetCacheImage(t *testing.T) {
	spec.Run(t, "TestAssetCache", testAssetCacheImage)
}

func testAssetCacheImage(t *testing.T, when spec.G, it spec.S) {
	var (
		assert     = h.NewAssertionManager(t)
		firstAsset = dist.Asset{
			Sha256:  "first-sha256",
			ID:      "first-asset",
			Version: "1.1.1",
			Name:    "First Asset",
			Stacks:  []string{"stack1", "stack2"},
		}
		secondAsset = dist.Asset{
			Sha256:  "second-sha256",
			ID:      "second-asset",
			Version: "2.2.2",
			Name:    "Second Asset",
			Stacks:  []string{"stack1", "stack2"},
		}

		baseImage *fakes.Image

		firstAssetBlob  asset.Blob
		secondAssetBlob asset.Blob

		mockController  *gomock.Controller
		mockLayerWriter *testmocks.MockLayerWriter

		subject *asset.Image
	)
	it.Before(func() {
		mockController = gomock.NewController(t)
		mockLayerWriter = testmocks.NewMockLayerWriter(mockController)
		baseImage = fakes.NewImage("fake-base-image", "", nil)

		firstAssetBlob = fakes2.NewFakeAssetBlob("first layer contents", firstAsset)
		secondAssetBlob = fakes2.NewFakeAssetBlob("second layer contents", secondAsset)

		subject = asset.NewImage(baseImage, mockLayerWriter)
	})
	when("#AddAssetBlobs", func() {
		it("adds asset layers to the LayerWriter", func() {
			mockLayerWriter.EXPECT().AddAssetBlobs(firstAssetBlob, secondAssetBlob)
			subject.AddAssetBlobs(firstAssetBlob, secondAssetBlob)
		})
	})

	when("#Save", func() {
		when("OS is linux", func() {
			it.Before(func() {
				assert.Succeeds(baseImage.SetOS("linux"))
			})
			it("uses LayerWriter to write assets to image", func() {
				mockLayerWriter.EXPECT().Open()
				mockLayerWriter.EXPECT().Write(subject)
				mockLayerWriter.EXPECT().Close()

				assert.Succeeds(subject.Save("first-name", "second-name"))
			})
		})
		when("OS is windows", func() {
			it.Before(func() {
				assert.Succeeds(baseImage.SetOS("windows"))
			})
			it("uses LayerWriter to write assets to image", func() {
				mockLayerWriter.EXPECT().Open()
				mockLayerWriter.EXPECT().Write(subject)
				mockLayerWriter.EXPECT().Close()

				assert.Succeeds(subject.Save("first-name", "second-name"))

				path, err := baseImage.FindLayerWithPath("Files/Windows")
				assert.Nil(err)

				// assert this has at least one windows specific file
				assert.NotEqual(path, "")
			})
		})
	})
	when("error cases", func() {
		var mockImage *testmocks2.MockImage
		it.Before(func() {
			mockImage = testmocks2.NewMockImage(mockController)
		})
		when("unable to get image OS" ,func() {
			it("errors with a helpful message", func() {
				subject = asset.NewImage(mockImage, mockLayerWriter)

				mockImage.EXPECT().OS().Return("", errors.New("error getting OS"))

				err := subject.Save()
				assert.ErrorContains(err,"unable to get asset cache image os")
			})
		})

		when("unable to open writer", func() {
			it("errors with a helpful message", func() {
				subject = asset.NewImage(mockImage, mockLayerWriter)

				mockImage.EXPECT().OS().Return("linux", nil)
				mockLayerWriter.EXPECT().Open().Return(errors.New("asset writer error"))

				err := subject.Save()
				assert.ErrorContains(err,"unable to open asset writer")
			})
		})

		when("adding windows base layer", func() {
			when("writing base layer fails", func() {
				it("errors with helpful message" ,func() {
					windowsBaseLayerDiffID := "sha256:067833dadeca9180bb71a211248cbc0f6ada05499bfdef07dfb04e2eb96e7c82"
					subject = asset.NewImage(mockImage, mockLayerWriter)
					mockImage.EXPECT().OS().Return("windows", nil)
					mockImage.EXPECT().AddLayerWithDiffID(gomock.Any(), windowsBaseLayerDiffID).Return(errors.New("error writing windows base layer"))

					err := subject.Save()
					assert.ErrorContains(err, "unable to write windows base layer")
				})
			})
		})

		when("writing LayerWriter fails to Write", func() {
			it("errors with a helpful message", func() {
				subject = asset.NewImage(mockImage, mockLayerWriter)
				mockImage.EXPECT().OS().Return("linux", nil)
				mockLayerWriter.EXPECT().Open()
				mockLayerWriter.EXPECT().Write(subject).Return(errors.New("error writing asset layers"))
				mockLayerWriter.EXPECT().Close()


				err := subject.Save()
				assert.ErrorContains(err, "unable to write asset layers to image")
			})
		})

		when("underlying image save fails", func() {
			it("errors with a helpful message", func() {
				subject = asset.NewImage(mockImage, mockLayerWriter)
				mockImage.EXPECT().OS().Return("linux", nil)
				mockLayerWriter.EXPECT().Open()
				mockLayerWriter.EXPECT().Write(subject)
				mockLayerWriter.EXPECT().Close()

				mockImage.EXPECT().Save().Return(errors.New("image write error"))

				err := subject.Save()
				assert.ErrorContains(err, "image write error")
			})
		})
	})
}
