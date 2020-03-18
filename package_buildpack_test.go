package pack_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/api"
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
		out              bytes.Buffer
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockDownloader = testmocks.NewMockDownloader(mockController)
		mockImageFactory = testmocks.NewMockImageFactory(mockController)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)

		var err error
		subject, err = pack.NewClient(
			pack.WithLogger(logging.NewLogWithWriters(&out, &out)),
			pack.WithDownloader(mockDownloader),
			pack.WithImageFactory(mockImageFactory),
			pack.WithFetcher(mockImageFetcher),
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
				Publish: true,
			}))
		})

		shouldFetchNestedPackage := func(demon, pull bool) {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), nestedPackage.Name(), demon, pull).Return(nestedPackage, nil)
		}

		shouldNotFindNestedPackageWhenCallingImageFetcherWith := func(demon, pull bool) {
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
				shouldFetchNestedPackage(true, true)
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
					Publish: false,
					NoPull:  false,
				}))
			})
		})

		when("publish=true and no-pull=false", func() {
			it("should use remote nested package image", func() {
				shouldFetchNestedPackage(false, true)
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
					Publish: true,
					NoPull:  false,
				}))
			})
		})

		when("publish=true and no-pull=true", func() {
			it("should push to registry and not pull nested package image", func() {
				shouldFetchNestedPackage(false, false)
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
					Publish: true,
					NoPull:  true,
				}))
			})
		})

		when("publish=false no-pull=true and there is no local image", func() {
			it("should fail without trying to retrieve nested image from registry", func() {
				shouldNotFindNestedPackageWhenCallingImageFetcherWith(true, false)

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
					Publish: false,
					NoPull:  true,
				}), "not found")
			})
		})
	})

	when("nested package is not a valid package", func() {
		it("should error", func() {
			notPackageImage := fakes.NewImage("not/package", "", nil)
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), notPackageImage.Name(), true, true).Return(notPackageImage, nil)

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
				Publish: false,
				NoPull:  false,
			}), "could not find label 'io.buildpacks.buildpackage.metadata' on image 'not/package'")
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
				Publish: false,
				NoPull:  false,
			})
			h.AssertError(t, err, "unknown format: 'invalid-format'")
		})
	})
}
