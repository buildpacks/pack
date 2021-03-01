package pack_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	"github.com/buildpacks/pack/pkg/archive"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
)

func TestCreateAssetCacheCommand(t *testing.T) {
	spec.Run(t, "CreateAssetCacheCommand", testCreateAssetCacheCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateAssetCacheCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		client           *pack.Client
		assert           = h.NewAssertionManager(t)
		logger           logging.Logger
		mockController   *gomock.Controller
		mockDownloader   *testmocks.MockDownloader
		mockImageFactory *testmocks.MockImageFactory
		mockImageFetcher *testmocks.MockImageFetcher
		mockDockerClient *testmocks.MockCommonAPIClient
		fakeImage        *fakes.Image
		out              bytes.Buffer
		tmpDir           string
	)
	it.Before(func() {
		var err error
		logger = ilogging.NewLogWithWriters(&out, &out, ilogging.WithVerbose())
		mockController = gomock.NewController(t)
		mockDownloader = testmocks.NewMockDownloader(mockController)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)
		mockImageFactory = testmocks.NewMockImageFactory(mockController)
		mockDockerClient = testmocks.NewMockCommonAPIClient(mockController)
		client, err = pack.NewClient(
			pack.WithLogger(logger),
			pack.WithDownloader(mockDownloader),
			pack.WithImageFactory(mockImageFactory),
			pack.WithFetcher(mockImageFetcher),
			pack.WithDockerClient(mockDockerClient),
		)
		assert.Nil(err)

		tmpDir, err = ioutil.TempDir("", "create-asset-cache-command-test")
		assert.Nil(err)
	})
	when("#CreateAssetCache", func() {
		when("using a local buildpackage", func() {
			var (
				firstAssetBlob  blob.Blob
				secondAssetBlob blob.Blob
			)

			it.Before(func() {
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
			it("succeeds", func() {
				imageName := "test-cache-image"
				imgRef, err := name.NewTag(imageName)
				assert.Nil(err)

				fakeImage = fakes.NewImage(imageName, "somesha256", imgRef)

				mockImageFactory.EXPECT().NewImage(imageName, true).Return(fakeImage, nil)
				mockDownloader.EXPECT().Download(gomock.Any(), "https://first-asset-uri", gomock.Any()).Do(func(_ ...interface{}) {
					time.Sleep(2 * time.Second)
				}).Return(firstAssetBlob, nil)
				mockDownloader.EXPECT().Download(gomock.Any(), "https://first-asset-replace-uri", gomock.Any()).Return(firstAssetBlob, nil)
				mockDownloader.EXPECT().Download(gomock.Any(), "https://second-asset-uri", gomock.Any()).Return(secondAssetBlob, nil)

				assert.Succeeds(client.CreateAssetCache(context.Background(), pack.CreateAssetCacheOptions{
					ImageName: imageName,
					Assets: []dist.Asset{
						{
							ID:      "first-asset",
							Name:    "First Asset",
							Sha256:  "first-sha256",
							Stacks:  []string{"io.buildpacks.stacks.bionic"},
							URI:     "https://first-asset-uri",
							Version: "1.2.3",
						},
						{
							ID:      "first-asset-replace",
							Name:    "First Asset Replace",
							Sha256:  "first-sha256",
							Stacks:  []string{"io.buildpacks.stacks.bionic"},
							URI:     "https://first-asset-replace-uri",
							Version: "1.2.3",
						},
						{
							ID:      "second-asset",
							Name:    "Second Asset",
							Sha256:  "second-sha256",
							Stacks:  []string{"io.buildpacks.stacks.bionic"},
							URI:     "https://second-asset-uri",
							Version: "4.5.6",
						},
						{
							ID:      "third-asset",
							Name:    "Third Asset",
							Sha256:  "third-sha256",
							Stacks:  []string{"io.buildpacks.stacks.bionic"},
							Version: "7.8.9",
						},
					},
					OS: "linux",
				}))

				assert.Equal(fakeImage.IsSaved(), true)

				// validate that we added layers
				assert.Equal(fakeImage.NumberOfAddedLayers(), 2)

				//validate layers metadata
				layersLabel, err := fakeImage.Label(dist.AssetCacheLayersLabel)
				assert.Nil(err)

				var assetMap dist.AssetMap
				assert.Succeeds(json.NewDecoder(strings.NewReader(layersLabel)).Decode(&assetMap))
				assert.Equal(assetMap, dist.AssetMap{
					"first-sha256": dist.AssetValue{
						ID:          "first-asset-replace",
						Name:        "First Asset Replace",
						LayerDiffID: "sha256:268dd0ebfea28592faa58771c467a3ad1a0f169b10a2f575f3d1080bab5a06d2",
						Stacks:      []string{"io.buildpacks.stacks.bionic"},
						URI:         "https://first-asset-replace-uri",
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

			when("publish", func() {
				it("creates a remote asset cache image", func() {
					imageName := "test-publish-cache-image"
					imgRef, err := name.NewTag(imageName)
					assert.Nil(err)

					fakeImage = fakes.NewImage(imageName, "somesha256", imgRef)

					mockImageFactory.EXPECT().NewImage(imageName, false).Return(fakeImage, nil)

					assert.Succeeds(client.CreateAssetCache(context.Background(), pack.CreateAssetCacheOptions{
						ImageName: imageName,
						Assets:    []dist.Asset{},
						Publish:   true,
						OS:        "linux",
					}))

					assert.Equal(fakeImage.IsSaved(), true)
				})
			})

			when("windows", func() {
				it("creates a windows cache image", func() {
					imageName := "test-windwos-cache-image"
					imgRef, err := name.NewTag(imageName)
					assert.Nil(err)

					fakeImage = fakes.NewImage(imageName, "somesha256", imgRef)

					mockImageFactory.EXPECT().NewImage(imageName, false).Return(fakeImage, nil)

					assert.Succeeds(client.CreateAssetCache(context.Background(), pack.CreateAssetCacheOptions{
						ImageName: imageName,
						Assets:    []dist.Asset{},
						Publish:   true,
						OS:        "windows",
					}))

					assert.Equal(fakeImage.IsSaved(), true)

					// check that windows image is properly set up
					imgOS, err := fakeImage.OS()
					assert.Nil(err)
					assert.Equal(imgOS, "windows")

					assert.Equal(fakeImage.NumberOfAddedLayers(), 1)
				})
			})
		})

		when("failure cases", func() {
			when("unable to create a new image", func() {
				it("fails with an error message", func() {
					imageName := "some-example-image"
					mockImageFactory.EXPECT().NewImage(imageName, true).Return(nil, errors.New("image fetch error"))

					err := client.CreateAssetCache(context.Background(), pack.CreateAssetCacheOptions{
						ImageName: imageName,
						OS:        "linux",
					})

					assert.ErrorContains(err, "unable to create asset cache image:")
				})
			})
			when("asset sha256 doesn't match downloaded artifact sha256", func() {
				it("fails with an error message", func() {
					imageName := "fail-cache-image"
					imgRef, err := name.NewTag(imageName)
					assert.Nil(err)

					fakeImage = fakes.NewImage(imageName, "somesha256", imgRef)

					mockImageFactory.EXPECT().NewImage(imgRef.String(), true).Return(fakeImage, nil)
					mockDownloader.EXPECT().Download(gomock.Any(), "https://first-asset-uri", gomock.Any(), gomock.Any()).Return(nil, errors.New("blob download error"))

					err = client.CreateAssetCache(context.Background(), pack.CreateAssetCacheOptions{
						ImageName: imageName,
						Assets: []dist.Asset{
							{
								ID:      "first-asset",
								Name:    "First Asset",
								Sha256:  "first-sha256",
								Stacks:  []string{"io.buildpacks.stacks.bionic"},
								URI:     "https://first-asset-uri",
								Version: "1.2.3",
							},
						},
						OS: "linux",
					})

					assert.ErrorContains(err, "blob download error")
				})
			})
		})
	})
}
