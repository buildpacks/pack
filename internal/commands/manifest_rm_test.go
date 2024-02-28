package commands_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestManifestRemoveCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestRemoveCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testManifestRemoveCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         *logging.LogWithWriters
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
	)

	it.Before(func() {
		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)

		command = commands.ManifestRemove(logger, mockClient)
	})
	it("should remove index", func() {
		prepareRemoveManifest(t, mockClient)

		command.SetArgs([]string{
			"some-index",
			"some-image",
		})
		h.AssertNil(t, command.Execute())
	})
	it("should return an error", func() {
		prepareRemoveManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "some-image"})
		err := command.Execute()
		h.AssertNil(t, err)

		err = command.Execute()
		h.AssertNotNil(t, err)
	})
	it("should have help flag", func() {
		command.SetArgs([]string{"--help"})
		h.AssertNilE(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
}

func prepareRemoveManifest(t *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		RemoveManifest(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		After(
			mockClient.
				EXPECT().
				RemoveManifest(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).
				Times(1).
				Return(nil),
		).
		Return([]error{
			errors.New("image doesn't exists"),
		})
}
