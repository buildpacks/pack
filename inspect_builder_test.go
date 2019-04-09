package pack_test

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/buildpack/lifecycle/image/fakes"
	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestInspectBuilder(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "InspectBuilder", testInspectBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testInspectBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		client           *pack.Client
		MockImageFetcher *mocks.MockImageFetcher
		mockController   *gomock.Controller
		builderImage     *fakes.Image
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		MockImageFetcher = mocks.NewMockImageFetcher(mockController)
		client = pack.NewClient(&config.Config{
			RunImages: []config.RunImage{
				{Image: "some/run-image", Mirrors: []string{"some/local-mirror"}},
			},
		},
			logging.NewLogger(ioutil.Discard, ioutil.Discard, false, false),
			MockImageFetcher,
		)
		builderImage = fakes.NewImage(t, "some/builder", "", "")
	})

	it.After(func() {
		mockController.Finish()
	})

	when("the image exists", func() {
		for _, useDaemon := range []bool{true, false} {
			when(fmt.Sprintf("daemon is %t", useDaemon), func() {
				it.Before(func() {
					if useDaemon {
						MockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/builder", true, false).Return(builderImage, nil)
					} else {
						MockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/builder", false, false).Return(builderImage, nil)
					}
				})

				when("the builder image has a metadata label", func() {
					it.Before(func() {
						h.AssertNil(t, builderImage.SetLabel("io.buildpacks.stack.id", "test.stack.id"))
						h.AssertNil(t, builderImage.SetLabel("io.buildpacks.builder.metadata", `{
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
  "groups": [
    {
      "buildpacks": [
        {
          "id": "test.bp.one",
          "version": "1.0.0",
          "latest": true
        }
      ]
    }
  ]
}`))
					})

					it("returns the builder with the given name", func() {
						builderInfo, err := client.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.RunImage, "some/run-image")
					})

					it("set the local run image mirrors", func() {
						builderInfo, err := client.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.LocalRunImageMirrors, []string{"some/local-mirror"})
					})

					it("set the defaults run image mirrors", func() {
						builderInfo, err := client.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.RunImageMirrors, []string{"gcr.io/some/default"})
					})

					it("sets the buildpacks", func() {
						builderInfo, err := client.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.Buildpacks[0], pack.BuildpackInfo{
							ID:      "test.bp.one",
							Version: "1.0.0",
							Latest:  true,
						})
					})

					it("sets the groups", func() {
						builderInfo, err := client.InspectBuilder("some/builder", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, builderInfo.Groups[0], []pack.BuildpackInfo{{
							ID:      "test.bp.one",
							Version: "1.0.0",
							Latest:  true,
						}})
					})
				})
			})
		}
	})

	when("fetcher fails to fetch the image", func() {
		it.Before(func() {
			MockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/builder", false, false).Return(nil, errors.New("some-error"))
		})

		it("returns an error", func() {
			_, err := client.InspectBuilder("some/builder", false)
			h.AssertError(t, err, "some-error")
		})
	})

	when("the image does not exist", func() {
		it.Before(func() {
			notFoundImage := fakes.NewImage(t, "", "", "")
			notFoundImage.Delete()
			MockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/builder", true, false).Return(nil, errors.Wrap(image.ErrNotFound, "some-error"))
		})

		it("return nil metadata", func() {
			metadata, err := client.InspectBuilder("some/builder", true)
			h.AssertNil(t, err)
			h.AssertNil(t, metadata)
		})
	})
}
