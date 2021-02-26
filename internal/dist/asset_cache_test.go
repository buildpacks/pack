package dist_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
			blobMap := dist.BlobMap{
				"first-sha256": dist.NewBlobAssetPair(firstAssetBlob, dist.AssetValue{
					ID:      "first-asset",
					Name:    "First Asset",
					Stacks:  []string{"io.buildpacks.stacks.bionic"},
					URI:     "https://first-asset-uri",
					Version: "1.2.3",
				}),
				"second-sha256": dist.NewBlobAssetPair(secondAssetBlob, dist.AssetValue{
					ID:      "second-asset",
					Name:    "Second Asset",
					Stacks:  []string{"io.buildpacks.stacks.bionic"},
					URI:     "https://second-asset-uri",
					Version: "4.5.6",
				}),
				"third-sha256": dist.NewBlobAssetPair(nil, dist.AssetValue{
					ID:      "third-asset",
					Name:    "Third Asset",
					Stacks:  []string{"io.buildpacks.stacks.bionic"},
					URI:     "https://third-asset-uri",
					Version: "7.8.9",
				}),
			}

			tarWriterFactory, err := layer.NewWriterFactory("linux")
			assert.Nil(err)

			subject := dist.NewAssetCacheImage(fakeImage, blobMap, tarWriterFactory)
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
			it("saves a windows based cache image", func() {
				tag, err := name.NewTag("windows-asset-cache-image")
				assert.Nil(err)
				fakeImage := fakes.NewImage("windows-asset-cache-image", "some-top-level-sha", tag)

				blobMap := dist.BlobMap{
					"first-sha256": dist.NewBlobAssetPair(firstAssetBlob, dist.AssetValue{
						ID:      "first-asset",
						Name:    "First Asset",
						Stacks:  []string{"io.buildpacks.stacks.windows"},
						URI:     "https://first-asset-uri",
						Version: "1.2.3",
					}),
				}

				windowsWriterFactory, err := layer.NewWriterFactory("windows")
				assert.Nil(err)

				subject := dist.NewAssetCacheImage(fakeImage, blobMap, windowsWriterFactory)

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
				assert.Equal(fakeImage.NumberOfAddedLayers(), 1)

				firstLayerReader, err := fakeImage.GetLayer("sha256:8896d17ea5e9fd048fc77b0f3b0423ea95ea5137536e623e839b93d90216b0bd")
				assert.Nil(err)

				_, b, err := archive.ReadTarEntry(firstLayerReader, "Files/cnb/assets/first-sha256")
				assert.Nil(err)
				assert.Contains(string(b), "first-asset-blob-contents.")
			})
		})
	})
	when("failure cases", func() {
		when("unable to read asset blob", func() {
			it("returns an error message", func() {
				invalidBlob := blob.NewBlob(":::::")
				tag, err := name.NewTag("asset-cache-image")
				assert.Nil(err)
				fakeImage := fakes.NewImage("asset-cache-image", "some-top-level-sha", tag)

				blobMap := dist.BlobMap{
					"first-sha256": dist.NewBlobAssetPair(invalidBlob, dist.AssetValue{
						ID:      "first-asset",
						Name:    "First Asset",
						Stacks:  []string{"io.buildpacks.stacks.bionic"},
						URI:     "https://first-asset-uri",
						Version: "1.2.3",
					}),
				}

				tarWriterFactory, err := layer.NewWriterFactory("linux")
				assert.Nil(err)

				subject := dist.NewAssetCacheImage(fakeImage, blobMap, tarWriterFactory)
				err = subject.Save()

				assert.ErrorContains(err, `unable to open blob for asset "first-sha256"`)
			})
		})
	})
}
