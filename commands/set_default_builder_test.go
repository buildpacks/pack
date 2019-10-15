package commands_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/commands"
	cmdmocks "github.com/buildpack/pack/commands/mocks"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/internal/fakes"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestSetDefaultBuilder(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "Commands", testSetDefaultBuilderCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testSetDefaultBuilderCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         logging.Logger
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *cmdmocks.MockPackClient
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockClient = cmdmocks.NewMockPackClient(mockController)
		logger = fakes.NewFakeLogger(&outBuf)
		command = commands.SetDefaultBuilder(logger, config.Config{}, mockClient)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#SetDefaultBuilder", func() {
		when("no builder provided", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectBuilder(gomock.Any(), false).Return(&pack.BuilderInfo{}, nil).AnyTimes()
			})

			it("display suggested builders", func() {
				command.SetArgs([]string{})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Suggested builders:")
			})
		})

		when("empty builder name is provided", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectBuilder(gomock.Any(), false).Return(&pack.BuilderInfo{}, nil).AnyTimes()
			})

			it("display suggested builders", func() {
				command.SetArgs([]string{})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Suggested builders:")
			})
		})

		when("valid builder is provider", func() {
			when("in local", func() {
				it("sets default builder", func() {
					imageName := "some/image"
					mockClient.EXPECT().InspectBuilder(imageName, true).Return(&pack.BuilderInfo{
						Stack: "test.stack.id",
					}, nil)

					command.SetArgs([]string{imageName})
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, outBuf.String(), fmt.Sprintf("Builder '%s' is now the default builder", imageName))
				})
			})

			when("in remote", func() {
				it("sets default builder", func() {
					imageName := "some/image"

					localCall := mockClient.EXPECT().InspectBuilder(imageName, true).Return(nil, nil)

					mockClient.EXPECT().InspectBuilder(imageName, false).Return(&pack.BuilderInfo{
						Stack: "test.stack.id",
					}, nil).After(localCall)

					command.SetArgs([]string{imageName})
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, outBuf.String(), fmt.Sprintf("Builder '%s' is now the default builder", imageName))
				})
			})
		})

		when("invalid builder is provided", func() {
			it("error is presented", func() {
				imageName := "nonbuilder/image"

				mockClient.EXPECT().InspectBuilder(imageName, true).Return(
					nil,
					fmt.Errorf("failed to inspect image %s", imageName))

				command.SetArgs([]string{imageName})

				h.AssertNotNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "ERROR: failed to inspect image nonbuilder/image")
			})
		})

		when("non-existent builder is provided", func() {
			it("error is present", func() {
				imageName := "nonexisting/image"

				localCall := mockClient.EXPECT().InspectBuilder(imageName, true).Return(
					nil,
					nil)

				mockClient.EXPECT().InspectBuilder(imageName, false).Return(
					nil,
					nil).After(localCall)

				command.SetArgs([]string{imageName})

				h.AssertNotNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "ERROR: builder 'nonexisting/image' not found")
			})
		})
	})
}
