package pack_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types"

	"github.com/buildpacks/pack/config"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/lifecycle/api"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/internal/logging"
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

		mockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: "linux"}, nil).AnyTimes()
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
				dependencyPath := "fakePath.file"
				mockDownloader.EXPECT().Download(gomock.Any(), dependencyPath).Return(blob.NewBlob("no-file.txt"), nil).AnyTimes()

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
						Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
						Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: dependencyPath}}},
					},
					Publish:    false,
					PullPolicy: config.PullAlways,
				})

				h.AssertError(t, err, "inspecting buildpack blob")
			})
		})
	})

	when("FormatImage", func() {
		when("nested package lives in registry", func() {
			var nestedPackage *fakes.Image

			it.Before(func() {
				nestedPackage = fakes.NewImage("nested/package-"+h.RandString(12), "", nil)
				mockImageFactory.EXPECT().NewImage(nestedPackage.Name(), false).Return(nestedPackage, nil)

				h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: nestedPackage.Name(),
					Config: pubbldpkg.Config{
						Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
							API:    api.MustParse("0.2"),
							Info:   dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
							Stacks: []dist.Stack{{ID: "some.stack.id"}},
						})},
					},
					Publish:    true,
					PullPolicy: config.PullAlways,
				}))
			})

			shouldFetchNestedPackage := func(demon bool, pull config.PullPolicy) {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), nestedPackage.Name(), demon, pull).Return(nestedPackage, nil)
			}

			shouldNotFindNestedPackageWhenCallingImageFetcherWith := func(demon bool, pull config.PullPolicy) {
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

			when("publish=false and no-pull=false", func() {
				it("should pull and use local nested package image", func() {
					shouldFetchNestedPackage(true, config.PullAlways)
					packageImage := shouldCreateLocalPackage()

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packageImage.Name(),
						Config: pubbldpkg.Config{
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
						PullPolicy: config.PullAlways,
					}))
				})
			})

			when("publish=true and no-pull=false", func() {
				it("should use remote nested package image", func() {
					shouldFetchNestedPackage(false, config.PullAlways)
					packageImage := shouldCreateRemotePackage()

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packageImage.Name(),
						Config: pubbldpkg.Config{
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
						PullPolicy: config.PullAlways,
					}))
				})
			})

			when("publish=true and pull-policy=never", func() {
				it("should push to registry and not pull nested package image", func() {
					shouldFetchNestedPackage(false, config.PullNever)
					packageImage := shouldCreateRemotePackage()

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packageImage.Name(),
						Config: pubbldpkg.Config{
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
						PullPolicy: config.PullNever,
					}))
				})
			})

			when("publish=false pull-policy=never and there is no local image", func() {
				it("should fail without trying to retrieve nested image from registry", func() {
					shouldNotFindNestedPackageWhenCallingImageFetcherWith(true, config.PullNever)

					h.AssertError(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: "some/package",
						Config: pubbldpkg.Config{
							Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
								API:    api.MustParse("0.2"),
								Info:   dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
								Stacks: []dist.Stack{{ID: "some.stack.id"}},
							})},
							Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: nestedPackage.Name()}}},
						},
						Publish:    false,
						PullPolicy: config.PullNever,
					}), "not found")
				})
			})
		})

		when("nested package is not a valid package", func() {
			it("should error", func() {
				notPackageImage := fakes.NewImage("not/package", "", nil)
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), notPackageImage.Name(), true, config.PullAlways).Return(notPackageImage, nil)

				h.AssertError(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: "some/package",
					Config: pubbldpkg.Config{
						Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
							API:    api.MustParse("0.2"),
							Info:   dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
							Stacks: []dist.Stack{{ID: "some.stack.id"}},
						})},
						Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: notPackageImage.Name()}}},
					},
					Publish:    false,
					PullPolicy: config.PullAlways,
				}), "extracting buildpacks from 'not/package': could not find label 'io.buildpacks.buildpackage.metadata'")
			})
		})
	})

	when("FormatFile", func() {
		var (
			nestedPackage     *fakes.Image
			childDescriptor   dist.BuildpackDescriptor
			packageDescriptor dist.BuildpackDescriptor
			tmpDir            string
			err               error
		)

		it.Before(func() {
			childDescriptor = dist.BuildpackDescriptor{
				API:    api.MustParse("0.2"),
				Info:   dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
				Stacks: []dist.Stack{{ID: "some.stack.id"}},
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
						Buildpack: dist.BuildpackURI{URI: createBuildpack(childDescriptor)},
					},
					Publish:    true,
					PullPolicy: config.PullAlways,
				}))

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), nestedPackage.Name(), true, config.PullAlways).Return(nestedPackage, nil)
			})

			it("should pull and use local nested package image", func() {
				packagePath := filepath.Join(tmpDir, "test.cnb")

				h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: packagePath,
					Config: pubbldpkg.Config{
						Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
						Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: nestedPackage.Name()}}},
					},
					Publish:    false,
					PullPolicy: config.PullAlways,
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
						Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
						Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: createBuildpack(childDescriptor)}}},
					},
					Publish:    false,
					PullPolicy: config.PullAlways,
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
							Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
							Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: bpURL}}},
						},
						Publish:    false,
						PullPolicy: config.PullAlways,
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
							Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
							Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: bpURL}}},
						},
						Publish:    false,
						PullPolicy: config.PullAlways,
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
						Buildpack: dist.BuildpackURI{URI: createBuildpack(childDescriptor)},
					},
					Publish:    true,
					PullPolicy: config.PullAlways,
				}))

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), nestedPackage.Name(), true, config.PullAlways).Return(nestedPackage, nil)
			})

			it("should include both of them", func() {
				packagePath := filepath.Join(tmpDir, "test.cnb")

				h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: packagePath,
					Config: pubbldpkg.Config{
						Buildpack: dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
						Dependencies: []dist.ImageOrURI{{ImageRef: dist.ImageRef{ImageName: nestedPackage.Name()}},
							{BuildpackURI: dist.BuildpackURI{URI: createBuildpack(secondChildDescriptor)}}},
					},
					Publish:    false,
					PullPolicy: config.PullAlways,
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

				h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: dependencyPackagePath,
					Config: pubbldpkg.Config{
						Buildpack: dist.BuildpackURI{URI: createBuildpack(childDescriptor)},
					},
					PullPolicy: config.PullAlways,
					Format:     pack.FormatFile,
				}))

				mockDownloader.EXPECT().Download(gomock.Any(), dependencyPackagePath).Return(blob.NewBlob(dependencyPackagePath), nil).AnyTimes()
			})

			it("should open file and correctly add buildpacks", func() {
				packagePath := filepath.Join(tmpDir, "test.cnb")

				h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: packagePath,
					Config: pubbldpkg.Config{
						Buildpack:    dist.BuildpackURI{URI: createBuildpack(packageDescriptor)},
						Dependencies: []dist.ImageOrURI{{BuildpackURI: dist.BuildpackURI{URI: dependencyPackagePath}}},
					},
					Publish:    false,
					PullPolicy: config.PullAlways,
					Format:     pack.FormatFile,
				}))

				assertPackageBPFileHasBuildpacks(t, packagePath, []dist.BuildpackDescriptor{packageDescriptor, childDescriptor})
			})
		})
	})

	when("unknown format is provided", func() {
		it("should error", func() {
			err := subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
				Name:   "some-buildpack",
				Format: "invalid-format",
				Config: pubbldpkg.Config{
					Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
						API:    api.MustParse("0.2"),
						Info:   dist.BuildpackInfo{ID: "bp.1", Version: "1.2.3"},
						Stacks: []dist.Stack{{ID: "some.stack.id"}},
					})},
				},
				Publish:    false,
				PullPolicy: config.PullAlways,
			})
			h.AssertError(t, err, "unknown format: 'invalid-format'")
		})
	})
}

func assertPackageBPFileHasBuildpacks(t *testing.T, path string, descriptors []dist.BuildpackDescriptor) {
	packageBlob := blob.NewBlob(path)
	mainBP, depBPs, err := buildpackage.BuildpacksFromOCILayoutBlob(packageBlob)
	h.AssertNil(t, err)
	h.AssertBuildpacksHaveDescriptors(t, append([]dist.Buildpack{mainBP}, depBPs...), descriptors)
}
