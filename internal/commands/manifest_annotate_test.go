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

func TestManifestAnnotationsCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestAnnotateCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testManifestAnnotateCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.ManifestAnnotate(logger, mockClient)
	})

	when("args are valid", func() {
		it.Before(func() {
			prepareAnnotateManifest(t, mockClient)
		})

		it("should annotate images with given flags", func() {
			command.SetArgs([]string{
				"some-index",
				"busybox@sha256:6457d53fb065d6f250e1504b9bc42d5b6c65941d57532c072d929dd0628977d0",
				"--os",
				"linux",
				"--arch",
				"arm",
				"--variant",
				"v6",
			})
			h.AssertNilE(t, command.Execute())
		})

		it("should have help flag", func() {
			command.SetArgs([]string{"--help"})
			h.AssertNilE(t, command.Execute())
		})
	})

	when("args are invalid", func() {
		it("error when missing mandatory arguments", func() {
			command.SetArgs([]string{"some-index"})
			err := command.Execute()
			h.AssertNotNil(t, err)
			h.AssertError(t, err, "accepts 2 arg(s), received 1")
		})

		it("should return an error when annotations defined invalidly", func() {
			command.SetArgs([]string{"some-index", "busybox@sha256:6457d53fb065d6f250e1504b9bc42d5b6c65941d57532c072d929dd0628977d0", "--annotations", "some-key"})
			err := command.Execute()
			h.AssertEq(t, err.Error(), `invalid argument "some-key" for "--annotations" flag: some-key must be formatted as key=value`)
		})
	})
}

func prepareAnnotateManifest(_ *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		AnnotateManifest(
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		Return(nil)
}
