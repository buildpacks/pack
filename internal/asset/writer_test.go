package asset_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/asset"
	fakes3 "github.com/buildpacks/pack/internal/asset/fakes"
	testmocks2 "github.com/buildpacks/pack/internal/asset/testmocks"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/layer"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
)

func TestLayerWriter(t *testing.T) {
	spec.Run(t, "layerWriter", testLayerWriter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLayerWriter(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController *gomock.Controller
		baseImage      *fakes.Image
		firstAsset     = dist.Asset{
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
		thirdAsset = dist.Asset{
			Sha256:  "third-sha256",
			ID:      "third-asset",
			Version: "3.3.3",
			Name:    "Third Asset",
			Stacks:  []string{"stack1", "stack2"},
		}
		firstAssetBlob  asset.Blob
		secondAssetBlob asset.Blob
		thirdAssetBlob  asset.Blob

		subject asset.LayerWriter
	)
	it.Before(func() {
		mockController = gomock.NewController(t)
		lw, err := layer.NewWriterFactory("linux")
		h.AssertNil(t, err)

		firstAssetBlob = fakes3.NewFakeAssetBlob("first layer contents", firstAsset)
		secondAssetBlob = fakes3.NewFakeAssetBlob("second layer contents", secondAsset)
		thirdAssetBlob = fakes3.NewFakeAssetBlob("third layer contents", thirdAsset)

		baseImage = fakes.NewImage("fake-base-image", "", nil)
		subject = asset.NewLayerWriter(lw)
	})
	when("#Write", func() {
		it("adds asset layers and layer metadata", func() {
			subject.AddAssetBlobs(firstAssetBlob, secondAssetBlob, thirdAssetBlob)
			err := subject.Open()
			//defer subject.Close()

			err = subject.Write(baseImage)
			h.AssertNil(t, err)

			assertHasSameAssets(t, baseImage, firstAsset.Sha256, "first layer contents")
			assertHasSameAssets(t, baseImage, secondAsset.Sha256, "second layer contents")
			assertHasSameAssets(t, baseImage, thirdAsset.Sha256, "third layer contents")

			label, err := baseImage.Label("io.buildpacks.asset.layers")
			h.AssertNil(t, err)

			var assetMap dist.AssetMap
			h.AssertNil(t, json.Unmarshal([]byte(label), &assetMap))

			h.AssertEq(t, assetMap, dist.AssetMap{
				"first-sha256": dist.AssetValue{
					ID:          "first-asset",
					Version:     "1.1.1",
					Name:        "First Asset",
					LayerDiffID: "sha256:a4862301135f9226bb817638e448911b47798ec58a36a147c51176b5337ff92b",
					Stacks:      []string{"stack1", "stack2"},
				},
				"second-sha256": dist.AssetValue{
					ID:          "second-asset",
					Version:     "2.2.2",
					Name:        "Second Asset",
					LayerDiffID: "sha256:77a3e5c96a61c35e4c56c080cd851b1a558c6f7877b6bcf67aaf97a59aeb2171",
					Stacks:      []string{"stack1", "stack2"},
				},
				"third-sha256": dist.AssetValue{
					ID:          "third-asset",
					Version:     "3.3.3",
					Name:        "Third Asset",
					LayerDiffID: "sha256:f9e6914d8f71a425ecb9168da71685b1a77a81e838d950bf4f4825e36b88605d",
					Stacks:      []string{"stack1", "stack2"},
				},
			})
		})
	})

	when("#AddAssetBlobs", func() {
		it("updates the asset metadata", func() {
			subject.AddAssetBlobs(firstAssetBlob, secondAssetBlob, thirdAssetBlob)
			metadata := subject.AssetMetadata()
			h.AssertEq(t, metadata, dist.AssetMap{
				"first-sha256": dist.AssetValue{
					ID:      "first-asset",
					Version: "1.1.1",
					Name:    "First Asset",
					Stacks:  []string{"stack1", "stack2"},
				},
				"second-sha256": dist.AssetValue{
					ID:      "second-asset",
					Version: "2.2.2",
					Name:    "Second Asset",
					Stacks:  []string{"stack1", "stack2"},
				},
				"third-sha256": dist.AssetValue{
					ID:      "third-asset",
					Version: "3.3.3",
					Name:    "Third Asset",
					Stacks:  []string{"stack1", "stack2"},
				},
			})
		})
	})

	when("error cases", func() {
		when("writing before opening LayerWriter", func() {
			it("errors with a helpful message", func() {
				err := subject.Write(baseImage)
				h.AssertError(t, err, "layerWriter must be opened before writing")
			})
		})
		when("multiple calls to Open", func() {
			it.Before(func() {
				h.AssertNil(t, subject.Open())
			})
			it.After(func() {
				subject.Close()
			})
			it("errors with a helpful message", func() {
				err := subject.Open()
				h.AssertError(t, err, "unable to open writer: writer already open")
			})
		})
		when("closing without unopened writer", func() {
			it("errors with a helpful message", func() {
				err := subject.Close()
				h.AssertError(t, err, "unable to close writer: writer is not open")

			})
		})
		when("unable to write Layers to Writable", func() {
			it.Before(func() {
				h.AssertNil(t, subject.Open())
			})
			it.After(func() {
				subject.Close()
			})
			it("erros with a helpful message", func() {
				mockImage := testmocks.NewMockImage(mockController)
				mockImage.EXPECT().AddLayerWithDiffID(gomock.Any(), gomock.Any()).Return(errors.New("add layer error"))

				subject.AddAssetBlobs(firstAssetBlob)
				err := subject.Write(mockImage)
				h.AssertError(t, err, "unable to write layer")
			})
		})

		when("no metadata found asset when writing", func() {
			it.Before(func() {
				h.AssertNil(t, subject.Open())
			})
			it.After(func() {
				subject.Close()
			})

			it("errors with a helpful message", func() {
				mockBlob := testmocks2.NewMockBlob(mockController)
				mockBlob.EXPECT().AssetDescriptor().Return(firstAsset)

				subject.AddAssetBlobs(mockBlob)

				// change so we get new asset sha256 values
				mockBlob.EXPECT().AssetDescriptor().Return(secondAsset).AnyTimes()
				mockBlob.EXPECT().Open().Return(secondAssetBlob.Open())

				firstAsset.Sha256 = "new-unknown-sha"
				err := subject.Write(baseImage)
				h.AssertError(t, err, fmt.Sprintf("unknown sha256 asset value %s", secondAssetBlob.AssetDescriptor().Sha256))
			})
		})
	})
}

func assertHasSameAssets(t *testing.T, image *fakes.Image, assetSha256, expectedContents string) {
	t.Helper()

	assetPath := fmt.Sprintf("/cnb/assets/%s", assetSha256)
	layerTar, err := image.FindLayerWithPath(assetPath)
	h.AssertNil(t, err)

	h.AssertOnTarEntry(t, layerTar, assetPath, h.ContentEquals(expectedContents))
}
