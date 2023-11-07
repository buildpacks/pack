package client_test

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

	when("#AddManifest", func() {
		when("no flags specified", func() {

		})
		when("when --all flags passed", func() {

		})
		when("when --os flags passed", func() {

		})
		when("when --arch flags passed", func() {

		})
		when("when --variant flags passed", func() {

		})
		when("when --os-version flags passed", func() {

		})
		when("when --features flags passed", func() {

		})
		when("when --os-features flags passed", func() {

		})
		when("when --annotations flags passed", func() {

		})
		when("when multiple flags passed", func() {

		})
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
		when("when manifest is passed on second arg with --all option", func() {

		})
		when("when manifest list in locally available", func() {

		})
		when("when manifest is not locally available", func() {

		})
		when("when manifest is locally available passed", func() {

		})
		when("when multiple manifests passed", func() {

		})
	})
}
