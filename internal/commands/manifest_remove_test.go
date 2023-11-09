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

	when("#ManifestRemove", func() {
		when("when no args passed", func() {

		})
		when("when manifest list reference is incorrect", func() {

		})
		when("when manifest passed in-place of manifest list on first arg", func() {

		})
		when("when manifest list is locally available", func() {

		})
		when("when manifest list is not locally available", func() {

		})
		when("when multiple manifests passed", func() {

		})
	})
}
