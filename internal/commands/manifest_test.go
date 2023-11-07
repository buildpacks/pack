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

func TestManifestCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testManifestCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.NewManifestCommand(logger, mockClient)
	})

	when("#ManifestAdd", func() {
		when("no flags specified", func() {

		})
		when("add is passed as flag", func() {

		})
		when("create is passed as flag", func() {
			
		})
		when("annotate is passed as flag", func() {
			
		})
		when("remove is passed as flag", func() {
			
		})
		when("inspect is passed as flag", func() {
			
		})
		when("rm is passed as flag", func() {
			
		})
		when("push is passed as flag", func() {
			
		})
	})
}
