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

	when("#ManifestCreate", func() {
		when("no flags specified", func() {

		})
		when("when --all flags passed", func() {

		})
		when("when --os flags passed", func() {

		})
		when("when --arch flags passed", func() {

		})
		when("when --registry flags passed", func() {

		})
		when("when --format flags passed", func() {

		})
		when("when --format flags not passed", func() {

		})
		when("when --insecure flags passed", func() {

		})
		when("when --publish flags passed", func() {

		})
		when("when --ammend flags passed", func() {

		})
		when("when multiple flags passed", func() {

		})
		when("when no args passed", func() {

		})
		when("when manifest list reference is incorrect", func() {

		})
		when("when manifest reference is incorrect", func() {

		})
		when("when manifest list is passed on second arg", func() {

		})
		when("when manifest is passed on second arg with --all option", func() {

		})
		when("when manifest list is locally available", func() {

		})
		when("when manifest list is not locally available", func() {

		})
		when("when manifest is not locally available", func() {

		})
		when("when manifest is locally available passed", func() {

		})
		when("when multiple manifests passed", func() {

		})
	})
}
