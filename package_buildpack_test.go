package pack_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/lifecycle/api"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/buildpackage"
	cfg "github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/internal/paths"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
)

func TestPackageBuildpack(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "PackageBuildpack", testPackageBuildpack, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPackageBuildpack(t *testing.T, when spec.G, it spec.S) {
	var (
		subject          *pack.Client
		mockController   *gomock.Controller
		mockDownloader   *testmocks.MockDownloader
		mockImageFactory *testmocks.MockImageFactory
		mockImageFetcher *testmocks.MockImageFetcher
		mockDockerClient *testmocks.MockCommonAPIClient
		out              bytes.Buffer
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockDownloader = testmocks.NewMockDownloader(mockController)
		mockImageFactory = testmocks.NewMockImageFactory(mockController)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)
		mockDockerClient = testmocks.NewMockCommonAPIClient(mockController)

		var err error
		subject, err = pack.NewClient(
			pack.WithLogger(logging.NewLogWithWriters(&out, &out)),
			pack.WithDownloader(mockDownloader),
			pack.WithImageFactory(mockImageFactory),
			pack.WithFetcher(mockImageFetcher),
			pack.WithDockerClient(mockDockerClient),
		)
		h.AssertNil(t, err)
	})

	it.After(func() {
		mockController.Finish()
	})

	createBuildpack := func(descriptor dist.BuildpackDescriptor) string {
		bp, err := ifakes.NewFakeBuildpackBlob(descriptor, 0644)
		h.AssertNil(t, err)
		url := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
		mockDownloader.EXPECT().Download(gomock.Any(), url).Return(bp, nil).AnyTimes()
		return url
	}

	when("buildpack has issues", func() {
		when("buildpack has no URI", func() {
			it("should fail", func() {
				err := subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: "Fake-Name",
					Config: pubbldpkg.Config{
						Platform:  dist.Platform{OS: "linux"},
						Buildpack: dist.BuildpackURI{URI: ""},
					},
					Publish: true,
				})
				h.AssertError(t, err, "buildpack URI must be provided")
			})
		})

		when("can't download buildpack", func() {
			it("should fail", func() {
				bpURL := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
				mockDownloader.EXPECT().Download(gomock.Any(), bpURL).Return(nil, image.ErrNotFound).AnyTimes()

				err := subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: "Fake-Name",
					Config: pubbldpkg.Config{
						Platform:  dist.Platform{OS: "linux"},
						Buildpack: dist.BuildpackURI{URI: bpURL},
					},
					Publish: true,
				})
				h.AssertError(t, err, "downloading buildpack")
			})
		})

		when("buildpack isn't a valid buildpack", func() {
			it("should fail", func() {
				fakeBlob := blob.NewBlob(filepath.Join("testdata", "empty-file"))
				bpURL := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
				mockDownloader.EXPECT().Download(gomock.Any(), bpURL).Return(fakeBlob, nil).AnyTimes()

				err := subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: "Fake-Name",
					Config: pubbldpkg.Config{
						Platform:  dist.Platform{OS: "linux"},
						Buildpack: dist.BuildpackURI{URI: bpURL},
					},
					Publish: true,
				})
				h.AssertError(t, err, "creating buildpack")
			})
		})
	})

	when("dependencies have issues", func() {
		when("dependencies include a flawed packaged buildpack file", func() {
			it("should fail", func() {
				dependencyPath := "http://example.com/flawed.file"
				mockDownloader.EXPECT().Download(gomock.Any(), dependencyPath).Return(blob.NewBlob("no-file.txt"), nil).AnyTimes()

				mockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: "linux"}, nil).AnyTimes()

				packageDescriptor := dist.BuildpackDescriptor{
					API:  api.MustParse("0.2"),
					Info: dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
					Order: dist.Order{{
						Group: []dist.BuildpackRef{{
							BuildpackInfo: dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
							Optional:      false,
						}},
					}},
				}

				err := subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: "test",
					Config: pubbldpkg.Config{
						Platform:     dist.Platform{OS: "linux"},
						Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
						Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: dependencyPath}}},
					},
					Publish:    false,
					PullPolicy: pubcfg.PullAlways,
				})

				h.AssertError(t, err, "inspecting buildpack blob")
			})
		})
	})

	when("FormatImage", func() {
		when("simple package for both OS formats (experimental only)", func() {
			it("creates package image based on daemon OS", func() {
				for _, daemonOS := range []string{"linux", "windows"} {
					localMockDockerClient := testmocks.NewMockCommonAPIClient(mockController)
					localMockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: daemonOS}, nil).AnyTimes()

					packClientWithExperimental, err := pack.NewClient(
						pack.WithDockerClient(localMockDockerClient),
						pack.WithDownloader(mockDownloader),
						pack.WithImageFactory(mockImageFactory),
						pack.WithExperimental(true),
					)
					h.AssertNil(t, err)

					fakeImage := fakes.NewImage("basic/package-"+h.RandString(12), "", nil)
					mockImageFactory.EXPECT().NewImage(fakeImage.Name(), true).Return(fakeImage, nil)

					fakeBlob := blob.NewBlob(filepath.Join("testdata", "empty-file"))
					bpURL := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
					mockDownloader.EXPECT().Download(gomock.Any(), bpURL).Return(fakeBlob, nil).AnyTimes()

					h.AssertNil(t, packClientWithExperimental.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Format: pack.FormatImage,
						Name:   fakeImage.Name(),
						Config: pubbldpkg.Config{
							Platform: dist.Platform{OS: daemonOS},
							Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
								API:    api.MustParse("0.2"),
								Info:   dist.BuildpackInfo{ID: "bp.basic", Version: "2.3.4"},
								Stacks: []dist.Stack{{ID: "some.stack.id"}},
							})},
						},
						PullPolicy: pubcfg.PullNever,
					}))

					actualImageOS, err := fakeImage.OS()
					h.AssertNil(t, err)
					h.AssertEq(t, actualImageOS, daemonOS)
				}
			})

			it("fails without experimental on Windows daemons", func() {
				windowsMockDockerClient := testmocks.NewMockCommonAPIClient(mockController)

				packClientWithoutExperimental, err := pack.NewClient(
					pack.WithDockerClient(windowsMockDockerClient),
					pack.WithExperimental(false),
				)
				h.AssertNil(t, err)

				err = packClientWithoutExperimental.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Config: pubbldpkg.Config{
						Platform: dist.Platform{
							OS: "windows",
						},
					},
				})
				h.AssertError(t, err, "Windows buildpackage support is currently experimental.")
			})

			it("fails for mismatched platform and daemon os", func() {
				windowsMockDockerClient := testmocks.NewMockCommonAPIClient(mockController)
				windowsMockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: "windows"}, nil).AnyTimes()

				packClientWithoutExperimental, err := pack.NewClient(
					pack.WithDockerClient(windowsMockDockerClient),
					pack.WithExperimental(false),
				)
				h.AssertNil(t, err)

				err = packClientWithoutExperimental.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Config: pubbldpkg.Config{
						Platform: dist.Platform{
							OS: "linux",
						},
					},
				})

				h.AssertError(t, err, "invalid 'platform.os' specified: DOCKER_OS is 'windows'")
			})
		})

		when("nested package lives in registry", func() {
			var nestedPackage *fakes.Image

			it.Before(func() {
				nestedPackage = fakes.NewImage("nested/package-"+h.RandString(12), "", nil)
				mockImageFactory.EXPECT().NewImage(nestedPackage.Name(), false).Return(nestedPackage, nil)

				mockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: "linux"}, nil).AnyTimes()

				h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: nestedPackage.Name(),
					Config: pubbldpkg.Config{
						Platform: dist.Platform{OS: "linux"},
						Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
							API:    api.MustParse("0.2"),
							Info:   dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
							Stacks: []dist.Stack{{ID: "some.stack.id"}},
						})},
					},
					Publish:    true,
					PullPolicy: pubcfg.PullAlways,
				}))
			})

			shouldFetchNestedPackage := func(demon bool, pull pubcfg.PullPolicy) {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), nestedPackage.Name(), demon, pull).Return(nestedPackage, nil)
			}

			shouldNotFindNestedPackageWhenCallingImageFetcherWith := func(demon bool, pull pubcfg.PullPolicy) {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), nestedPackage.Name(), demon, pull).Return(nil, image.ErrNotFound)
			}

			shouldCreateLocalPackage := func() imgutil.Image {
				img := fakes.NewImage("some/package-"+h.RandString(12), "", nil)
				mockImageFactory.EXPECT().NewImage(img.Name(), true).Return(img, nil)
				return img
			}

			shouldCreateRemotePackage := func() *fakes.Image {
				img := fakes.NewImage("some/package-"+h.RandString(12), "", nil)
				mockImageFactory.EXPECT().NewImage(img.Name(), false).Return(img, nil)
				return img
			}

			when("publish=false and pull-policy=always", func() {
				it("should pull and use local nested package image", func() {
					shouldFetchNestedPackage(true, pubcfg.PullAlways)
					packageImage := shouldCreateLocalPackage()

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packageImage.Name(),
						Config: pubbldpkg.Config{
							Platform: dist.Platform{OS: "linux"},
							Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
								API:  api.MustParse("0.2"),
								Info: dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
								Order: dist.Order{{
									Group: []dist.BuildpackRef{{
										BuildpackInfo: dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
										Optional:      false,
									}},
								}},
							})},
							Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: nestedPackage.Name()}}},
						},
						Publish:    false,
						PullPolicy: pubcfg.PullAlways,
					}))
				})
			})

			when("publish=true and pull-policy=always", func() {
				it("should use remote nested package image", func() {
					shouldFetchNestedPackage(false, pubcfg.PullAlways)
					packageImage := shouldCreateRemotePackage()

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packageImage.Name(),
						Config: pubbldpkg.Config{
							Platform: dist.Platform{OS: "linux"},
							Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
								API:  api.MustParse("0.2"),
								Info: dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
								Order: dist.Order{{
									Group: []dist.BuildpackRef{{
										BuildpackInfo: dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
										Optional:      false,
									}},
								}},
							})},
							Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: nestedPackage.Name()}}},
						},
						Publish:    true,
						PullPolicy: pubcfg.PullAlways,
					}))
				})
			})

			when("publish=true and pull-policy=never", func() {
				it("should push to registry and not pull nested package image", func() {
					shouldFetchNestedPackage(false, pubcfg.PullNever)
					packageImage := shouldCreateRemotePackage()

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packageImage.Name(),
						Config: pubbldpkg.Config{
							Platform: dist.Platform{OS: "linux"},
							Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
								API:  api.MustParse("0.2"),
								Info: dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
								Order: dist.Order{{
									Group: []dist.BuildpackRef{{
										BuildpackInfo: dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
										Optional:      false,
									}},
								}},
							})},
							Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: nestedPackage.Name()}}},
						},
						Publish:    true,
						PullPolicy: pubcfg.PullNever,
					}))
				})
			})

			when("publish=false pull-policy=never and there is no local image", func() {
				it("should fail without trying to retrieve nested image from registry", func() {
					shouldNotFindNestedPackageWhenCallingImageFetcherWith(true, pubcfg.PullNever)

					h.AssertError(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: "some/package",
						Config: pubbldpkg.Config{
							Platform: dist.Platform{OS: "linux"},
							Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
								API:    api.MustParse("0.2"),
								Info:   dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
								Stacks: []dist.Stack{{ID: "some.stack.id"}},
							})},
							Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: nestedPackage.Name()}}},
						},
						Publish:    false,
						PullPolicy: pubcfg.PullNever,
					}), "not found")
				})
			})
		})

		when("nested package is not a valid package", func() {
			it("should error", func() {
				notPackageImage := fakes.NewImage("not/package", "", nil)
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), notPackageImage.Name(), true, pubcfg.PullAlways).Return(notPackageImage, nil)

				mockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: "linux"}, nil).AnyTimes()

				h.AssertError(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: "some/package",
					Config: pubbldpkg.Config{
						Platform: dist.Platform{OS: "linux"},
						Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
							API:    api.MustParse("0.2"),
							Info:   dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
							Stacks: []dist.Stack{{ID: "some.stack.id"}},
						})},
						Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: notPackageImage.Name()}}},
					},
					Publish:    false,
					PullPolicy: pubcfg.PullAlways,
				}), "extracting buildpacks from 'not/package': could not find label 'io.buildpacks.buildpackage.metadata'")
			})
		})
	})

	when("FormatFile", func() {
		when("simple package for both OS formats (experimental only)", func() {
			it("creates package image in either OS format", func() {
				tmpDir, err := ioutil.TempDir("", "package-buildpack")
				h.AssertNil(t, err)
				defer os.Remove(tmpDir)

				for _, imageOS := range []string{"linux", "windows"} {
					localMockDockerClient := testmocks.NewMockCommonAPIClient(mockController)
					localMockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: imageOS}, nil).AnyTimes()

					packClientWithExperimental, err := pack.NewClient(
						pack.WithDockerClient(localMockDockerClient),
						pack.WithLogger(logging.NewLogWithWriters(&out, &out)),
						pack.WithDownloader(mockDownloader),
						pack.WithExperimental(true),
					)
					h.AssertNil(t, err)

					fakeBlob := blob.NewBlob(filepath.Join("testdata", "empty-file"))
					bpURL := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
					mockDownloader.EXPECT().Download(gomock.Any(), bpURL).Return(fakeBlob, nil).AnyTimes()

					packagePath := filepath.Join(tmpDir, h.RandString(12)+"-test.cnb")
					h.AssertNil(t, packClientWithExperimental.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Format: pack.FormatFile,
						Name:   packagePath,
						Config: pubbldpkg.Config{
							Platform: dist.Platform{OS: imageOS},
							Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
								API:    api.MustParse("0.2"),
								Info:   dist.BuildpackInfo{ID: "bp.basic", Version: "2.3.4"},
								Stacks: []dist.Stack{{ID: "some.stack.id"}},
							})},
						},
						PullPolicy: pubcfg.PullNever,
					}))
				}
			})
		})

		when("nested package", func() {
			var (
				nestedPackage     *fakes.Image
				childDescriptor   dist.BuildpackDescriptor
				packageDescriptor dist.BuildpackDescriptor
				tmpDir            string
				err               error
				childAssets       = []dist.AssetInfo{{
					ID:      "child-asset",
					Name:    "child-asset-name",
					Sha256:  "child-asset-sha256",
					Stacks:  []string{"some.stack.id"},
					Version: "1.2.3",
				}}
			)

			it.Before(func() {
				childDescriptor = dist.BuildpackDescriptor{
					API:    api.MustParse("0.2"),
					Info:   dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
					Stacks: []dist.Stack{{ID: "some.stack.id"}},
					Assets: childAssets,
				}

				packageDescriptor = dist.BuildpackDescriptor{
					API:  api.MustParse("0.2"),
					Info: dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
					Order: dist.Order{{
						Group: []dist.BuildpackRef{{
							BuildpackInfo: dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
							Optional:      false,
						}},
					}},
				}

				tmpDir, err = ioutil.TempDir("", "package-buildpack")
				h.AssertNil(t, err)
			})

			it.After(func() {
				h.AssertNil(t, os.RemoveAll(tmpDir))
			})

			when("dependencies are packaged buildpack image", func() {
				it.Before(func() {
					nestedPackage = fakes.NewImage("nested/package-"+h.RandString(12), "", nil)
					mockImageFactory.EXPECT().NewImage(nestedPackage.Name(), false).Return(nestedPackage, nil)

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: nestedPackage.Name(),
						Config: pubbldpkg.Config{
							Platform:  dist.Platform{OS: "linux"},
							Buildpack: dist.BuildpackURI{URI: createBuildpack(childDescriptor)},
						},
						Publish:    true,
						PullPolicy: pubcfg.PullAlways,
					}))

					mockImageFetcher.EXPECT().Fetch(gomock.Any(), nestedPackage.Name(), true, pubcfg.PullAlways).Return(nestedPackage, nil)
				})

				it("should pull and use local nested package image", func() {
					packagePath := filepath.Join(tmpDir, "test.cnb")

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packagePath,
						Config: pubbldpkg.Config{
							Platform:     dist.Platform{OS: "linux"},
							Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
							Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: nestedPackage.Name()}}},
						},
						Publish:    false,
						PullPolicy: pubcfg.PullAlways,
						Format:     pack.FormatFile,
					}))

					assertPackageBPFileHasBuildpacks(t, packagePath, []dist.BuildpackDescriptor{packageDescriptor, childDescriptor})
				})
			})

			when("dependencies are unpackaged buildpack", func() {
				it("should work", func() {
					packagePath := filepath.Join(tmpDir, "test.cnb")

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packagePath,
						Config: pubbldpkg.Config{
							Platform:     dist.Platform{OS: "linux"},
							Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
							Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: createBuildpack(childDescriptor)}}},
						},
						Publish:    false,
						PullPolicy: pubcfg.PullAlways,
						Format:     pack.FormatFile,
					}))

					assertPackageBPFileHasBuildpacks(t, packagePath, []dist.BuildpackDescriptor{packageDescriptor, childDescriptor})
				})

				when("dependency download fails", func() {
					it("should error", func() {
						bpURL := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
						mockDownloader.EXPECT().Download(gomock.Any(), bpURL).Return(nil, image.ErrNotFound).AnyTimes()

						packagePath := filepath.Join(tmpDir, "test.cnb")

						err = subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
							Name: packagePath,
							Config: pubbldpkg.Config{
								Platform:     dist.Platform{OS: "linux"},
								Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
								Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: bpURL}}},
							},
							Publish:    false,
							PullPolicy: pubcfg.PullAlways,
							Format:     pack.FormatFile,
						})
						h.AssertError(t, err, "downloading buildpack")
					})
				})

				when("dependency isn't a valid buildpack", func() {
					it("should error", func() {
						fakeBlob := blob.NewBlob(filepath.Join("testdata", "empty-file"))
						bpURL := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
						mockDownloader.EXPECT().Download(gomock.Any(), bpURL).Return(fakeBlob, nil).AnyTimes()

						packagePath := filepath.Join(tmpDir, "test.cnb")

						err = subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
							Name: packagePath,
							Config: pubbldpkg.Config{
								Platform:     dist.Platform{OS: "linux"},
								Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
								Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: bpURL}}},
							},
							Publish:    false,
							PullPolicy: pubcfg.PullAlways,
							Format:     pack.FormatFile,
						})
						h.AssertError(t, err, "creating buildpack")
					})
				})
			})

			when("dependencies include packaged buildpack image and unpacked buildpack", func() {
				var secondChildDescriptor dist.BuildpackDescriptor

				it.Before(func() {
					secondChildDescriptor = dist.BuildpackDescriptor{
						API:    api.MustParse("0.2"),
						Info:   dist.BuildpackInfo{ID: "bp.nested1", Version: "2.3.4"},
						Stacks: []dist.Stack{{ID: "some.stack.id"}},
					}

					packageDescriptor.Order = append(packageDescriptor.Order, dist.OrderEntry{Group: []dist.BuildpackRef{{
						BuildpackInfo: dist.BuildpackInfo{ID: secondChildDescriptor.Info.ID, Version: secondChildDescriptor.Info.Version},
						Optional:      false,
					}}})

					nestedPackage = fakes.NewImage("nested/package-"+h.RandString(12), "", nil)
					mockImageFactory.EXPECT().NewImage(nestedPackage.Name(), false).Return(nestedPackage, nil)

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: nestedPackage.Name(),
						Config: pubbldpkg.Config{
							Platform:  dist.Platform{OS: "linux"},
							Buildpack: dist.BuildpackURI{URI: createBuildpack(childDescriptor)},
						},
						Publish:    true,
						PullPolicy: pubcfg.PullAlways,
					}))

					mockImageFetcher.EXPECT().Fetch(gomock.Any(), nestedPackage.Name(), true, pubcfg.PullAlways).Return(nestedPackage, nil)
				})

				it("should include both of them", func() {
					packagePath := filepath.Join(tmpDir, "test.cnb")

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packagePath,
						Config: pubbldpkg.Config{
							Platform:  dist.Platform{OS: "linux"},
							Buildpack: dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
							Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: nestedPackage.Name()}},
								{BuildpackURI: dist.BuildpackURI{URI: createBuildpack(secondChildDescriptor)}}},
						},
						Publish:    false,
						PullPolicy: pubcfg.PullAlways,
						Format:     pack.FormatFile,
					}))

					assertPackageBPFileHasBuildpacks(t, packagePath, []dist.BuildpackDescriptor{packageDescriptor, childDescriptor, secondChildDescriptor})
				})
			})

			when("dependencies include a packaged buildpack file", func() {
				var (
					dependencyPackagePath string
				)
				it.Before(func() {
					dependencyPackagePath = filepath.Join(tmpDir, "dep.cnb")
					dependencyPackageURI, err := paths.FilePathToURI(dependencyPackagePath, "")
					h.AssertNil(t, err)

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: dependencyPackagePath,
						Config: pubbldpkg.Config{
							Platform:  dist.Platform{OS: "linux"},
							Buildpack: dist.BuildpackURI{URI: createBuildpack(childDescriptor)},
						},
						PullPolicy: pubcfg.PullAlways,
						Format:     pack.FormatFile,
					}))

					mockDownloader.EXPECT().Download(gomock.Any(), dependencyPackageURI).Return(blob.NewBlob(dependencyPackagePath), nil).AnyTimes()
				})

				it("should open file and correctly add buildpacks", func() {
					packagePath := filepath.Join(tmpDir, "test.cnb")

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packagePath,
						Config: pubbldpkg.Config{
							Platform:     dist.Platform{OS: "linux"},
							Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
							Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: dependencyPackagePath}}},
						},
						Publish:    false,
						PullPolicy: pubcfg.PullAlways,
						Format:     pack.FormatFile,
					}))

					assertPackageBPFileHasBuildpacks(t, packagePath, []dist.BuildpackDescriptor{packageDescriptor, childDescriptor})
				})
			})

			when("dependencies include a buildpack registry urn file", func() {
				var (
					tmpDir          string
					registryFixture string
					packHome        string
				)
				it.Before(func() {
					var err error

					childDescriptor = dist.BuildpackDescriptor{
						API:    api.MustParse("0.2"),
						Info:   dist.BuildpackInfo{ID: "example/foo", Version: "1.1.0"},
						Stacks: []dist.Stack{{ID: "some.stack.id"}},
					}

					packageDescriptor = dist.BuildpackDescriptor{
						API:  api.MustParse("0.2"),
						Info: dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
						Order: dist.Order{{
							Group: []dist.BuildpackRef{{
								BuildpackInfo: dist.BuildpackInfo{ID: "example/foo", Version: "1.1.0"},
								Optional:      false,
							}},
						}},
					}

					tmpDir, err = ioutil.TempDir("", "registry")
					h.AssertNil(t, err)

					packHome = filepath.Join(tmpDir, ".pack")
					err = os.MkdirAll(packHome, 0755)
					h.AssertNil(t, err)
					os.Setenv("PACK_HOME", packHome)

					registryFixture = h.CreateRegistryFixture(t, tmpDir, filepath.Join("testdata", "registry"))
					h.AssertNotNil(t, registryFixture)

					packageImage := fakes.NewImage("example.com/some/package@sha256:74eb48882e835d8767f62940d453eb96ed2737de3a16573881dcea7dea769df7", "", nil)
					packageImage.AddLayerWithDiffID("testdata/empty-file", "sha256:xxx")
					packageImage.SetLabel("io.buildpacks.buildpackage.metadata", `{"id":"example/foo", "version":"1.1.0", "stacks":[{"id":"some.stack.id"}]}`)
					packageImage.SetLabel("io.buildpacks.buildpack.layers", `{"example/foo":{"1.1.0":{"api": "0.2", "layerDiffID":"sha256:xxx", "stacks":[{"id":"some.stack.id"}]}}}`)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), true, pubcfg.PullAlways).Return(packageImage, nil)

					packHome := filepath.Join(tmpDir, "packHome")
					h.AssertNil(t, os.Setenv("PACK_HOME", packHome))
					configPath := filepath.Join(packHome, "config.toml")
					h.AssertNil(t, cfg.Write(cfg.Config{
						Registries: []cfg.Registry{
							{
								Name: "some-registry",
								Type: "github",
								URL:  registryFixture,
							},
						},
					}, configPath))
				})

				it.After(func() {
					os.Unsetenv("PACK_HOME")
					err := os.RemoveAll(tmpDir)
					h.AssertNil(t, err)
				})

				it("should open file and correctly add buildpacks", func() {
					packagePath := filepath.Join(tmpDir, "test.cnb")

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packagePath,
						Config: pubbldpkg.Config{
							Platform:     dist.Platform{OS: "linux"},
							Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
							Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: "urn:cnb:registry:example/foo@1.1.0"}}},
						},
						Publish:    false,
						PullPolicy: pubcfg.PullAlways,
						Format:     pack.FormatFile,
						Registry:   "some-registry",
					}))

					assertPackageBPFileHasBuildpacks(t, packagePath, []dist.BuildpackDescriptor{packageDescriptor, childDescriptor})
				})
			})
		})
	})

	when("unknown format is provided", func() {
		it("should error", func() {
			mockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: "linux"}, nil).AnyTimes()

			err := subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
				Name:   "some-buildpack",
				Format: "invalid-format",
				Config: pubbldpkg.Config{
					Platform: dist.Platform{OS: "linux"},
					Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
						API:    api.MustParse("0.2"),
						Info:   dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
						Stacks: []dist.Stack{{ID: "some.stack.id"}},
					})},
				},
				Publish:    false,
				PullPolicy: pubcfg.PullAlways,
			})
			h.AssertError(t, err, "unknown format: 'invalid-format'")
		})
	})
}

func assertPackageBPFileHasBuildpacks(t *testing.T, path string, descriptors []dist.BuildpackDescriptor) {
	t.Helper()

	packageBlob := blob.NewBlob(path)
	mainBP, depBPs, err := buildpackage.BuildpacksFromOCILayoutBlob(packageBlob)
	h.AssertNil(t, err)
	h.AssertBuildpacksHaveDescriptors(t, append([]dist.Buildpack{mainBP}, depBPs...), descriptors)
}
