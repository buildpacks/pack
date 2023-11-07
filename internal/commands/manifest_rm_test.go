package commands_test

import (
	"bytes"
	"testing"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"
)

func TestManifestRemoveCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestRemoveCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testManifestRemoveCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.ManifestRemove(logger, mockClient)
	})

	when("#ManifestRm", func() {
		when("when no args passed", func() {

		})
		when("when manifest list reference is incorrect", func() {

		})
		when("when manifest reference is incorrect", func() {

		})
		when("when manifest passed in-place of manifest list on first arg", func() {

		})
		when("when manifest list is passed on second arg", func() {

		})
		when("when manifest is passed on second arg", func() {

		})
		when("when manifest list is locally available", func() {

		})
		when("when manifest list is not locally available", func() {

		})
		when("when manifest is locally available", func() {

		})
		when("when manifest is not locally available", func() {

		})
		when("when multiple manifests passed", func() {

		})
	})
}
