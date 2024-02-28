package commands_test

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestManifestExistsCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestExistsCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testManifestExistsCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.ManifestExists(logger, mockClient)
	})
	it("should annotate images with given flags", func() {
		prepareExistsManifest(t, mockClient)

		command.SetArgs([]string{
			"some-index",
		})
		h.AssertNil(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
	it("should return an error when index doesn't exists", func() {
		prepareNotExistsManifest(t, mockClient)

		command.SetArgs([]string{"some-other-index"})
		err := command.Execute()
		h.AssertEq(t, err.Error(), "no index found with given name")
	})
	it("should have help flag", func() {
		prepareExistsManifest(t, mockClient)

		command.SetArgs([]string{"--help"})
		h.AssertNilE(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
}

func prepareExistsManifest(t *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		ExistsManifest(
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		Return(nil)
}

func prepareNotExistsManifest(t *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		ExistsManifest(
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		Return(errors.New("no index found with given name"))
}
