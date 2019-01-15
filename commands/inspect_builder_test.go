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
	"github.com/buildpack/pack/commands/mocks"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCommands(t *testing.T) {
	spec.Run(t, "Commands", testCommands, spec.Parallel(), spec.Report(report.Terminal{}))
}

//move somewhere else
//go:generate mockgen -package mocks -destination mocks/image.go github.com/buildpack/lifecycle/image Image

func testCommands(t *testing.T, when spec.G, it spec.S) {

	var (
		command          *cobra.Command
		logger           *logging.Logger
		outBuf           bytes.Buffer
		mockInspector    *mocks.MockBuilderInspector
		mockController   *gomock.Controller
		mockImageFactory *mocks.MockImageFactory
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockInspector = mocks.NewMockBuilderInspector(mockController)
		mockImageFactory = mocks.NewMockImageFactory(mockController)

		logger = logging.NewLogger(&outBuf, &outBuf, false, false)
	})

	when("#InspectBuilder", func() {
		when("image cannot be found", func() {
			it.Before(func() {
				command = commands.InspectBuilder(logger, mockInspector, mockImageFactory)
			})

			it("logs 'Not present'", func() {
				mockImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("some/image", false).Return(mockImage, nil)
				mockImageFactory.EXPECT().NewRemote("some/image").Return(mockImage, nil)
				mockImage.EXPECT().Found().Return(false, nil).AnyTimes()
				command.SetArgs([]string{
					"some/image",
				})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), "Remote\n------\nNot present\n\nLocal\n-----\nNot present\n")
			})
		})

		when("image factory returns an error", func() {
			it.Before(func() {
				command = commands.InspectBuilder(logger, mockInspector, mockImageFactory)
			})

			it("logs the error message", func() {
				mockImageFactory.EXPECT().NewLocal("some/image", false).Return(nil, errors.New("some local error"))
				mockImageFactory.EXPECT().NewRemote("some/image").Return(nil, errors.New("some remote error"))
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
			it.Before(func() {
				command = commands.InspectBuilder(logger, mockInspector, mockImageFactory)
			})

			it("displays the run image information for local and remote", func() {
				mockRemoteImage := mocks.NewMockImage(mockController)
				mockLocalImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("some/image", false).Return(mockLocalImage, nil)
				mockImageFactory.EXPECT().NewRemote("some/image").Return(mockRemoteImage, nil)

				mockRemoteImage.EXPECT().Found().Return(true, nil)
				mockLocalImage.EXPECT().Found().Return(true, nil)

				mockInspector.EXPECT().Inspect(mockRemoteImage).Return(pack.Builder{
					LocalRunImages:   []string{"first/image", "second/image"},
					DefaultRunImages: []string{"first/default", "second/default"},
				}, nil)

				mockInspector.EXPECT().Inspect(mockRemoteImage).Return(pack.Builder{
					LocalRunImages:   []string{"first/local", "second/local"},
					DefaultRunImages: []string{"first/local-default", "second/local-default"},
				}, nil)

				command.SetArgs([]string{
					"some/image",
				})

				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), `Remote
------
Run Images:
	first/image (user-configured)
	second/image (user-configured)
	first/default
	second/default

Local
-----
Run Images:
	first/local (user-configured)
	second/local (user-configured)
	first/local-default
	second/local-default
`)
			})
		})
	})
}
