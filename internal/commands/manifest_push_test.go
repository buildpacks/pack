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

func TestManifestPushCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestPushCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testManifestPushCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.ManifestPush(logger, mockClient)
	})
	it("should annotate images with given flags", func() {
		preparePushManifest(t, mockClient)

		command.SetArgs([]string{
			"some-index",
			"-f",
			"v2s2",
			"--purge",
			"--insecure",
		})
		h.AssertNil(t, command.Execute())
	})
	it("should return an error when index not exists locally", func() {
		preparePushManifestWithError(t, mockClient)

		command.SetArgs([]string{"some-index"})
		err := command.Execute()
		h.AssertNotNil(t, err)
	})
	it("should have help flag", func() {
		preparePushManifest(t, mockClient)

		command.SetArgs([]string{"--help"})
		h.AssertNilE(t, command.Execute())
		h.AssertEq(t, outBuf.String(), "")
	})
}

func preparePushManifest(_ *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		PushManifest(
			gomock.Any(),
		).
		AnyTimes().
		Return(nil)
}

func preparePushManifestWithError(_ *testing.T, mockClient *testmocks.MockPackClient) {
	mockClient.
		EXPECT().
		PushManifest(
			gomock.Any(),
		).
		AnyTimes().
		Return(errors.New("unable to push Image"))
}
