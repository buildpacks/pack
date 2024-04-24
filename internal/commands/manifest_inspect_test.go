package commands_test

import (
	"bytes"
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

func TestManifestInspectCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestInspectCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testManifestInspectCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.ManifestInspect(logger, mockClient)
	})
	it("should annotate images with given flags", func() {
		prepareInspectManifest(t, mockClient)

		command.SetArgs([]string{
			"some-index",
		})
		h.AssertNil(t, command.Execute())
	})
	it("should return an error when index not passed", func() {
		prepareInspectManifest(t, mockClient)

		command.SetArgs([]string(nil))
		err := command.Execute()
		h.AssertNotNil(t, err)
	})
	it("should have help flag", func() {
		prepareInspectManifest(t, mockClient)

		command.SetArgs([]string{"--help"})
		h.AssertNilE(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
}

func prepareInspectManifest(t *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		InspectManifest(
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		Return(nil)
}
