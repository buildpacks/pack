package commands_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/image"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestCreateAssetCache(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "CreateAssetCache", testCreateAssetCache, spec.Random(), spec.Report(report.Terminal{}))
}

func testCreateAssetCache(t *testing.T, when spec.G, it spec.S) {
	var (
		command          *cobra.Command
		logger           logging.Logger
		outBuf           bytes.Buffer
		mockController   *gomock.Controller
		mockClient       *testmocks.MockPackClient
		buildpackLocator string
		cfg              config.Config
		assert           = h.NewAssertionManager(t)
		apiVersion       = api.MustParse("1.2")

		firstAsset = dist.Asset{
			ID:      "first-asset",
			Name:    "First Asset",
			Sha256:  "first-sha256",
			Stacks:  []string{"io.buildpacks.stacks.bionic"},
			URI:     "https://first-asset-uri",
			Version: "1.2.3",
		}

		secondAsset = dist.Asset{
			ID:      "second-asset",
			Name:    "Second Asset",
			Sha256:  "second-sha256",
			Stacks:  []string{"io.buildpacks.stacks.bionic"},
			URI:     "https://second-asset-uri",
			Version: "4.5.6",
		}

		firstBuildpack = dist.BuildpackInfo{
			ID:      "buildpackA",
			Version: "1.2.3",
		}

		secondBuildpack = dist.BuildpackInfo{
			ID:      "buildpackB",
			Version: "4.5.6",
		}

		buildpackLayers = dist.BuildpackLayers{
			"buildpackA": map[string]dist.BuildpackLayerInfo{
				"1.2.3": dist.BuildpackLayerInfo{
					API:         apiVersion,
					Stacks:      nil,
					Order:       nil,
					LayerDiffID: "",
					Homepage:    "",
					Assets:      []dist.Asset{firstAsset},
				},
			},
			"buildpackB": map[string]dist.BuildpackLayerInfo{
				"4.5.6": dist.BuildpackLayerInfo{
					API:         apiVersion,
					Stacks:      nil,
					Order:       nil,
					LayerDiffID: "",
					Homepage:    "",
					Assets:      []dist.Asset{secondAsset},
				},
			},
		}
	)

	it.Before(func() {
		cfg = config.Config{
			DefaultRegistryName: "default-reg",
			PullPolicy:          "if-not-present",
		}

		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		command = commands.CreateAssetCache(logger, cfg, mockClient)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#CreateAssetCache", func() {
		when("buildpack image", func() {
			when("image-preference = prefer-remote", func() {
				it("looks for remote image first then local image", func() {
					buildpackLocator = "some-image-org/some-image-name:latest"
					daemonValues := []bool{}
					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        false,
						Registry:      "default-reg",
					}).Do(func(_ interface{}) {
						fmt.Println("in do function")
						daemonValues = append(daemonValues, false)
					}).Return(nil, image.ErrNotFound)

					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        true,
						Registry:      "default-reg",
					}).Do(func(_ interface{}) {
						daemonValues = append(daemonValues, true)
					}).Return(&pack.BuildpackInfo{
						BuildpackMetadata: buildpackage.Metadata{},
						Buildpacks:        []dist.BuildpackInfo{firstBuildpack},
						BuildpackLayers:   buildpackLayers,
					}, nil)

					mockClient.EXPECT().CreateAssetCache(gomock.Any(), pack.CreateAssetCacheOptions{
						ImageName: "some/asset-cache",
						Assets:    []dist.Asset{firstAsset},
						OS:        "linux",
					})

					command.SetArgs([]string{
						"some/asset-cache",
						"--buildpack", buildpackLocator,
						"--image-preference", "prefer-remote",
					})

					assert.Nil(command.Execute())

					assert.Equal(daemonValues, []bool{false, true})
				})
			})
			when("image-preference = only-local", func() {
				it("looks up only a local image", func() {
					buildpackLocator = "some-image-org/some-image-name:latest"

					daemonValues := []bool{}
					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        true,
						Registry:      "default-reg",
					}).Do(func(_ interface{}) {
						daemonValues = append(daemonValues, true)
					}).Return(&pack.BuildpackInfo{
						BuildpackMetadata: buildpackage.Metadata{},
						Buildpacks:        []dist.BuildpackInfo{firstBuildpack},
						BuildpackLayers:   buildpackLayers,
					}, nil)

					mockClient.EXPECT().CreateAssetCache(gomock.Any(), pack.CreateAssetCacheOptions{
						ImageName: "some/asset-cache",
						Assets:    []dist.Asset{firstAsset},
						OS:        "linux",
					})

					command.SetArgs([]string{
						"some/asset-cache",
						"--buildpack", buildpackLocator,
						"--image-preference", "only-local",
					})

					assert.Nil(command.Execute())

					assert.Equal(daemonValues, []bool{true})
				})
			})

			when("image-preference = only-remote", func() {
				it("looks up only a remote image", func() {
					buildpackLocator = "some-image-org/some-image-name:latest"

					daemonValues := []bool{}
					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        false,
						Registry:      "default-reg",
					}).Do(func(_ interface{}) {
						daemonValues = append(daemonValues, true)
					}).Return(&pack.BuildpackInfo{
						BuildpackMetadata: buildpackage.Metadata{},
						Buildpacks:        []dist.BuildpackInfo{firstBuildpack},
						BuildpackLayers:   buildpackLayers,
					}, nil)

					mockClient.EXPECT().CreateAssetCache(gomock.Any(), pack.CreateAssetCacheOptions{
						ImageName: "some/asset-cache",
						Assets:    []dist.Asset{firstAsset},
						OS:        "linux",
					})

					command.SetArgs([]string{
						"some/asset-cache",
						"--buildpack", buildpackLocator,
						"--image-preference", "only-remote",
					})

					assert.Nil(command.Execute())

					assert.Equal(daemonValues, []bool{true})
				})
			})

			when("image-preference = prefer-local", func() {
				it("looks for local image first then remote image", func() {
					buildpackLocator = "some-image-org/some-image-name:latest"
					daemonValues := []bool{}
					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        true,
						Registry:      "default-reg",
					}).Do(func(_ interface{}) {
						daemonValues = append(daemonValues, true)
					}).Return(nil, image.ErrNotFound)

					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        false,
						Registry:      "default-reg",
					}).Do(func(_ interface{}) {
						daemonValues = append(daemonValues, false)
					}).Return(&pack.BuildpackInfo{
						BuildpackMetadata: buildpackage.Metadata{},
						Buildpacks:        []dist.BuildpackInfo{firstBuildpack},
						BuildpackLayers:   buildpackLayers,
					}, nil)

					mockClient.EXPECT().CreateAssetCache(gomock.Any(), pack.CreateAssetCacheOptions{
						ImageName: "some/asset-cache",
						Assets:    []dist.Asset{firstAsset},
						OS:        "linux",
					})

					command.SetArgs([]string{
						"some/asset-cache",
						"--buildpack", buildpackLocator,
						"--image-preference", "prefer-local",
					})

					assert.Nil(command.Execute())

					assert.Equal(daemonValues, []bool{true, false})
				})
			})
		})

		when("buildpack URI", func() {
			when("local path", func() {
				when("", func() {
					it.Before(func() {
						buildpackLocator = "some-local-locator"
					})
					it("succeeds and creates a local asset cache", func() {
						command.SetArgs([]string{
							"some/asset-cache",
							"--buildpack", buildpackLocator,
						})

						mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
							BuildpackName: buildpackLocator,
							Daemon:        true,
							Registry:      "default-reg",
						}).Return(
							&pack.BuildpackInfo{
								BuildpackMetadata: buildpackage.Metadata{},
								Buildpacks:        []dist.BuildpackInfo{firstBuildpack, secondBuildpack},
								BuildpackLayers:   buildpackLayers,
							},
							nil,
						)
						mockClient.EXPECT().CreateAssetCache(gomock.Any(), pack.CreateAssetCacheOptions{
							ImageName: "some/asset-cache",
							Assets:    []dist.Asset{firstAsset, secondAsset},
							OS:        "linux",
						})

						assert.Nil(command.Execute())
					})
				})
			})
		})

		when("buildpack on registry", func() {
			it.Before(func() {
				buildpackLocator = "urn:cnb:registry:example/buildpack"
			})
			when("passing in registry", func() {
				it("over-rides default registry", func() {
					command.SetArgs([]string{
						"some/asset-cache",
						"--buildpack", buildpackLocator,
						"--buildpack-registry", "some-other-registry",
					})

					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        true,
						Registry:      "some-other-registry",
					}).Return(
						&pack.BuildpackInfo{
							BuildpackMetadata: buildpackage.Metadata{},
							Buildpacks:        []dist.BuildpackInfo{firstBuildpack, secondBuildpack},
							BuildpackLayers:   buildpackLayers,
						},
						nil,
					)
					mockClient.EXPECT().CreateAssetCache(gomock.Any(), pack.CreateAssetCacheOptions{
						ImageName: "some/asset-cache",
						Assets:    []dist.Asset{firstAsset, secondAsset},
						OS:        "linux",
					})

					assert.Nil(command.Execute())
				})
			})
		})

		when("--publish", func() {
			it.Before(func() {
				buildpackLocator = "some-image-org/some-image-name:latest"
			})
			it("publishes resulting cache image", func() {
				command.SetArgs([]string{
					"some/asset-cache",
					"--buildpack", buildpackLocator,
					"--publish",
				})

				mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
					BuildpackName: buildpackLocator,
					Daemon:        true,
					Registry:      "default-reg",
				}).Return(
					&pack.BuildpackInfo{
						BuildpackMetadata: buildpackage.Metadata{},
						Buildpacks:        []dist.BuildpackInfo{firstBuildpack, secondBuildpack},
						BuildpackLayers:   buildpackLayers,
					},
					nil,
				)
				mockClient.EXPECT().CreateAssetCache(gomock.Any(), pack.CreateAssetCacheOptions{
					ImageName: "some/asset-cache",
					Assets:    []dist.Asset{firstAsset, secondAsset},
					Publish:   true,
					OS:        "linux",
				})

				assert.Succeeds(command.Execute())
			})
		})

		when("--os windows", func() {
			it("succeeds", func() {
				buildpackLocator = "some-windows-buildpack"
				command.SetArgs([]string{
					"some/asset-cache",
					"--buildpack", buildpackLocator,
					"--os", "windows",
				})

				mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
					BuildpackName: buildpackLocator,
					Daemon:        true,
					Registry:      "default-reg",
				}).Return(
					&pack.BuildpackInfo{
						BuildpackMetadata: buildpackage.Metadata{},
						Buildpacks:        []dist.BuildpackInfo{firstBuildpack, secondBuildpack},
						BuildpackLayers:   buildpackLayers,
					},
					nil,
				)
				mockClient.EXPECT().CreateAssetCache(gomock.Any(), pack.CreateAssetCacheOptions{
					ImageName: "some/asset-cache",
					Assets:    []dist.Asset{firstAsset, secondAsset},
					OS:        "windows",
				})

				assert.Succeeds(command.Execute())
			})
		})

		when("failure cases", func() {
			when("invalid asset cache image name is used", func() {
				it("errors with a informative message", func() {
					command.SetArgs([]string{
						"::::",
						"--buildpack", "some-locator",
					})
					err := command.Execute()
					assert.ErrorContains(err, `unable to parse cache image name`)
				})
			})
			when("no --buildpack flag is specified", func() {
				it("errors with a informative message", func() {
					command.SetArgs([]string{"error/asset-cache-error"})
					err := command.Execute()
					assert.ErrorContains(err, "must specify a buildpack locator using the --buildpack flag")
				})
			})
			when("unknown image-preference", func() {
				it("errors with informative message", func() {
					command.SetArgs([]string{
						"some/asset-cache",
						"--buildpack", "some-locator",
						"--image-preference", "boopdoop",
					})

					err := command.Execute()
					assert.ErrorContains(err, `unknown image preference: "boopdoop"`)
				})
			})
			when("unknown os option", func() {
				it("errors with an informative message", func() {
					command.SetArgs([]string{
						"some/asset-cache",
						"--buildpack", "some-locator",
						"--os", "schwindodos",
					})

					err := command.Execute()
					assert.ErrorContains(err, `unknown os type: "schwindodos"`)
				})
			})

			when("no buildpack is found", func() {
				it("errors with informative message", func() {
					buildpackLocator = "some-image-org/some-non-exitant-image:latest"
					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        true,
						Registry:      "default-reg",
					}).Return(nil, errors.New("does not exist"))

					command.SetArgs([]string{
						"some/asset-cache",
						"--buildpack", buildpackLocator,
					})

					err := command.Execute()
					assert.ErrorContains(err, "buildpack not found")
				})
			})
			when("no buildpack image is found", func() {
				it("errors with informative message", func() {
					buildpackLocator = "some-image-org/some-non-exitant-image:latest"
					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        true,
						Registry:      "default-reg",
					}).Return(nil, image.ErrNotFound)

					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        false,
						Registry:      "default-reg",
					}).Return(nil, image.ErrNotFound)

					command.SetArgs([]string{
						"some/asset-cache",
						"--buildpack", buildpackLocator,
					})

					err := command.Execute()
					assert.ErrorContains(err, "buildpack not found")
				})
			})
			when("CreateAssetCache fails", func() {
				it("errors with informative message", func() {
					buildpackLocator = "some-image-org/some-non-exitant-image:latest"

					command.SetArgs([]string{
						"some/asset-cache",
						"--buildpack", buildpackLocator,
						"--publish",
					})

					mockClient.EXPECT().InspectBuildpack(pack.InspectBuildpackOptions{
						BuildpackName: buildpackLocator,
						Daemon:        true,
						Registry:      "default-reg",
					}).Return(
						&pack.BuildpackInfo{
							BuildpackMetadata: buildpackage.Metadata{},
							Buildpacks:        []dist.BuildpackInfo{firstBuildpack, secondBuildpack},
							BuildpackLayers:   buildpackLayers,
						},
						nil,
					)
					mockClient.EXPECT().CreateAssetCache(gomock.Any(), pack.CreateAssetCacheOptions{
						ImageName: "some/asset-cache",
						Assets:    []dist.Asset{firstAsset, secondAsset},
						Publish:   true,
						OS:        "linux",
					}).Return(errors.New("asset-cache-creation-error"))

					err := command.Execute()
					assert.ErrorContains(err, "error, unable to create asset cache")
					assert.ErrorContains(err, "asset-cache-creation-error")
				})
			})
		})
	})
}
