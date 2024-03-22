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

func TestManifestDeleteCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestDeleteCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testManifestDeleteCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.ManifestDelete(logger, mockClient)
	})
	it("should delete index", func() {
		prepareDeleteManifest(mockClient)

		command.SetArgs([]string{
			"some-index",
		})
		h.AssertNil(t, command.Execute())
	})
	it("should have help flag", func() {
		command.SetArgs([]string{"--help"})
		h.AssertNilE(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
	it("should return an error", func() {
		prepareDeleteManifest(mockClient)

		command.SetArgs([]string{"some-index"})
		err := command.Execute()
		h.AssertNil(t, err)

		err = command.Execute()
		h.AssertNotNil(t, err)
	})
}

func prepareDeleteManifest(mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		DeleteManifest(
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		After(
			mockClient.
				EXPECT().
				DeleteManifest(
					gomock.Any(),
					gomock.Any(),
				).
				Times(1).
				Return(nil),
		).
		Return([]error{
			errors.New("image index doesn't exists"),
		})
}
