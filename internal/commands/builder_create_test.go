package commands_test

import (
	"bytes"
	"fmt"
	"os"
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
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

const validConfig = `
[[buildpacks]]
  id = "some.buildpack"

[[order]]
	[[order.group]]
		id = "some.buildpack"

`

const validConfigWithExtensions = `
[[buildpacks]]
  id = "some.buildpack"

[[extensions]]
  id = "some.extension"

[[order]]
	[[order.group]]
		id = "some.buildpack"

[[order-extensions]]
	[[order-extensions.group]]
		id = "some.extension"

`

func TestCreateCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "CreateCommand", testCreateCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateCommand(t *testing.T, when spec.G, it spec.S) {
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
		tmpDir, err = os.MkdirTemp("", "create-builder-test")
		h.AssertNil(t, err)
		builderConfigPath = filepath.Join(tmpDir, "builder.toml")
		cfg = config.Config{}

		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)
		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
		command = commands.BuilderCreate(logger, cfg, mockClient)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#Create", func() {
		when("both --publish and pull-policy=never flags are specified", func() {
			it("errors with a descriptive message", func() {
				command.SetArgs([]string{
					"some/builder",
					"--config", "some-config-path",
					"--publish",
					"--pull-policy",
					"never",
				})
				err := command.Execute()
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "--publish and --pull-policy never cannot be used together. The --publish flag requires the use of remote images.")
			})
		})

		when("--pull-policy", func() {
			it("returns error for unknown policy", func() {
				command.SetArgs([]string{
					"some/builder",
					"--config", builderConfigPath,
					"--pull-policy", "unknown-policy",
				})
				h.AssertError(t, command.Execute(), "parsing pull policy")
			})
		})

		when("--pull-policy is not specified", func() {
			when("configured pull policy is invalid", func() {
				it("errors when config set with unknown policy", func() {
					cfg = config.Config{PullPolicy: "unknown-policy"}
					command = commands.BuilderCreate(logger, cfg, mockClient)
					command.SetArgs([]string{
						"some/builder",
						"--config", builderConfigPath,
					})
					h.AssertError(t, command.Execute(), "parsing pull policy")
				})
			})
		})

		when("--buildpack-registry flag is specified but experimental isn't set in the config", func() {
			it("errors with a descriptive message", func() {
				command.SetArgs([]string{
					"some/builder",
					"--config", "some-config-path",
					"--buildpack-registry", "some-registry",
				})
				err := command.Execute()
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "Support for buildpack registries is currently experimental.")
			})
		})

		when("warnings encountered in builder.toml", func() {
			it.Before(func() {
				h.AssertNil(t, os.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
  id = "some.buildpack"
`), 0666))
			})

			it("logs the warnings", func() {
				mockClient.EXPECT().CreateBuilder(gomock.Any(), gomock.Any()).Return(nil)

				command.SetArgs([]string{
					"some/builder",
					"--config", builderConfigPath,
				})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), "Warning: builder configuration: empty 'order' definition")
			})
		})

		when("uses --config", func() {
			it.Before(func() {
				h.AssertNil(t, os.WriteFile(builderConfigPath, []byte(validConfig), 0666))
			})

			when("buildConfigEnv files generated", func() {
				const ActionNONE = validConfig + `
				[[build.env]]
				name = "actionNone"
				value = "actionNoneValue"
				action = ""
				`
				const ActionDEFAULT = validConfig + `
				[[build.env]]
				name = "actionDefault"
				value = "actionDefaultValue"
				action = "default"
				`
				const ActionOVERRIDE = validConfig + `
				[[build.env]]
				name = "actionOverride"
				value = "actionOverrideValue"
				action = "override"
				`
				const ActionAPPEND = validConfig + `
				[[build.env]]
				name = "actionAppend"
				value = "actionAppendValue"
				action = "append"
				`
				const ActionPREPEND = validConfig + `
				[[build.env]]
				name = "actionPrepend"
				value = "actionPrependValue"
				action = "prepend"
				`
				const ActionDELIMIT = validConfig + `
				[[build.env]]
				name = "actionDelimit"
				value = "actionDelimitValue"
				action = "delim"
				`
				const ActionUNKNOWN = validConfig + `
				[[build.env]]
				name = "actionUnknown"
				value = "actionUnknownValue"
				action = "unknown"
				`
				const ActionMULTIPLE = validConfig + `
				[[build.env]]
				name = "actionAppend"
				value = "actionAppendValue"
				action = "append"
				[[build.env]]
				name = "actionPrepend"
				value = "actionPrependValue"
				action = "prepend"
				[[build.env]]
				name = "actionDelimit"
				value = "actionDelimitValue"
				action = "delim"
				`
				it("should create content as expected when ActionType `NONE`", func() {

				})
				it("should create content as expected when ActionType `DEFAULT`", func() {

				})
				it("should create content as expected when ActionType `OVERRIDE`", func() {

				})
				it("should create content as expected when ActionType `APPEND`", func() {

				})
				it("should create content as expected when ActionType `PREPEND`", func() {

				})
				it("should create content as expected when ActionType `DELIMIT`", func() {

				})
				it("should create content as expected when unknown ActionType passed", func() {

				})
				it("should create content as expected when multiple ActionTypes passed", func() {

				})
			})

			it("errors with a descriptive message", func() {
				command.SetArgs([]string{
					"some/builder",
					"--config", builderConfigPath,
				})
				h.AssertError(t, command.Execute(), "unknown flag: --builder-config")
			})
		})

		when("no config provided", func() {
			it("errors with a descriptive message", func() {
				command.SetArgs([]string{
					"some/builder",
				})
				h.AssertError(t, command.Execute(), "Please provide a builder config path")
			})
		})

		when("builder config has extensions but experimental isn't set in the config", func() {
			it.Before(func() {
				h.AssertNil(t, os.WriteFile(builderConfigPath, []byte(validConfigWithExtensions), 0666))
			})

			it("errors", func() {
				command.SetArgs([]string{
					"some/builder",
					"--config", builderConfigPath,
				})
				h.AssertError(t, command.Execute(), "builder config contains image extensions; support for image extensions is currently experimental")
			})
		})

		when("flatten is set to true", func() {
			it.Before(func() {
				h.AssertNil(t, os.WriteFile(builderConfigPath, []byte(validConfig), 0666))
			})

			when("flatten exclude doesn't have format <buildpack>@<version>", func() {
				it("errors with a descriptive message", func() {
					command.SetArgs([]string{
						"some/builder",
						"--config", builderConfigPath,
						"--flatten",
						"--flatten-exclude", "some-buildpack",
					})
					h.AssertError(t, command.Execute(), fmt.Sprintf("invalid format %s; please use '<buildpack-id>@<buildpack-version>' to exclude buildpack from flattening", "some-buildpack"))
				})
			})
		})
	})
}
