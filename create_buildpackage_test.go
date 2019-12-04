package pack_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/buildpack/imgutil"
	"github.com/buildpack/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/internal/api"
	"github.com/buildpack/pack/internal/buildpackage"
	"github.com/buildpack/pack/internal/dist"
	ifakes "github.com/buildpack/pack/internal/fakes"
	"github.com/buildpack/pack/internal/image"
	"github.com/buildpack/pack/internal/logging"
	h "github.com/buildpack/pack/testhelpers"
	"github.com/buildpack/pack/testmocks"
)

func TestCreatePackage(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "CreatePackage", testCreatePackage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreatePackage(t *testing.T, when spec.G, it spec.S) {
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
		bp, err := ifakes.NewBuildpackFromDescriptor(descriptor, 0644)
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

			bpd := dist.BuildpackDescriptor{
				API:    api.MustParse("0.2"),
				Info:   dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
				Stacks: []dist.Stack{{ID: "some.stack.id"}},
			}

			h.AssertNil(t, subject.CreatePackage(context.TODO(), pack.CreatePackageOptions{
				Name: nestedPackage.Name(),
				Config: buildpackage.Config{
					Default:    bpd.Info,
					Buildpacks: []dist.BuildpackURI{{URI: createBuildpack(bpd)}},
					Stacks:     bpd.Stacks,
				},
				Publish: true,
			}))
		})

		shouldCallImageFetcherWith := func(demon, pull bool) {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), nestedPackage.Name(), demon, pull).Return(nestedPackage, nil)
		}

		shouldNotFindImageWhenCallingImageFetcherWith := func(demon, pull bool) {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), nestedPackage.Name(), demon, pull).Return(nil, image.ErrNotFound)
		}

		shouldCreateLocalImage := func() imgutil.Image {
			img := fakes.NewImage("some/package"+h.RandString(12), "", nil)
			mockImageFactory.EXPECT().NewImage(img.Name(), true).Return(img, nil)
			return img
		}

		shouldCreateRemoteImage := func() *fakes.Image {
			img := fakes.NewImage("some/package"+h.RandString(12), "", nil)
			mockImageFactory.EXPECT().NewImage(img.Name(), false).Return(img, nil)
			return img
		}

		when("publish=false and no-pull=false", func() {
			it("should pull and use local image", func() {
				shouldCallImageFetcherWith(true, true)
				localImage := shouldCreateLocalImage()
				h.AssertNil(t, subject.CreatePackage(context.TODO(), pack.CreatePackageOptions{
					Name: localImage.Name(),
					Config: buildpackage.Config{
						Default:  dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
						Packages: []dist.ImageRef{{Ref: nestedPackage.Name()}},
						Stacks:   []dist.Stack{{ID: "some.stack.id"}},
					},
					Publish: false,
					NoPull:  false,
				}))
			})
		})

		when("publish=true and no-pull=false", func() {
			it("should use remote image", func() {
				shouldCallImageFetcherWith(false, true)
				packageImage := shouldCreateRemoteImage()

				h.AssertNil(t, subject.CreatePackage(context.TODO(), pack.CreatePackageOptions{
					Name: packageImage.Name(),
					Config: buildpackage.Config{
						Default:  dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
						Packages: []dist.ImageRef{{Ref: nestedPackage.Name()}},
						Stacks:   []dist.Stack{{ID: "some.stack.id"}},
					},
					Publish: true,
					NoPull:  false,
				}))
			})
		})

		when("publish=true and no-pull=true", func() {
			it("should not pull image and push to registry", func() {
				shouldCallImageFetcherWith(false, false)
				packageImage := shouldCreateRemoteImage()

				h.AssertNil(t, subject.CreatePackage(context.TODO(), pack.CreatePackageOptions{
					Name: packageImage.Name(),
					Config: buildpackage.Config{
						Default:  dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
						Packages: []dist.ImageRef{{Ref: nestedPackage.Name()}},
						Stacks:   []dist.Stack{{ID: "some.stack.id"}},
					},
					Publish: true,
					NoPull:  true,
				}))
			})
		})

		when("publish=false no-pull=true and there is no local image", func() {
			it("should fail without trying to retrieve image from registry", func() {
				shouldNotFindImageWhenCallingImageFetcherWith(true, false)

				h.AssertError(t, subject.CreatePackage(context.TODO(), pack.CreatePackageOptions{
					Name: "some/package",
					Config: buildpackage.Config{
						Default:  dist.BuildpackInfo{ID: "bp.nested", Version: "2.3.4"},
						Packages: []dist.ImageRef{{Ref: nestedPackage.Name()}},
						Stacks:   []dist.Stack{{ID: "some.stack.id"}},
					},
					Publish: false,
					NoPull:  true,
				}), "not found")
			})
		})
	})

	when("nested package is not a package", func() {
		it("should error", func() {
			notPackageImage := fakes.NewImage("not/package", "", nil)
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), notPackageImage.Name(), true, true).Return(notPackageImage, nil)

			h.AssertError(t, subject.CreatePackage(context.TODO(), pack.CreatePackageOptions{
				Name: "",
				Config: buildpackage.Config{
					Default:  dist.BuildpackInfo{ID: "bp.1.id", Version: "bp.1.version"},
					Packages: []dist.ImageRef{{Ref: notPackageImage.Name()}},
					Stacks:   []dist.Stack{{ID: "stack.1.id"}},
				},
				Publish: false,
				NoPull:  false,
			}), "label 'io.buildpacks.buildpack.layers' not present on package 'not/package'")
		})
	})
}
