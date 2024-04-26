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

func TestManifestAddCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestAddCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testManifestAddCommand(t *testing.T, when spec.G, it spec.S) {
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
		command = commands.ManifestAdd(logger, mockClient)
	})

	when("args are valid", func() {
		it.Before(func() {
			prepareAddManifest(t, mockClient)
		})

		it("should add image with current platform specs", func() {
			command.SetArgs([]string{"some-index", "busybox:1.36-musl"})
			err := command.Execute()
			h.AssertNil(t, err)
			h.AssertEq(t, outBuf.String(), "")
		})

		it("should have help flag", func() {
			prepareAnnotateManifest(t, mockClient)

			command.SetArgs([]string{"--help"})
			h.AssertNilE(t, command.Execute())
			h.AssertEq(t, outBuf.String(), "")
		})
	})

	when("args are invalid", func() {
		it("error when missing mandatory arguments", func() {
			command.SetArgs([]string{"some-index"})
			err := command.Execute()
			h.AssertNotNil(t, err)
			h.AssertError(t, err, "accepts 2 arg(s), received 1")
		})
	})
}

func prepareAddManifest(_ *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		AddManifest(
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		Return(nil)
}
