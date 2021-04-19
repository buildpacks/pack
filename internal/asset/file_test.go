package asset_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/asset"
	"github.com/buildpacks/pack/internal/asset/fakes"
	"github.com/buildpacks/pack/internal/asset/testmocks"
	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestAssetCacheFile(t *testing.T) {
	spec.Run(t, "File", testAssetCacheFile, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAssetCacheFile(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpFile  *os.File
		rawImage v1.Image

		mockController  *gomock.Controller
		mockLayerWriter *testmocks.MockLayerWriter

		assert  = h.NewAssertionManager(t)
		subject *asset.File
	)
	it.Before(func() {
		mockController = gomock.NewController(t)
		mockLayerWriter = testmocks.NewMockLayerWriter(mockController)

		var err error
		tmpFile, err = ioutil.TempFile("", "test-asset-cache-file")
		assert.Nil(err)

		rawImage = empty.Image

		subject = asset.NewFile(tmpFile.Name(), "", rawImage, mockLayerWriter)
	})

	when("#AddAssetBlobs", func() {
		var (
			firstAsset = dist.AssetInfo{
				Sha256:  "first-sha256",
				ID:      "first-asset",
				Version: "1.1.1",
				Name:    "First AssetInfo",
				Stacks:  []string{"stack1", "stack2"},
			}
			secondAsset = dist.AssetInfo{
				Sha256:  "second-sha256",
				ID:      "second-asset",
				Version: "2.2.2",
				Name:    "Second AssetInfo",
				Stacks:  []string{"stack1", "stack2"},
			}

			firstAssetBlob  asset.Blob
			secondAssetBlob asset.Blob
		)
		it.Before(func() {
			firstAssetBlob = fakes.NewFakeAssetBlob("first layer contents", firstAsset)
			secondAssetBlob = fakes.NewFakeAssetBlob("second layer contents", secondAsset)
		})
		it("adds asset layers to the LayerWriter", func() {
			mockLayerWriter.EXPECT().AddAssetBlobs(firstAssetBlob, secondAssetBlob)
			subject.AddAssetBlobs(firstAssetBlob, secondAssetBlob)
		})
	})

	// This is difficult to test due to the underlying usage of v1.Images
	// these are inaccessible and replaced rather than mutated over their lifetimes...
	when("#Save", func() {
		var tmpDir string
		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "test-asset-cache-file")
			assert.Nil(err)
		})
		it.After(func() {
			os.RemoveAll(tmpDir)
		})
		when("OS is linux", func() {
			it.Before(func() {
				assert.Succeeds(subject.SetOS("linux"))
			})
			it("uses LayerWriter to write assets to image", func() {
				mockLayerWriter.EXPECT().Open()
				mockLayerWriter.EXPECT().Write(subject)
				mockLayerWriter.EXPECT().Close()

				assert.Succeeds(subject.Save(filepath.Join(tmpDir, "first-name"), filepath.Join(tmpDir, "second-name")))
			})
		})
		when("OS is windows", func() {
			it.Before(func() {
				assert.Succeeds(subject.SetOS("windows"))
			})
			it("uses LayerWriter to write assets to image", func() {
				mockLayerWriter.EXPECT().Open()
				mockLayerWriter.EXPECT().Write(subject)
				mockLayerWriter.EXPECT().Close()

				assert.Succeeds(subject.Save(filepath.Join(tmpDir, "first-name"), filepath.Join(tmpDir, "second-name")))
			})
		})
	})

	when("error cases", func() {
		when("writing LayerWriter fails to Write", func() {
			it("errors with a helpful message", func() {
				assert.Succeeds(subject.SetOS("linux"))
				mockLayerWriter.EXPECT().Open()
				mockLayerWriter.EXPECT().Write(subject).Return(errors.New("error writing asset layers"))
				mockLayerWriter.EXPECT().Close()

				err := subject.Save()
				assert.ErrorContains(err, "unable to write asset layers to file")
			})
		})
		when("unable to open writer", func() {
			it("errors with a helpful message", func() {
				assert.Succeeds(subject.SetOS("linux"))
				mockLayerWriter.EXPECT().Open().Return(errors.New("open writer error"))

				err := subject.Save()
				assert.ErrorContains(err, "unable to open asset writer")
			})
		})
	})
}
