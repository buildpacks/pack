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

	when("#ManifestExists", func() {
		when("only one arg is passed", func() {

		})
		when("when more than one arg passed", func() {

		})
		when("when passed arg is manifest list", func() {
			it("if exists locally", func() {

			})
			it("if exists at registry only", func() {

			})
		})
		when("when passed arg is manifest", func() {

		})
	})
}