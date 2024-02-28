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

func TestManifestCreateCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestCreateCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testManifestCreateCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.ManifestCreate(logger, mockClient)
	})
	it("should annotate images with given flags", func() {
		prepareCreateManifest(t, mockClient)

		command.SetArgs([]string{
			"some-index",
			"busybox@sha256:6457d53fb065d6f250e1504b9bc42d5b6c65941d57532c072d929dd0628977d0",
			"--os",
			"linux",
			"--arch",
			"arm",
			"--format",
			"v2s2",
			"--insecure",
			"--publish",
		})
		h.AssertNil(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
	it("should return an error when platform's os and arch not defined", func() {
		prepareCreateManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox@sha256:6457d53fb065d6f250e1504b9bc42d5b6c65941d57532c072d929dd0628977d0", "--os", "linux"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "'os' or 'arch' is undefined")
		h.AssertEq(t, outBuf.String(), "ERROR: 'os' or 'arch' is undefined\n")
	})
	it("should have help flag", func() {
		prepareCreateManifest(t, mockClient)

		command.SetArgs([]string{"--help"})
		h.AssertNilE(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
}

func prepareCreateManifest(t *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		CreateManifest(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		Return(nil)
}
