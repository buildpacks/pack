package pack_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	pubbldr "github.com/buildpacks/pack/builder"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/api"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/internal/image"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
)

func TestCreateBuilder(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "create_builder", testCreateBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateBuilder(t *testing.T, when spec.G, it spec.S) {
	when("#CreateBuilder", func() {
		var (
			mockController     *gomock.Controller
			mockDownloader     *testmocks.MockDownloader
			mockImageFactory   *testmocks.MockImageFactory
			mockImageFetcher   *testmocks.MockImageFetcher
			fakeBuildImage     *fakes.Image
			fakeRunImage       *fakes.Image
			fakeRunImageMirror *fakes.Image
			opts               pack.CreateBuilderOptions
			subject            *pack.Client
			logger             logging.Logger
			out                bytes.Buffer
			tmpDir             string
		)

		it.Before(func() {
			logger = ilogging.NewLogWithWriters(&out, &out)
			mockController = gomock.NewController(t)
			mockDownloader = testmocks.NewMockDownloader(mockController)
			mockImageFetcher = testmocks.NewMockImageFetcher(mockController)
			mockImageFactory = testmocks.NewMockImageFactory(mockController)

			fakeBuildImage = fakes.NewImage("some/build-image", "", nil)
			h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.mixins", `["mixinX", "build:mixinY"]`))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_USER_ID", "1234"))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_GROUP_ID", "4321"))

			fakeRunImage = fakes.NewImage("some/run-image", "", nil)
			h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

			fakeRunImageMirror = fakes.NewImage("localhost:5000/some/run-image", "", nil)
			h.AssertNil(t, fakeRunImageMirror.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

			mockDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/bp-one.tgz").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "some/buildpack/dir").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "file:///some-lifecycle").Return(blob.NewBlob(filepath.Join("testdata", "lifecycle")), nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "file:///some-lifecycle-platform-0-1").Return(blob.NewBlob(filepath.Join("testdata", "lifecycle-platform-0.1")), nil).AnyTimes()

			var err error
			subject, err = pack.NewClient(
				pack.WithLogger(logger),
				pack.WithDownloader(mockDownloader),
				pack.WithImageFactory(mockImageFactory),
				pack.WithFetcher(mockImageFetcher),
			)
			h.AssertNil(t, err)

			opts = pack.CreateBuilderOptions{
				BuilderName: "some/builder",
				Config: pubbldr.Config{
					Description: "Some description",
					Buildpacks: []pubbldr.BuildpackConfig{
						{
							BuildpackInfo: dist.BuildpackInfo{ID: "bp.one", Version: "1.2.3"},
							ImageOrURI: dist.ImageOrURI{
								BuildpackURI: dist.BuildpackURI{
									URI: "https://example.fake/bp-one.tgz",
								},
							},
						},
					},
					Order: []dist.OrderEntry{{
						Group: []dist.BuildpackRef{
							{BuildpackInfo: dist.BuildpackInfo{ID: "bp.one", Version: "1.2.3"}, Optional: false},
						}},
					},
					Stack: pubbldr.StackConfig{
						ID:              "some.stack.id",
						BuildImage:      "some/build-image",
						RunImage:        "some/run-image",
						RunImageMirrors: []string{"localhost:5000/some/run-image"},
					},
					Lifecycle: pubbldr.LifecycleConfig{URI: "file:///some-lifecycle"},
				},
				Publish: false,
				NoPull:  false,
			}

			tmpDir, err = ioutil.TempDir("", "create-builder-test")
			h.AssertNil(t, err)
		})

		it.After(func() {
			mockController.Finish()
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		var prepareFetcherWithRunImages = func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", gomock.Any(), gomock.Any()).Return(fakeRunImage, nil).AnyTimes()
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", gomock.Any(), gomock.Any()).Return(fakeRunImageMirror, nil).AnyTimes()
		}

		var prepareFetcherWithBuildImage = func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", gomock.Any(), gomock.Any()).Return(fakeBuildImage, nil)
		}

		var configureBuilderWithLifecycleAPIv0_1 = func() {
			opts.Config.Lifecycle = pubbldr.LifecycleConfig{URI: "file:///some-lifecycle-platform-0-1"}
		}

		var successfullyCreateBuilder = func() *builder.Builder {
			t.Helper()

			err := subject.CreateBuilder(context.TODO(), opts)
			h.AssertNil(t, err)

			h.AssertEq(t, fakeBuildImage.IsSaved(), true)
			bldr, err := builder.FromImage(fakeBuildImage)
			h.AssertNil(t, err)

			return bldr
		}

		when("validating the builder config", func() {
			it("should fail when the stack ID is empty", func() {
				opts.Config.Stack.ID = ""

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack.id is required")
			})

			it("should fail when the stack ID from the builder config does not match the stack ID from the build image", func() {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", true, true).Return(fakeBuildImage, nil)
				h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.id", "other.stack.id"))
				prepareFetcherWithRunImages()

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack 'some.stack.id' from builder config is incompatible with stack 'other.stack.id' from build image")
			})

			it("should fail when the build image is empty", func() {
				opts.Config.Stack.BuildImage = ""

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack.build-image is required")
			})

			it("should fail when the run image is empty", func() {
				opts.Config.Stack.RunImage = ""

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack.run-image is required")
			})

			it("should fail when lifecycle version is not a semver", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Lifecycle.URI = ""
				opts.Config.Lifecycle.Version = "not-semver"

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "'lifecycle.version' must be a valid semver")
			})

			it("should fail when both lifecycle version and uri are present", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Lifecycle.URI = "file://some-lifecycle"
				opts.Config.Lifecycle.Version = "something"

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "'lifecycle' can only declare 'version' or 'uri', not both")
			})

			it("should fail when buildpack ID does not match downloaded buildpack", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Buildpacks[0].ID = "does.not.match"

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "buildpack from URI 'https://example.fake/bp-one.tgz' has ID 'bp.one' which does not match ID 'does.not.match' from builder config")
			})

			it("should fail when buildpack version does not match downloaded buildpack", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Buildpacks[0].Version = "0.0.0"

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "buildpack from URI 'https://example.fake/bp-one.tgz' has version '1.2.3' which does not match version '0.0.0' from builder config")
			})
		})

		when("validating the run image config", func() {
			it("should fail when the stack ID from the builder config does not match the stack ID from the run image", func() {
				prepareFetcherWithRunImages()
				h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "other.stack.id"))

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack 'some.stack.id' from builder config is incompatible with stack 'other.stack.id' from run image 'some/run-image'")
			})

			it("should fail when the stack ID from the builder config does not match the stack ID from the run image mirrors", func() {
				prepareFetcherWithRunImages()
				h.AssertNil(t, fakeRunImageMirror.SetLabel("io.buildpacks.stack.id", "other.stack.id"))

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack 'some.stack.id' from builder config is incompatible with stack 'other.stack.id' from run image 'localhost:5000/some/run-image'")
			})

			it("should warn when the run image cannot be found", func() {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", true, true).Return(fakeBuildImage, nil)

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", false, false).Return(nil, errors.Wrap(image.ErrNotFound, "yikes!"))
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", true, false).Return(nil, errors.Wrap(image.ErrNotFound, "yikes!"))

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", false, false).Return(nil, errors.Wrap(image.ErrNotFound, "yikes!"))
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", true, false).Return(nil, errors.Wrap(image.ErrNotFound, "yikes!"))

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)

				h.AssertContains(t, out.String(), "Warning: run image 'some/run-image' is not accessible")
			})

			when("publish is true", func() {
				it("should only try to validate the remote run image", func() {
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", true, gomock.Any()).Times(0)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", true, gomock.Any()).Times(0)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", true, gomock.Any()).Times(0)

					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", false, gomock.Any()).Return(fakeBuildImage, nil)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", false, gomock.Any()).Return(fakeRunImage, nil)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", false, gomock.Any()).Return(fakeRunImageMirror, nil)

					opts.Publish = true

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertNil(t, err)
				})
			})
		})

		when("only lifecycle version is provided", func() {
			it("should download from predetermined uri", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Lifecycle.URI = ""
				opts.Config.Lifecycle.Version = "3.4.5"

				mockDownloader.EXPECT().Download(
					gomock.Any(),
					"https://github.com/buildpacks/lifecycle/releases/download/v3.4.5/lifecycle-v3.4.5+linux.x86-64.tgz",
				).Return(
					blob.NewBlob(filepath.Join("testdata", "lifecycle")), nil,
				)

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)
			})
		})

		when("no lifecycle version or URI is provided", func() {
			it("should download default lifecycle", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Lifecycle.URI = ""
				opts.Config.Lifecycle.Version = ""

				mockDownloader.EXPECT().Download(
					gomock.Any(),
					fmt.Sprintf(
						"https://github.com/buildpacks/lifecycle/releases/download/v%s/lifecycle-v%s+linux.x86-64.tgz",
						builder.DefaultLifecycleVersion,
						builder.DefaultLifecycleVersion,
					),
				).Return(
					blob.NewBlob(filepath.Join("testdata", "lifecycle")), nil,
				)

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)
			})
		})

		when("buildpack mixins are not satisfied", func() {
			it("should return an error", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.mixins", ""))

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "validating buildpacks: buildpack 'bp.one@1.2.3' requires missing mixin(s): build:mixinY, mixinX")
			})
		})

		when("creation succeeds", func() {
			it("should set basic metadata", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()

				bldr := successfullyCreateBuilder()

				h.AssertEq(t, bldr.Name(), "some/builder")
				h.AssertEq(t, bldr.Description(), "Some description")
				h.AssertEq(t, bldr.UID, 1234)
				h.AssertEq(t, bldr.GID, 4321)
				h.AssertEq(t, bldr.StackID, "some.stack.id")
			})

			it("should set buildpack and order metadata", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()

				bldr := successfullyCreateBuilder()

				bpInfo := dist.BuildpackInfo{
					ID:      "bp.one",
					Version: "1.2.3",
				}
				h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{bpInfo})
				h.AssertEq(t, bldr.Order(), dist.Order{{
					Group: []dist.BuildpackRef{{
						BuildpackInfo: bpInfo,
						Optional:      false,
					}},
				}})
			})

			it("should embed the lifecycle", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()

				bldr := successfullyCreateBuilder()

				h.AssertEq(t, bldr.LifecycleDescriptor().Info.Version.String(), "3.4.5")
				h.AssertEq(t, bldr.LifecycleDescriptor().API.PlatformVersion.String(), "0.2")

				layerTar, err := fakeBuildImage.FindLayerWithPath("/cnb/lifecycle")
				h.AssertNil(t, err)
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/detector")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/restorer")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/analyzer")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/builder")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/exporter")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/launcher")
			})
		})

		when("creation succeeds for platform API < 0.2", func() {
			it("should set basic metadata", func() {
				configureBuilderWithLifecycleAPIv0_1()
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()

				bldr := successfullyCreateBuilder()

				h.AssertEq(t, bldr.Name(), "some/builder")
				h.AssertEq(t, bldr.Description(), "Some description")
				h.AssertEq(t, bldr.UID, 1234)
				h.AssertEq(t, bldr.GID, 4321)
				h.AssertEq(t, bldr.StackID, "some.stack.id")
			})

			it("should set buildpack and order metadata", func() {
				configureBuilderWithLifecycleAPIv0_1()
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()

				bldr := successfullyCreateBuilder()

				bpInfo := dist.BuildpackInfo{
					ID:      "bp.one",
					Version: "1.2.3",
				}
				h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{bpInfo})
				h.AssertEq(t, bldr.Order(), dist.Order{{
					Group: []dist.BuildpackRef{{
						BuildpackInfo: bpInfo,
						Optional:      false,
					}},
				}})
			})

			it("should embed the lifecycle", func() {
				configureBuilderWithLifecycleAPIv0_1()
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()

				bldr := successfullyCreateBuilder()

				h.AssertEq(t, bldr.LifecycleDescriptor().Info.Version.String(), "3.4.5")
				h.AssertEq(t, bldr.LifecycleDescriptor().API.PlatformVersion.String(), "0.1")

				layerTar, err := fakeBuildImage.FindLayerWithPath("/cnb/lifecycle")
				h.AssertNil(t, err)
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/detector")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/restorer")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/analyzer")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/builder")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/exporter")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/cacher")
				assertTarHasFile(t, layerTar, "/cnb/lifecycle/launcher")
			})
		})

		when("windows", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")
			})

			it("disallows directory-based buildpacks", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Buildpacks[0].URI = "testdata/buildpack"

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t,
					err,
					"buildpack 'testdata/buildpack': directory-based buildpacks are not currently supported on Windows")
			})
		})

		when("is posix", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS == "windows", "Skipped on windows")
			})

			it("supports directory buildpacks", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Buildpacks[0].URI = "some/buildpack/dir"

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)
			})
		})

		when("packages", func() {
			createBuildpack := func(descriptor dist.BuildpackDescriptor) string {
				bp, err := ifakes.NewFakeBuildpackBlob(descriptor, 0644)
				h.AssertNil(t, err)
				url := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
				mockDownloader.EXPECT().Download(gomock.Any(), url).Return(bp, nil).AnyTimes()
				return url
			}

			when("package image lives in registry", func() {
				var packageImage *fakes.Image

				it.Before(func() {
					packageImage = fakes.NewImage("some/package-"+h.RandString(12), "", nil)
					mockImageFactory.EXPECT().NewImage(packageImage.Name(), false).Return(packageImage, nil)

					bpd := dist.BuildpackDescriptor{
						API:    api.MustParse("0.3"),
						Info:   dist.BuildpackInfo{ID: "some.pkg.bp", Version: "2.3.4"},
						Stacks: []dist.Stack{{ID: "some.stack.id"}},
					}

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						ImageName: packageImage.Name(),
						Config: pubbldpkg.Config{
							Buildpack: dist.BuildpackURI{URI: createBuildpack(bpd)},
						},
						Publish: true,
					}))
				})

				shouldFetchPackageImageWith := func(demon, pull bool) {
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), demon, pull).Return(packageImage, nil)
				}

				prepareFetcherWithMissingPackageImage := func() {
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), gomock.Any(), gomock.Any()).Return(nil, image.ErrNotFound)
				}

				when("publish=false and no-pull=false", func() {
					it("should pull and use local package image", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"

						opts.Publish = false
						opts.NoPull = false
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.BuildpackConfig{
								ImageOrURI: dist.ImageOrURI{
									ImageRef: dist.ImageRef{ImageName: packageImage.Name()},
								},
							},
						)

						shouldFetchPackageImageWith(true, true)
						h.AssertNil(t, subject.CreateBuilder(context.TODO(), opts))
					})
				})

				when("publish=true and no-pull=false", func() {
					it("should use remote package image", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"

						opts.Publish = true
						opts.NoPull = false
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.BuildpackConfig{
								ImageOrURI: dist.ImageOrURI{
									ImageRef: dist.ImageRef{ImageName: packageImage.Name()},
								},
							},
						)

						shouldFetchPackageImageWith(false, true)
						h.AssertNil(t, subject.CreateBuilder(context.TODO(), opts))
					})
				})

				when("publish=true and no-pull=true", func() {
					it("should push to registry and not pull package image", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"

						opts.Publish = true
						opts.NoPull = true
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.BuildpackConfig{
								ImageOrURI: dist.ImageOrURI{
									ImageRef: dist.ImageRef{ImageName: packageImage.Name()},
								},
							},
						)

						shouldFetchPackageImageWith(false, false)
						h.AssertNil(t, subject.CreateBuilder(context.TODO(), opts))
					})
				})

				when("publish=false no-pull=true and there is no local package image", func() {
					it("should fail without trying to retrieve package image from registry", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"

						opts.Publish = false
						opts.NoPull = true
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.BuildpackConfig{
								ImageOrURI: dist.ImageOrURI{
									ImageRef: dist.ImageRef{ImageName: packageImage.Name()},
								},
							},
						)

						prepareFetcherWithMissingPackageImage()

						h.AssertError(t, subject.CreateBuilder(context.TODO(), opts), "not found")
					})
				})
			})

			when("package image is not a valid package", func() {
				it("should error", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.BuilderName = "some/builder"

					notPackageImage := fakes.NewImage("not/package", "", nil)
					opts.Config.Buildpacks = append(
						opts.Config.Buildpacks,
						pubbldr.BuildpackConfig{
							ImageOrURI: dist.ImageOrURI{
								ImageRef: dist.ImageRef{ImageName: notPackageImage.Name()},
							},
						},
					)

					mockImageFetcher.EXPECT().Fetch(gomock.Any(), notPackageImage.Name(), gomock.Any(), gomock.Any()).Return(notPackageImage, nil)
					h.AssertNil(t, notPackageImage.SetLabel("io.buildpacks.buildpack.layers", ""))

					h.AssertError(t, subject.CreateBuilder(context.TODO(), opts), "could not find label 'io.buildpacks.buildpackage.metadata' on image 'not/package'")
				})
			})
		})
	})
}

func assertTarHasFile(t *testing.T, tarFile, path string) {
	t.Helper()

	exist := tarHasFile(t, tarFile, path)
	if !exist {
		t.Fatalf("%s does not exist in %s", path, tarFile)
	}
}

func tarHasFile(t *testing.T, tarFile, path string) (exist bool) {
	t.Helper()

	r, err := os.Open(tarFile)
	h.AssertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		h.AssertNil(t, err)

		if header.Name == path {
			return true
		}
	}

	return false
}
