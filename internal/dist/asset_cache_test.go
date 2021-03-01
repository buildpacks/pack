package dist_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/testmocks"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/layer"
	"github.com/buildpacks/pack/pkg/archive"
	"github.com/buildpacks/pack/testhelpers"
)

func TestAssetCache(t *testing.T) {
	spec.Run(t, "TestAssetCache", testAssetCache)
}

func testAssetCache(t *testing.T, when spec.G, it spec.S) {
	var (
		assert          = testhelpers.NewAssertionManager(t)
		tmpDir          string
		firstAssetBlob  blob.Blob
		secondAssetBlob blob.Blob
	)
	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "create-asset-cache-command-test")
		assert.Nil(err)

		firstAssetBlobPath := filepath.Join(tmpDir, "firstAssetBlob")
		assert.Succeeds(ioutil.WriteFile(firstAssetBlobPath, []byte(`
first-asset-blob-contents.
`), os.ModePerm))
		firstAssetBlob = blob.NewBlob(firstAssetBlobPath)

		secondAssetBlobPath := filepath.Join(tmpDir, "secondAssetBlob")
		assert.Succeeds(ioutil.WriteFile(secondAssetBlobPath, []byte(`
second-asset-blob-contents.
`), os.ModePerm))
		secondAssetBlob = blob.NewBlob(secondAssetBlobPath)
	})

	when("#Save", func() {
		it("saves the cache", func() {
			tag, err := name.NewTag("asset-cache-image")
			assert.Nil(err)
			fakeImage := fakes.NewImage("asset-cache-image", "some-top-level-sha", tag)
			pairs := []dist.BlobAssetPair{
				{firstAssetBlob, dist.Asset{
					Sha256:  "first-sha256",
					ID:      "first-asset",
					Name:    "First Asset",
					Stacks:  []string{"io.buildpacks.stacks.bionic"},
					URI:     "https://first-asset-uri",
					Version: "1.2.3",
				}},
				{secondAssetBlob, dist.Asset{
					Sha256:  "second-sha256",
					ID:      "second-asset",
					Name:    "Second Asset",
					Stacks:  []string{"io.buildpacks.stacks.bionic"},
					URI:     "https://second-asset-uri",
					Version: "4.5.6",
				}},
				{
					nil, dist.Asset{
						Sha256:  "third-sha256",
						ID:      "third-asset",
						Name:    "Third Asset",
						Stacks:  []string{"io.buildpacks.stacks.bionic"},
						URI:     "https://third-asset-uri",
						Version: "7.8.9",
					}},
			}

			tarWriterFactory, err := layer.NewWriterFactory("linux")
			assert.Nil(err)

			subject, err := dist.NewAssetCacheImage(fakeImage, tarWriterFactory)
			assert.Nil(err)

			subject.AddAssetLayers(pairs...)

			assert.Succeeds(subject.Save())

			assert.Equal(fakeImage.IsSaved(), true)

			// validate that we added layers
			assert.Equal(fakeImage.NumberOfAddedLayers(), 2)

			//validate layers metadata
			layersLabel, err := fakeImage.Label(dist.AssetCacheLayersLabel)
			assert.Nil(err)

			var assetMetadata dist.AssetMap
			assert.Succeeds(json.NewDecoder(strings.NewReader(layersLabel)).Decode(&assetMetadata))
			assert.Equal(assetMetadata, dist.AssetMap{
				"first-sha256": dist.AssetValue{
					ID:          "first-asset",
					Name:        "First Asset",
					LayerDiffID: "sha256:268dd0ebfea28592faa58771c467a3ad1a0f169b10a2f575f3d1080bab5a06d2",
					Stacks:      []string{"io.buildpacks.stacks.bionic"},
					URI:         "https://first-asset-uri",
					Version:     "1.2.3",
				}, "second-sha256": dist.AssetValue{
					ID:          "second-asset",
					Name:        "Second Asset",
					LayerDiffID: "sha256:02698069f5d4415f04fb0037428f99b50d0d3dd9a59836dde261b7ef17823049",
					Stacks:      []string{"io.buildpacks.stacks.bionic"},
					URI:         "https://second-asset-uri",
					Version:     "4.5.6",
				},
			})

			firstLayerName, err := fakeImage.FindLayerWithPath("/cnb/assets/first-sha256")
			assert.Nil(err)
			assert.NotEqual(firstLayerName, "")

			firstLayerReader, err := fakeImage.GetLayer("sha256:268dd0ebfea28592faa58771c467a3ad1a0f169b10a2f575f3d1080bab5a06d2")
			assert.Nil(err)

			_, b, err := archive.ReadTarEntry(firstLayerReader, "/cnb/assets/first-sha256")
			assert.Nil(err)
			assert.Contains(string(b), "first-asset-blob-contents.")

			secondLayerName, err := fakeImage.FindLayerWithPath("/cnb/assets/second-sha256")
			assert.Nil(err)

			assert.NotEqual(secondLayerName, "")

			secondLayerReader, err := fakeImage.GetLayer("sha256:02698069f5d4415f04fb0037428f99b50d0d3dd9a59836dde261b7ef17823049")
			assert.Nil(err)

			_, b, err = archive.ReadTarEntry(secondLayerReader, "/cnb/assets/second-sha256")
			assert.Nil(err)
			assert.Contains(string(b), "second-asset-blob-contents.")
		})

		when("windows", func() {
			it.Before(func() {
				it.Before(func() {
					var err error
					tmpDir, err = ioutil.TempDir("", "create-asset-cache-command-test")
					assert.Nil(err)

					firstAssetBlobPath := filepath.Join(tmpDir, "firstAssetBlob")
					assert.Succeeds(ioutil.WriteFile(firstAssetBlobPath, []byte(`
			first-asset-blob-contents.
			`), os.ModePerm))
					firstAssetBlob = blob.NewBlob(firstAssetBlobPath)
				})
			})
			it("saves a windows based cache image, with an extra windows base layer", func() {
				tag, err := name.NewTag("windows-asset-cache-image")
				assert.Nil(err)
				fakeImage := fakes.NewImage("windows-asset-cache-image", "some-top-level-sha", tag)
				assert.Succeeds(fakeImage.SetOS(config.WindowsOS))

				pairs := []dist.BlobAssetPair{
					{firstAssetBlob, dist.Asset{
						Sha256:  "first-sha256",
						ID:      "first-asset",
						Name:    "First Asset",
						Stacks:  []string{"io.buildpacks.stacks.windows"},
						URI:     "https://first-asset-uri",
						Version: "1.2.3",
					}},
				}

				windowsWriterFactory, err := layer.NewWriterFactory("windows")
				assert.Nil(err)

				subject, err := dist.NewAssetCacheImage(fakeImage, windowsWriterFactory)
				assert.Nil(err)

				subject.AddAssetLayers(pairs...)

				assert.Succeeds(subject.Save())
				assert.Equal(fakeImage.IsSaved(), true)

				layersLabel, err := fakeImage.Label(dist.AssetCacheLayersLabel)
				assert.Nil(err)

				var assetMetadata dist.AssetMap
				assert.Succeeds(json.NewDecoder(strings.NewReader(layersLabel)).Decode(&assetMetadata))
				assert.Equal(assetMetadata, dist.AssetMap{
					"first-sha256": dist.AssetValue{
						ID:          "first-asset",
						Name:        "First Asset",
						LayerDiffID: "sha256:8896d17ea5e9fd048fc77b0f3b0423ea95ea5137536e623e839b93d90216b0bd",
						Stacks:      []string{"io.buildpacks.stacks.windows"},
						URI:         "https://first-asset-uri",
						Version:     "1.2.3",
					},
				})

				fmt.Printf("%#v\n", fakeImage)
				assert.Equal(fakeImage.NumberOfAddedLayers(), 2)

				firstLayerReader, err := fakeImage.GetLayer("sha256:8896d17ea5e9fd048fc77b0f3b0423ea95ea5137536e623e839b93d90216b0bd")
				assert.Nil(err)

				_, b, err := archive.ReadTarEntry(firstLayerReader, "Files/cnb/assets/first-sha256")
				assert.Nil(err)
				assert.Contains(string(b), "first-asset-blob-contents.")
			})
		})
	})
	when("failure cases", func() {
		var (
			mockController *gomock.Controller
			mockImage      *testmocks.MockImage
		)
		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImage = testmocks.NewMockImage(mockController)
		})

		when("unable to read asset blob", func() {
			it("returns an error message", func() {
				invalidBlob := blob.NewBlob(":::::")
				tag, err := name.NewTag("asset-cache-image")
				assert.Nil(err)
				fakeImage := fakes.NewImage("asset-cache-image", "some-top-level-sha", tag)

				pairs := []dist.BlobAssetPair{
					{invalidBlob, dist.Asset{
						Sha256:  "first-sha256",
						ID:      "first-asset",
						Name:    "First Asset",
						Stacks:  []string{"io.buildpacks.stacks.bionic"},
						URI:     "https://first-asset-uri",
						Version: "1.2.3",
					}},
				}

				tarWriterFactory, err := layer.NewWriterFactory("linux")
				assert.Nil(err)

				subject, err := dist.NewAssetCacheImage(fakeImage, tarWriterFactory)
				assert.Nil(err)

				subject.AddAssetLayers(pairs...)
				err = subject.Save()

				assert.ErrorContains(err, `unable to open blob for asset "first-sha256"`)
			})
		})

		when("error getting image OS", func() {
			it("returns an error message", func() {
				mockImage.EXPECT().OS().Return("", errors.New("error getting image os"))

				tarWriterFactory, err := layer.NewWriterFactory("linux")
				assert.Nil(err)

				subject, err := dist.NewAssetCacheImage(mockImage, tarWriterFactory)
				assert.Nil(err)

				err = subject.Save("some-name")
				assert.ErrorContains(err, "unable to get asset cache image os: error getting image os")
			})
		})

		when("writing windows base layer", func() {
			it("return an error message", func() {
				mockImage.EXPECT().OS().Return(config.WindowsOS, nil)
				windowsBaseLayerSha256 := "sha256:067833dadeca9180bb71a211248cbc0f6ada05499bfdef07dfb04e2eb96e7c82"
				mockImage.EXPECT().AddLayerWithDiffID(gomock.Any(), windowsBaseLayerSha256).Return(errors.New("the error"))

				tarWriterFactory, err := layer.NewWriterFactory(config.WindowsOS)
				assert.Nil(err)

				subject, err := dist.NewAssetCacheImage(mockImage, tarWriterFactory)
				assert.Nil(err)

				err = subject.Save("some-name")
				assert.ErrorContains(err, "unable to write windows base layer: the error")
			})
		})

		when("unable to add assets to cache image", func() {
			var assetBlob blob.Blob
			it.Before(func() {
				assetBlobPath := filepath.Join(tmpDir, "firstAssetBlob")
				assert.Succeeds(ioutil.WriteFile(assetBlobPath, []byte(`
first-asset-blob-contents.
`), os.ModePerm))
				assetBlob = blob.NewBlob(assetBlobPath)
			})
			it("returns an error message", func() {
				mockImage.EXPECT().OS().Return(config.LinuxOS, nil)
				mockImage.EXPECT().AddLayer(gomock.Any()).Return(errors.New("the error"))

				pair := dist.BlobAssetPair{assetBlob, dist.Asset{
					Sha256:  "first-sha256",
					ID:      "first-asset",
					Name:    "First Asset",
					Stacks:  []string{"io.buildpacks.stacks.windows"},
					URI:     "https://first-asset-uri",
					Version: "1.2.3",
				}}

				tarWriterFactory, err := layer.NewWriterFactory(config.LinuxOS)
				assert.Nil(err)

				subject, err := dist.NewAssetCacheImage(mockImage, tarWriterFactory)
				assert.Nil(err)

				subject.AddAssetLayers(pair)

				err = subject.Save("some-name")
				assert.ErrorContains(err, `unable to add asset blob "first-sha256" to layer`)
			})
		})
		when("unable to set asset label", func() {
			it("returns an error message", func() {
				mockImage.EXPECT().OS().Return(config.LinuxOS, nil)
				mockImage.EXPECT().SetLabel(dist.AssetCacheLayersLabel, gomock.Any()).Return(errors.New("the error"))

				tarWriterFactory, err := layer.NewWriterFactory(config.LinuxOS)
				assert.Nil(err)

				subject, err := dist.NewAssetCacheImage(mockImage, tarWriterFactory)
				assert.Nil(err)

				err = subject.Save("some-name")
				assert.ErrorContains(err, "unable to set asset cache label")
			})
		})

		when("underlying image save fails", func() {
			it("returns an error message", func() {
				imageName := "some-image-name"
				mockImage.EXPECT().OS().Return(config.LinuxOS, nil)
				mockImage.EXPECT().SetLabel(dist.AssetCacheLayersLabel, gomock.Any()).Return(nil)
				mockImage.EXPECT().Save(imageName).Return(errors.New("save error"))

				tarWriterFactory, err := layer.NewWriterFactory(config.LinuxOS)
				assert.Nil(err)

				subject, err := dist.NewAssetCacheImage(mockImage, tarWriterFactory)
				assert.Nil(err)

				err = subject.Save(imageName)
				assert.ErrorContains(err, "save error")
			})
		})
	})
}
