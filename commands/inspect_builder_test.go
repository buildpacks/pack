package commands_test

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/commands"
	cmdmocks "github.com/buildpack/pack/commands/mocks"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCommands(t *testing.T) {
	spec.Run(t, "Commands", testCommands, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCommands(t *testing.T, when spec.G, it spec.S) {

	var (
		command        *cobra.Command
		logger         *logging.Logger
		outBuf         bytes.Buffer
		mockInspector  *cmdmocks.MockBuilderInspector
		mockController *gomock.Controller
		mockFetcher    *mocks.MockFetcher
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockInspector = cmdmocks.NewMockBuilderInspector(mockController)
		mockFetcher = mocks.NewMockFetcher(mockController)

		logger = logging.NewLogger(&outBuf, &outBuf, false, false)
		command = commands.InspectBuilder(logger, mockInspector, mockFetcher)
	})

	when("#InspectBuilder", func() {
		when("image cannot be found", func() {
			it("logs 'Not present'", func() {
				mockImage := mocks.NewMockImage(mockController)
				mockFetcher.EXPECT().FetchLocalImage("some/image").Return(mockImage, nil)
				mockFetcher.EXPECT().FetchRemoteImage("some/image").Return(mockImage, nil)
				mockImage.EXPECT().Found().Return(false, nil).AnyTimes()
				command.SetArgs([]string{
					"some/image",
				})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), "Remote\n------\nNot present\n\nLocal\n-----\nNot present\n")
			})
		})

		when("image fetcher returns an error", func() {
			it("logs the error message", func() {
				mockFetcher.EXPECT().FetchLocalImage("some/image").Return(nil, errors.New("some local error"))
				mockFetcher.EXPECT().FetchRemoteImage("some/image").Return(nil, errors.New("some remote error"))
				command.SetArgs([]string{
					"some/image",
				})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), `Remote
------
ERROR: failed to get image 'some/image': some remote error

Local
-----
ERROR: failed to get image 'some/image': some local error
`)
			})
		})

		when("is successful", func() {
			it("displays the run image information for local and remote", func() {
				mockRemoteImage := mocks.NewMockImage(mockController)
				mockLocalImage := mocks.NewMockImage(mockController)
				mockFetcher.EXPECT().FetchLocalImage("some/image").Return(mockLocalImage, nil)
				mockFetcher.EXPECT().FetchRemoteImage("some/image").Return(mockRemoteImage, nil)

				mockRemoteImage.EXPECT().Found().Return(true, nil)
				mockLocalImage.EXPECT().Found().Return(true, nil)

				mockInspector.EXPECT().Inspect(mockRemoteImage).Return(pack.Builder{
					RunImage:             "run/image",
					LocalRunImageMirrors: []string{"first/image", "second/image"},
					RunImageMirrors:      []string{"first/default", "second/default"},
				}, nil)

				mockInspector.EXPECT().Inspect(mockRemoteImage).Return(pack.Builder{
					RunImage:             "run/image",
					LocalRunImageMirrors: []string{"first/local", "second/local"},
					RunImageMirrors:      []string{"first/local-default", "second/local-default"},
				}, nil)

				command.SetArgs([]string{
					"some/image",
				})

				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), `Remote
------
Run Image: run/image
Run Image Mirrors:
	first/image (user-configured)
	second/image (user-configured)
	first/default
	second/default

Local
-----
Run Image: run/image
Run Image Mirrors:
	first/local (user-configured)
	second/local (user-configured)
	first/local-default
	second/local-default

`)
			})
		})
	})
}
