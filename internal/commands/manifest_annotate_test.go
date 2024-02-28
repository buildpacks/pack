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
	it("should annotate images with given flags", func() {
		prepareAnnotateManifest(t, mockClient)

		command.SetArgs([]string{
			"some-index",
			"busybox@sha256:6457d53fb065d6f250e1504b9bc42d5b6c65941d57532c072d929dd0628977d0",
			"--os",
			"linux",
			"--arch",
			"arm",
			"--variant",
			"v6",
			"--os-version",
			"22.04",
		})
		h.AssertNil(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
	it("should return an error when platform's os and arch not defined", func() {
		prepareAnnotateManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox@sha256:6457d53fb065d6f250e1504b9bc42d5b6c65941d57532c072d929dd0628977d0", "--os", "linux"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "'os' or 'arch' is undefined")
		h.AssertEq(t, outBuf.String(), "ERROR: 'os' or 'arch' is undefined\n")
	})
	it("should return an error when features defined invalidly", func() {
		prepareAnnotateManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox@sha256:6457d53fb065d6f250e1504b9bc42d5b6c65941d57532c072d929dd0628977d0", "--features"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "flag needs an argument: --features")
	})
	it("should return an error when osFeatures defined invalidly", func() {
		prepareAnnotateManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox@sha256:6457d53fb065d6f250e1504b9bc42d5b6c65941d57532c072d929dd0628977d0", "--os-features"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "flag needs an argument: --os-features")
	})
	it("should return an error when urls defined invalidly", func() {
		prepareAnnotateManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox@sha256:6457d53fb065d6f250e1504b9bc42d5b6c65941d57532c072d929dd0628977d0", "--urls"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "flag needs an argument: --urls")
	})
	it("should return an error when annotations defined invalidly", func() {
		prepareAnnotateManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox@sha256:6457d53fb065d6f250e1504b9bc42d5b6c65941d57532c072d929dd0628977d0", "--annotations", "some-key="})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "key(some-key) or value() is undefined")
	})
	it("should have help flag", func() {
		prepareAnnotateManifest(t, mockClient)

		command.SetArgs([]string{"--help"})
		h.AssertNilE(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
}

func prepareAnnotateManifest(t *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		AnnotateManifest(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		Return(nil)
}
