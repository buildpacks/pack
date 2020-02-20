package pack

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/google/go-cmp/cmp"

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
)

func TestInspectBuilder(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
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
	  "homepage": "http://geocities.com/cool-bp"
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

					it("returns the builder with the given name with information from the label", func() {
						builderInfo, err := subject.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)

						want := BuilderInfo{
							Description:     "Some description",
							Stack:           "test.stack.id",
							Mixins:          []string{"mixinOne", "mixinThree", "build:mixinTwo", "build:mixinFour"},
							RunImage:        "some/run-image",
							RunImageMirrors: []string{"gcr.io/some/default"},
							Buildpacks: []dist.BuildpackInfo{
								dist.BuildpackInfo{
									ID:       "test.bp.one",
									Version:  "1.0.0",
									Homepage: "http://geocities.com/cool-bp",
								},
							},
							Order: dist.Order{
								{
									Group: []dist.BuildpackRef{
										{
											BuildpackInfo: dist.BuildpackInfo{ID: "buildpack-1-id"},
											Optional:      false,
										},
										{
											BuildpackInfo: dist.BuildpackInfo{ID: "buildpack-2-id", Version: "buildpack-2-version-1"},
											Optional:      true,
										},
									},
								},
							},
							Lifecycle: builder.LifecycleDescriptor{
								Info: builder.LifecycleInfo{
									Version: builder.VersionMustParse("1.2.3"),
								},
							},
							CreatedBy: builder.CreatorMetadata{
								Name:    "pack",
								Version: "1.2.3",
							},
						}

						if diff := cmp.Diff(*builderInfo, want); diff != "" {
							t.Errorf("InspectBuilder() mismatch (-want +got):\n%s", diff)
						}
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
