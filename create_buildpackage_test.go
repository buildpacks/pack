package pack_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

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
		fakePackageImage *fakes.Image
		out              bytes.Buffer
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockDownloader = testmocks.NewMockDownloader(mockController)
		mockImageFactory = testmocks.NewMockImageFactory(mockController)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)

		fakePackageImage = fakes.NewImage("some/package", "", nil)
		mockImageFactory.EXPECT().NewImage("some/package", true).Return(fakePackageImage, nil).AnyTimes()

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
		fakePackageImage.Cleanup()
		mockController.Finish()
	})

	when("#CreatePackage", func() {
		when("package config is valid", func() {
			var opts pack.CreatePackageOptions

			it.Before(func() {
				buildpack1, err := ifakes.NewBuildpackFromDescriptor(dist.BuildpackDescriptor{
					API: api.MustParse("0.2"),
					Info: dist.BuildpackInfo{
						ID:      "bp.one",
						Version: "1.2.3",
					},
					Stacks: []dist.Stack{
						{ID: "some.stack.id"},
					},
					Order: nil,
				}, 0644)
				h.AssertNil(t, err)
				mockDownloader.EXPECT().Download(gomock.Any(), "https://example.com/bp.one.tgz").Return(buildpack1, nil).AnyTimes()

				opts = pack.CreatePackageOptions{
					Name: fakePackageImage.Name(),
					Config: buildpackage.Config{
						Default: dist.BuildpackInfo{
							ID:      "bp.nested",
							Version: "2.3.4",
						},
						Buildpacks: []dist.BuildpackURI{
							{URI: "https://example.com/bp.one.tgz"},
						},
						Packages: []dist.ImageRef{
							{Ref: "nested/package"},
						},
						Stacks: []dist.Stack{
							{ID: "some.stack.id"},
						},
					},
				}

				buildpack2, err := ifakes.NewBuildpackFromDescriptor(dist.BuildpackDescriptor{
					API: api.MustParse("0.2"),
					Info: dist.BuildpackInfo{
						ID:      "bp.nested",
						Version: "2.3.4",
					},
					Stacks: []dist.Stack{
						{ID: "some.stack.id"},
					},
					Order: nil,
				}, 0644)
				h.AssertNil(t, err)
				mockDownloader.EXPECT().Download(gomock.Any(), "https://example.com/bp.nested.tgz").Return(buildpack2, nil).AnyTimes()

				existingPackageImage := fakes.NewImage("nested/package", "", nil)
				mockImageFactory.EXPECT().NewImage("nested/package", true).Return(existingPackageImage, nil).AnyTimes()

				err = subject.CreatePackage(context.TODO(), pack.CreatePackageOptions{
					Name: "nested/package",
					Config: buildpackage.Config{
						Default: dist.BuildpackInfo{
							ID:      "bp.nested",
							Version: "2.3.4",
						},
						Buildpacks: []dist.BuildpackURI{
							{URI: "https://example.com/bp.nested.tgz"},
						},
						Stacks: []dist.Stack{
							{ID: "some.stack.id"},
						},
					},
					Publish: false,
				})
				h.AssertNil(t, err)

				// TODO: daemon and pull cases (https://github.com/buildpack/pack/issues/392)
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "nested/package", true, true).Return(existingPackageImage, nil).AnyTimes()
			})

			it("sets metadata", func() {
				h.AssertNil(t, subject.CreatePackage(context.TODO(), opts))
				h.AssertEq(t, fakePackageImage.IsSaved(), true)

				labelData, err := fakePackageImage.Label("io.buildpacks.buildpackage.metadata")
				h.AssertNil(t, err)
				var md buildpackage.Metadata
				h.AssertNil(t, json.Unmarshal([]byte(labelData), &md))

				h.AssertEq(t, md.ID, "bp.nested")
				h.AssertEq(t, md.Version, "2.3.4")
				h.AssertEq(t, len(md.Stacks), 1)
				h.AssertEq(t, md.Stacks[0].ID, "some.stack.id")
			})

			it("sets buildpack layers label", func() {
				h.AssertNil(t, subject.CreatePackage(context.TODO(), opts))
				h.AssertEq(t, fakePackageImage.IsSaved(), true)

				var bpLayers dist.BuildpackLayers
				_, err := dist.GetLabel(fakePackageImage, "io.buildpacks.buildpack.layers", &bpLayers)
				h.AssertNil(t, err)

				bp1Info, ok1 := bpLayers["bp.one"]["1.2.3"]
				h.AssertEq(t, ok1, true)
				h.AssertEq(t, bp1Info.Stacks, []dist.Stack{{
					ID: "some.stack.id",
				}})

				bp2Info, ok2 := bpLayers["bp.nested"]["2.3.4"]
				h.AssertEq(t, ok2, true)
				h.AssertEq(t, bp2Info.Stacks, []dist.Stack{{
					ID: "some.stack.id",
				}})
			})

			it("adds buildpack layers", func() {
				h.AssertNil(t, subject.CreatePackage(context.TODO(), opts))
				h.AssertEq(t, fakePackageImage.IsSaved(), true)

				buildpackExists := func(name, version string) {
					dirPath := fmt.Sprintf("/cnb/buildpacks/%s/%s", name, version)
					layerTar, err := fakePackageImage.FindLayerWithPath(dirPath)
					h.AssertNil(t, err)

					h.AssertOnTarEntry(t, layerTar, dirPath,
						h.IsDirectory(),
					)

					h.AssertOnTarEntry(t, layerTar, dirPath+"/bin/build",
						h.ContentEquals("build-contents"),
						h.HasOwnerAndGroup(0, 0),
						h.HasFileMode(0644),
					)

					h.AssertOnTarEntry(t, layerTar, dirPath+"/bin/detect",
						h.ContentEquals("detect-contents"),
						h.HasOwnerAndGroup(0, 0),
						h.HasFileMode(0644),
					)
				}

				buildpackExists("bp.one", "1.2.3")
				buildpackExists("bp.nested", "2.3.4")
			})

			when("when publish is true", func() {
				var fakeRemotePackageImage *fakes.Image

				it.Before(func() {
					fakeRemotePackageImage = fakes.NewImage("some/package", "", nil)
					mockImageFactory.EXPECT().NewImage("some/package", false).Return(fakeRemotePackageImage, nil).AnyTimes()

					opts.Publish = true
				})

				it.After(func() {
					fakeRemotePackageImage.Cleanup()
				})

				it("saves remote image", func() {
					h.AssertNil(t, subject.CreatePackage(context.TODO(), opts))
					h.AssertEq(t, fakeRemotePackageImage.IsSaved(), true)
				})
			})
		})
	})
}
