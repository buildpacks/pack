package pack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/buildpack/imgutil/fakes"
	"github.com/buildpack/lifecycle/metadata"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/image"
	ifakes "github.com/buildpack/pack/internal/fakes"
	h "github.com/buildpack/pack/testhelpers"
	"github.com/buildpack/pack/testmocks"
)

func TestInspectImage(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "InspectImage", testInspectImage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testInspectImage(t *testing.T, when spec.G, it spec.S) {
	var (
		subject          *Client
		mockImageFetcher *testmocks.MockImageFetcher
		mockController   *gomock.Controller
		fakeImage        *fakes.Image
		out              bytes.Buffer
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)

		subject = &Client{
			logger:       ifakes.NewFakeLogger(&out),
			imageFetcher: mockImageFetcher,
		}

		fakeImage = fakes.NewImage("some/image", "", nil)
		h.AssertNil(t, fakeImage.SetLabel("io.buildpacks.stack.id", "test.stack.id"))
		h.AssertNil(t, fakeImage.SetLabel(
			"io.buildpacks.lifecycle.metadata",
			`{
  "stack": {
    "runImage": {
      "image": "some-run-image",
      "mirrors": [
        "some-mirror",
        "other-mirror"
      ]
    }
  },
  "runImage": {
    "topLayer": "some-top-layer",
    "reference": "some-run-image-reference"
  }
}`,
		))
		h.AssertNil(t, fakeImage.SetLabel(
			"io.buildpacks.build.metadata",
			`{
  "bom": [
    {
      "name": "some-bom-element"
    }
  ],
  "buildpacks": [
    {
      "id": "some-buildpack",
      "version": "some-version"
    },
    {
      "id": "other-buildpack",
      "version": "other-version"
    }
  ],
  "launcher": {
    "version": "0.5.0"
  }
}`,
		))
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
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/image", true, false).Return(fakeImage, nil)
					} else {
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/image", false, false).Return(fakeImage, nil)
					}
				})

				it("returns the stack ID", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)
					h.AssertEq(t, info.StackID, "test.stack.id")
				})

				it("returns the stack", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)
					h.AssertEq(t, info.Stack,
						metadata.StackMetadata{
							RunImage: metadata.StackRunImageMetadata{
								Image: "some-run-image",
								Mirrors: []string{
									"some-mirror",
									"other-mirror",
								},
							},
						},
					)
				})

				it("returns the base image", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)
					h.AssertEq(t, info.Base,
						metadata.RunImageMetadata{
							TopLayer:  "some-top-layer",
							Reference: "some-run-image-reference",
						},
					)
				})

				it("returns the BOM", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)

					rawBOM, err := json.Marshal(info.BOM)
					h.AssertNil(t, err)
					h.AssertEq(t, string(rawBOM), `[{"name":"some-bom-element"}]`)
				})

				it("returns the buildpacks", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)

					h.AssertEq(t, len(info.Buildpacks), 2)
					h.AssertEq(t, info.Buildpacks[0].ID, "some-buildpack")
					h.AssertEq(t, info.Buildpacks[0].Version, "some-version")
					h.AssertEq(t, info.Buildpacks[1].ID, "other-buildpack")
					h.AssertEq(t, info.Buildpacks[1].Version, "other-version")
				})
			})
		}
	})

	when("the image doesn't exist", func() {
		it("returns nil", func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "not/some-image", true, false).Return(nil, image.ErrNotFound)

			info, err := subject.InspectImage("not/some-image", true)
			h.AssertNil(t, err)
			h.AssertNil(t, info)
		})
	})

	when("there is an error fetching the image", func() {
		it("returns the error", func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "not/some-image", true, false).Return(nil, errors.New("some-error"))

			_, err := subject.InspectImage("not/some-image", true)
			h.AssertError(t, err, "some-error")
		})
	})

	when("the image is missing labels", func() {
		it("returns empty data", func() {
			mockImageFetcher.EXPECT().
				Fetch(gomock.Any(), "missing/labels", true, false).
				Return(fakes.NewImage("missing/labels", "", nil), nil)
			info, err := subject.InspectImage("missing/labels", true)
			h.AssertNil(t, err)
			h.AssertEq(t, info, &ImageInfo{})
		})
	})

	when("the image has malformed labels", func() {
		var badImage *fakes.Image

		it.Before(func() {
			badImage = fakes.NewImage("bad/image", "", nil)
			mockImageFetcher.EXPECT().
				Fetch(gomock.Any(), "bad/image", true, false).
				Return(badImage, nil)
		})

		it("returns an error when layers md cannot parse", func() {
			h.AssertNil(t, badImage.SetLabel("io.buildpacks.lifecycle.metadata", "not   ----  json"))
			_, err := subject.InspectImage("bad/image", true)
			h.AssertError(t, err, "failed to parse label 'io.buildpacks.lifecycle.metadata'")
		})

		it("returns an error when build md cannot parse", func() {
			h.AssertNil(t, badImage.SetLabel("io.buildpacks.build.metadata", "not   ----  json"))
			_, err := subject.InspectImage("bad/image", true)
			h.AssertError(t, err, "failed to parse label 'io.buildpacks.build.metadata'")
		})
	})

	when("lifecycle version is 0.4.x or earlier", func() {
		it("includes an empty base image reference", func() {
			oldImage := fakes.NewImage("old/image", "", nil)
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "old/image", true, false).Return(oldImage, nil)

			h.AssertNil(t, oldImage.SetLabel(
				"io.buildpacks.lifecycle.metadata",
				`{
  "runImage": {
    "topLayer": "some-top-layer",
    "reference": "some-run-image-reference"
  }
}`,
			))
			h.AssertNil(t, oldImage.SetLabel(
				"io.buildpacks.build.metadata",
				`{
  "launcher": {
    "version": "0.4.0"
  }
}`,
			))

			info, err := subject.InspectImage("old/image", true)
			h.AssertNil(t, err)
			h.AssertEq(t, info.Base,
				metadata.RunImageMetadata{
					TopLayer:  "some-top-layer",
					Reference: "",
				},
			)
		})
	})
}
