package pack

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/buildpack/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/internal/builder"
	"github.com/buildpack/pack/internal/dist"
	"github.com/buildpack/pack/internal/image"
	"github.com/buildpack/pack/internal/logging"
	h "github.com/buildpack/pack/testhelpers"
	"github.com/buildpack/pack/testmocks"
)

func TestInspectBuilder(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "InspectBuilder", testInspectBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testInspectBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		subject          *Client
		mockImageFetcher *testmocks.MockImageFetcher
		mockController   *gomock.Controller
		builderImage     *fakes.Image
		out              bytes.Buffer
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)

		subject = &Client{
			logger:       logging.NewLogWithWriters(&out, &out),
			imageFetcher: mockImageFetcher,
		}

		builderImage = fakes.NewImage("some/builder", "", nil)
		h.AssertNil(t, builderImage.SetLabel("io.buildpacks.stack.id", "test.stack.id"))
		h.AssertNil(t, builderImage.SetLabel(
			"io.buildpacks.stack.mixins",
			`["mixinOne", "build:mixinTwo", "mixinThree", "build:mixinFour"]`,
		))
		h.AssertNil(t, builderImage.SetEnv("CNB_USER_ID", "1234"))
		h.AssertNil(t, builderImage.SetEnv("CNB_GROUP_ID", "4321"))
	})

	it.After(func() {
		mockController.Finish()
	})

	when("the image exists", func() {
		for _, useDaemon := range []bool{true, false} {
			useDaemon := useDaemon
			when(fmt.Sprintf("daemon is %t", useDaemon), func() {
				it.Before(func() {
					if useDaemon {
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/builder", true, false).Return(builderImage, nil)
					} else {
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/builder", false, false).Return(builderImage, nil)
					}
				})

				when("the builder image has appropriate metadata labels", func() {
					it.Before(func() {
						h.AssertNil(t, builderImage.SetLabel("io.buildpacks.builder.metadata", `{
  "description": "Some description",
  "stack": {
    "runImage": {
      "image": "some/run-image",
      "mirrors": [
        "gcr.io/some/default"
      ]
    }
  },
  "buildpacks": [
    {
      "id": "test.bp.one",
      "version": "1.0.0",
      "latest": true
    }
  ],
  "lifecycle": {"version": "1.2.3"},
  "createdBy": {"name": "pack", "version": "1.2.3"}
}`))

						h.AssertNil(t, builderImage.SetLabel(
							"io.buildpacks.buildpack.order",
							`[{"group": [{"id": "buildpack-1-id", "optional": false}, {"id": "buildpack-2-id", "version": "buildpack-2-version-1", "optional": true}]}]`,
						))
					})

					it("returns the builder with the given name", func() {
						builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.RunImage, "some/run-image")
					})

					it("set the creator info", func() {
						builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.CreatedBy.Name, "pack")
						h.AssertEq(t, builderInfo.CreatedBy.Version, "1.2.3")
					})

					it("set the description", func() {
						builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.Description, "Some description")
					})

					it("set the stack ID", func() {
						builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.Stack, "test.stack.id")
					})

					it("sets the stack mixins", func() {
						builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t,
							builderInfo.Mixins, []string{"mixinOne", "mixinThree", "build:mixinTwo", "build:mixinFour"},
						)
					})

					it("set the defaults run image mirrors", func() {
						builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.RunImageMirrors, []string{"gcr.io/some/default"})
					})

					it("sets the buildpacks", func() {
						builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.Buildpacks[0], builder.BuildpackMetadata{
							BuildpackInfo: dist.BuildpackInfo{
								ID:      "test.bp.one",
								Version: "1.0.0",
							},
							Latest: true,
						})
					})

					it("sets the groups", func() {
						builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.Order[0].Group[0], dist.BuildpackRef{
							BuildpackInfo: dist.BuildpackInfo{
								ID:      "buildpack-1-id",
								Version: "",
							},
							Optional: false,
						})
						h.AssertEq(t, builderInfo.Order[0].Group[1], dist.BuildpackRef{
							BuildpackInfo: dist.BuildpackInfo{
								ID:      "buildpack-2-id",
								Version: "buildpack-2-version-1",
							},
							Optional: true,
						})
					})

					it("sets the lifecycle version", func() {
						builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.Lifecycle.Info.Version.String(), "1.2.3")
					})

					when("the image has no mixins", func() {
						it.Before(func() {
							h.AssertNil(t, builderImage.SetLabel("io.buildpacks.stack.mixins", ""))
						})

						it("sets empty stack mixins", func() {
							builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
							h.AssertNil(t, err)
							h.AssertEq(t, builderInfo.Mixins, []string{})
						})
					})
				})
			})
		}
	})

	when("fetcher fails to fetch the image", func() {
		it.Before(func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/builder", false, false).Return(nil, errors.New("some-error"))
		})

		it("returns an error", func() {
			_, err := subject.InspectBuilder("some/builder", false)
			h.AssertError(t, err, "some-error")
		})
	})

	when("the image does not exist", func() {
		it.Before(func() {
			notFoundImage := fakes.NewImage("", "", nil)
			notFoundImage.Delete()
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/builder", true, false).Return(nil, errors.Wrap(image.ErrNotFound, "some-error"))
		})

		it("return nil metadata", func() {
			metadata, err := subject.InspectBuilder("some/builder", true)
			h.AssertNil(t, err)
			h.AssertNil(t, metadata)
		})
	})
}
