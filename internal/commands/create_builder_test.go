package commands_test

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestCreateBuilderCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Commands", testCreateBuilderCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateBuilderCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command           *cobra.Command
		logger            logging.Logger
		outBuf            bytes.Buffer
		mockController    *gomock.Controller
		mockClient        *testmocks.MockPackClient
		tmpDir            string
		builderConfigPath string
		cfg               config.Config
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "create-builder-test")
		h.AssertNil(t, err)
		builderConfigPath = filepath.Join(tmpDir, "builder.toml")
		cfg = config.Config{}

		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		command = commands.CreateBuilder(logger, cfg, mockClient)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#CreateBuilder", func() {
		when("both --publish and --no-pull flags are specified", func() {
			it("errors with a descriptive message", func() {
				command.SetArgs([]string{
					"some/builder",
					"--builder-config", "some-config-path",
					"--publish",
					"--no-pull",
				})
				err := command.Execute()
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "The --publish and --no-pull flags cannot be used together. The --publish flag requires the use of remote images.")
			})
		})

		when("--buildpack-registry flag is specified but experimental isn't set in the config", func() {
			it("errors with a descriptive message", func() {
				command.SetArgs([]string{
					"some/builder",
					"--builder-config", "some-config-path",
					"--buildpack-registry", "some-registry",
				})
				err := command.Execute()
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "Support for buildpack registries is currently experimental.\nTip: To enable experimental features, add `experimental = true` to {{CONFIG_PATH}}.")
			})
		})

		when("warnings encountered in builder.toml", func() {
			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
  id = "some.buildpack"
`), 0666))
			})

			it("logs the warnings", func() {
				mockClient.EXPECT().CreateBuilder(gomock.Any(), gomock.Any()).Return(nil)

				command.SetArgs([]string{
					"some/builder",
					"--builder-config", builderConfigPath,
				})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), "Warning: builder configuration: empty 'order' definition")
			})
		})
	})
}
