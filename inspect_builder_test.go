package pack_test

import (
	"errors"
	"fmt"
	"testing"

	imgtest "github.com/buildpack/lifecycle/testhelpers"
	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpack/pack/testhelpers"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/mocks"
	"github.com/buildpack/pack/testhelpers"
)

func TestInspectBuilder(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "InspectBuilder", testInspectBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testInspectBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		client         *pack.Client
		mockFetcher    *mocks.MockFetcher
		mockController *gomock.Controller
		builderImage   *imgtest.FakeImage
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockFetcher = mocks.NewMockFetcher(mockController)
		client = pack.NewClient(&config.Config{
			RunImages: []config.RunImage{
				{Image: "some/run-image", Mirrors: []string{"some/local-mirror"}},
			},
		}, mockFetcher)
		builderImage = imgtest.NewFakeImage(t, "some/builder", "", "")
	})

	it.After(func() {
		mockController.Finish()
	})

	when("the image exists", func() {
		for _, useDaemon := range []bool{true, false} {
			when(fmt.Sprintf("daemon is %t", useDaemon), func() {
				it.Before(func() {
					if useDaemon {
						mockFetcher.EXPECT().FetchLocalImage("some/builder").Return(builderImage, nil)
					} else {
						mockFetcher.EXPECT().FetchRemoteImage("some/builder").Return(builderImage, nil)
					}
				})

				when("the builder image has a metadata label", func() {
					it.Before(func() {
						testhelpers.AssertNil(t, builderImage.SetLabel("io.buildpacks.stack.id", "test.stack.id"))
						testhelpers.AssertNil(t, builderImage.SetLabel("io.buildpacks.builder.metadata", `{
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
			mockFetcher.EXPECT().FetchRemoteImage("some/builder").Return(nil, errors.New("some-error"))
		})
		it("returns an error", func() {
			_, err := client.InspectBuilder("some/builder", false)
			h.AssertError(t, err, "failed to get builder image 'some/builder': some-error")
		})
	})

	when("the image does not exist", func() {
		it.Before(func() {
			notFoundImage := imgtest.NewFakeImage(t, "", "", "")
			notFoundImage.Delete()
			mockFetcher.EXPECT().FetchLocalImage("some/builder").Return(notFoundImage, nil)
		})

		it("return nil metadata", func() {
			metadata, err := client.InspectBuilder("some/builder", true)
			h.AssertNil(t, err)
			h.AssertNil(t, metadata)
		})
	})
}
