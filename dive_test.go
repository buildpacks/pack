package pack

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	ioutil2 "gopkg.in/src-d/go-git.v4/utils/ioutil"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
)

func TestDive(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "InspectImage", testDive, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testDive(t *testing.T, when spec.G, it spec.S) {
	var (
		subject          *Client
		mockImageFetcher *testmocks.MockImageFetcher
		mockDockerClient *testmocks.MockCommonAPIClient
		mockController   *gomock.Controller
		fakeImage        *fakes.Image
		out              bytes.Buffer
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)
		mockDockerClient = testmocks.NewMockCommonAPIClient(mockController)

		var err error
		subject, err = NewClient(WithLogger(logging.NewLogWithWriters(&out, &out)), WithFetcher(mockImageFetcher), WithDockerClient(mockDockerClient))
		h.AssertNil(t, err)

		fakeImage = fakes.NewImage("some/image", "", nil)
		h.AssertNil(t, fakeImage.SetLabel("io.buildpacks.stack.id", "test.stack.id"))
		h.AssertNil(t, fakeImage.SetLabel(
			"io.buildpacks.lifecycle.metadata",
			`{
  "app": [
    {
      "sha": "sha256:app-sha"
    }
  ],
  "config": {
    "sha": "sha256:config-sha"
  },
  "launcher": {
    "sha": "sha256:launcher-sha"
  },
  "buildpacks": [
    {
      "key": "buildpack1",
      "version": "1.2.3",
      "layers": {
        "firstLayer": {
          "sha": "sha256:buildpack1-firstLayer-Sha256",
          "build": true,
          "launch": true,
          "cache": true
        }
      }
    },
    {
      "key": "buildpack2",
      "version": "4.5.6",
      "layers": {
        "firstLayer": {
          "sha": "sha256:buildpack2-firstLayer-Sha256",
          "build": false,
          "launch": true,
          "cache": false
        }
      }
    }
  ],
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
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/image", true, config.PullNever).Return(fakeImage, nil)
					} else {
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/image", false, config.PullNever).Return(fakeImage, nil)
					}

					testImageReader, err := newTestImageReader(filepath.Join("testdata", "test-image.tar.gz"))
					h.AssertNil(t, err)

					mockDockerClient.EXPECT().ImageSave(gomock.Any(), []string{"some/image"}).Return(testImageReader, nil)

				})
				when("reading metadata", func() {
					it("reads the app metadata", func() {
						result, err := subject.Dive("some/image", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, len(result.LayerLookupInfo.App), 1)
						h.AssertEq(t, result.LayerLookupInfo.App[0].SHA, `sha256:app-sha`)
					})

					//TODO: other metadata validation
				})
				when("reading image layer data", func() {
					it("has expected layers metadata", func() {
						result, err := subject.Dive("some/image", useDaemon)
						h.AssertNil(t, err)
						h.AssertEq(t, len(result.Image.Layers), 3)
						h.AssertEq(t, result.Image.Layers[0].Digest, "sha256:50644c29ef5a27c9a40c393a73ece2479de78325cae7d762ef3cdc19bf42dd0a")
						h.AssertEq(t, result.Image.Layers[1].Digest, "sha256:8afef23f86669b2edfc71e052427c4e6ef74f9b8b586e8e7732ef2dbb602a401")
						h.AssertEq(t, result.Image.Layers[2].Digest, "sha256:f250386a9c84c89a938f11dac8769830bcbc4f76aa5db6968c945de2de94213c")
					})
				})
			})
		}
	})
}

func newTestImageReader(path string) (io.ReadCloser, error) {
	file, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, err
	}
	gReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	return ioutil2.NewReadCloser(gReader, file), nil
}
