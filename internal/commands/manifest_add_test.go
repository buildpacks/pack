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

	spec.Run(t, "Commands", testManifestAddCommand, spec.Random(), spec.Report(report.Terminal{}))
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
	it("should add image with current platform specs", func() {
		prepareAddManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox:1.36-musl"})
		err := command.Execute()
		h.AssertNil(t, err)
		h.AssertEq(t, outBuf.String(), "")
	})
	it("should add images with given platform", func() {
		prepareAddManifest(t, mockClient)

		command.SetArgs([]string{
			"some-index",
			"busybox:1.36-musl",
			"--os",
			"linux",
			"--arch",
			"arm",
			"--variant",
			"v6",
			"--os-version",
			"22.04",
		})
		err := command.Execute()
		h.AssertNil(t, err)
		h.AssertEq(t, outBuf.String(), "")
	})
	it("should add return an error when platform's os and arch not defined", func() {
		prepareAddManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox:1.36-musl", "--os", "linux"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "'os' or 'arch' is undefined")
		h.AssertEq(t, outBuf.String(), "ERROR: 'os' or 'arch' is undefined\n")
	})
	it("should add all images", func() {
		prepareAddManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox:1.36-musl", "--all"})
		err := command.Execute()
		h.AssertNil(t, err)
		h.AssertEq(t, outBuf.String(), "")
	})
	it("should return an error when features defined invalidly", func() {
		prepareAddManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox:1.36-musl", "--features"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "flag needs an argument: --features")
	})
	it("should return an error when osFeatures defined invalidly", func() {
		prepareAddManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox:1.36-musl", "--os-features"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "flag needs an argument: --os-features")
	})
	it("should return an error when invalid arg passed", func() {
		prepareAddManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox:1.36-musl", "--urls"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "unknown flag: --urls")
	})
	it("should return an error when annotations defined invalidly", func() {
		prepareAddManifest(t, mockClient)

		command.SetArgs([]string{"some-index", "busybox:1.36-musl", "--annotations", "some-key"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), `invalid argument "some-key" for "--annotations" flag: some-key must be formatted as key=value`)
	})
	it("should have help flag", func() {
		prepareAddManifest(t, mockClient)

		command.SetArgs([]string{"--help"})
		h.AssertNilE(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
}

func prepareAddManifest(t *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		AddManifest(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		Return(nil)
}
