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

	"github.com/buildpacks/pack/builder"
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

var BuildConfigEnvSuffixNone = builder.BuildConfigEnv{
	Name:  "suffixNone",
	Value: "suffixNoneValue",
}

var BuildConfigEnvSuffixNoneWithEmptySuffix = builder.BuildConfigEnv{
	Name:   "suffixNoneWithEmptySuffix",
	Value:  "suffixNoneWithEmptySuffixValue",
	Suffix: "",
}

var BuildConfigEnvSuffixDefault = builder.BuildConfigEnv{
	Name:   "suffixDefault",
	Value:  "suffixDefaultValue",
	Suffix: "default",
}

var BuildConfigEnvSuffixOverride = builder.BuildConfigEnv{
	Name:   "suffixOverride",
	Value:  "suffixOverrideValue",
	Suffix: "override",
}

var BuildConfigEnvSuffixAppend = builder.BuildConfigEnv{
	Name:   "suffixAppend",
	Value:  "suffixAppendValue",
	Suffix: "append",
	Delim:  ":",
}

var BuildConfigEnvSuffixPrepend = builder.BuildConfigEnv{
	Name:   "suffixPrepend",
	Value:  "suffixPrependValue",
	Suffix: "prepend",
	Delim:  ":",
}

var BuildConfigEnvDelimWithoutSuffix = builder.BuildConfigEnv{
	Name:  "delimWithoutSuffix",
	Delim: ":",
}

var BuildConfigEnvSuffixUnknown = builder.BuildConfigEnv{
	Name:   "suffixUnknown",
	Value:  "suffixUnknownValue",
	Suffix: "unknown",
}

var BuildConfigEnvSuffixMultiple = []builder.BuildConfigEnv{
	{
		Name:   "MY_VAR",
		Value:  "suffixAppendValueValue",
		Suffix: "append",
		Delim:  ";",
	},
	{
		Name:   "MY_VAR",
		Value:  "suffixDefaultValue",
		Suffix: "default",
		Delim:  "%",
	},
	{
		Name:   "MY_VAR",
		Value:  "suffixPrependValue",
		Suffix: "prepend",
		Delim:  ":",
	},
}

var BuildConfigEnvEmptyValue = builder.BuildConfigEnv{
	Name:  "warning",
	Value: "",
}

var BuildConfigEnvEmptyName = builder.BuildConfigEnv{
	Name:   "",
	Value:  "suffixUnknownValue",
	Suffix: "default",
}

var BuildConfigEnvSuffixPrependWithoutDelim = builder.BuildConfigEnv{
	Name:   "suffixPrepend",
	Value:  "suffixPrependValue",
	Suffix: "prepend",
}

var BuildConfigEnvDelimWithoutSuffixAppendOrPrepend = builder.BuildConfigEnv{
	Name:  "delimWithoutActionAppendOrPrepend",
	Value: "some-value",
	Delim: ":",
}

var BuildConfigEnvDelimWithSameSuffixAndName = []builder.BuildConfigEnv{
	{
		Name:   "MY_VAR",
		Value:  "some-value",
		Suffix: "",
	},
	{
		Name:  "MY_VAR",
		Value: "some-value",
	},
}

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

		when("uses --builder-config", func() {
			it.Before(func() {
				h.AssertNil(t, os.WriteFile(builderConfigPath, []byte(validConfig), 0666))
			})

			it("errors with a descriptive message", func() {
				command.SetArgs([]string{
					"some/builder",
					"--builder-config", builderConfigPath,
				})
				h.AssertError(t, command.Execute(), "unknown flag: --builder-config")
			})
		})

		when("#ParseBuildpackConfigEnv", func() {
			it("should create envMap as expected when suffix is omitted", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvSuffixNone}, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvSuffixNone.Name: BuildConfigEnvSuffixNone.Value,
				})
				h.AssertEq(t, len(warnings), 0)
				h.AssertNil(t, err)
			})
			it("should create envMap as expected when suffix is empty string", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvSuffixNoneWithEmptySuffix}, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvSuffixNoneWithEmptySuffix.Name: BuildConfigEnvSuffixNoneWithEmptySuffix.Value,
				})
				h.AssertEq(t, len(warnings), 0)
				h.AssertNil(t, err)
			})
			it("should create envMap as expected when suffix is `default`", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvSuffixDefault}, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvSuffixDefault.Name + "." + string(BuildConfigEnvSuffixDefault.Suffix): BuildConfigEnvSuffixDefault.Value,
				})
				h.AssertEq(t, len(warnings), 0)
				h.AssertNil(t, err)
			})
			it("should create envMap as expected when suffix is `override`", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvSuffixOverride}, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvSuffixOverride.Name + "." + string(BuildConfigEnvSuffixOverride.Suffix): BuildConfigEnvSuffixOverride.Value,
				})
				h.AssertEq(t, len(warnings), 0)
				h.AssertNil(t, err)
			})
			it("should create envMap as expected when suffix is `append`", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvSuffixAppend}, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvSuffixAppend.Name + "." + string(BuildConfigEnvSuffixAppend.Suffix): BuildConfigEnvSuffixAppend.Value,
					BuildConfigEnvSuffixAppend.Name + ".delim":                                        BuildConfigEnvSuffixAppend.Delim,
				})
				h.AssertEq(t, len(warnings), 0)
				h.AssertNil(t, err)
			})
			it("should create envMap as expected when suffix is `prepend`", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvSuffixPrepend}, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvSuffixPrepend.Name + "." + string(BuildConfigEnvSuffixPrepend.Suffix): BuildConfigEnvSuffixPrepend.Value,
					BuildConfigEnvSuffixPrepend.Name + ".delim":                                         BuildConfigEnvSuffixPrepend.Delim,
				})
				h.AssertEq(t, len(warnings), 0)
				h.AssertNil(t, err)
			})
			it("should create envMap as expected when delim is specified", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvDelimWithoutSuffix}, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvDelimWithoutSuffix.Name:            BuildConfigEnvDelimWithoutSuffix.Value,
					BuildConfigEnvDelimWithoutSuffix.Name + ".delim": BuildConfigEnvDelimWithoutSuffix.Delim,
				})
				h.AssertNotEq(t, len(warnings), 0)
				h.AssertNil(t, err)
			})
			it("should create envMap with a warning when `value` is empty", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvEmptyValue}, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvEmptyValue.Name: BuildConfigEnvEmptyValue.Value,
				})
				h.AssertNotEq(t, len(warnings), 0)
				h.AssertNil(t, err)
			})
			it("should return an error when `name` is empty", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvEmptyName}, "")
				h.AssertEq(t, envMap, map[string]string(nil))
				h.AssertEq(t, len(warnings), 0)
				h.AssertNotNil(t, err)
			})
			it("should return warnings when `apprend` or `prepend` is used without `delim`", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvSuffixPrependWithoutDelim}, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvSuffixPrependWithoutDelim.Name + "." + string(BuildConfigEnvSuffixPrependWithoutDelim.Suffix): BuildConfigEnvSuffixPrependWithoutDelim.Value,
				})
				h.AssertNotEq(t, len(warnings), 0)
				h.AssertNotNil(t, err)
			})
			it("should return an error when unknown `suffix` is used", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{BuildConfigEnvSuffixUnknown}, "")
				h.AssertEq(t, envMap, map[string]string{})
				h.AssertEq(t, len(warnings), 0)
				h.AssertNotNil(t, err)
			})
			it("should override with the last specified delim when `[[build.env]]` has multiple delims with same `name` with a `append` or `prepend` suffix", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv(BuildConfigEnvSuffixMultiple, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvSuffixMultiple[0].Name + "." + string(BuildConfigEnvSuffixMultiple[0].Suffix): BuildConfigEnvSuffixMultiple[0].Value,
					BuildConfigEnvSuffixMultiple[1].Name + "." + string(BuildConfigEnvSuffixMultiple[1].Suffix): BuildConfigEnvSuffixMultiple[1].Value,
					BuildConfigEnvSuffixMultiple[2].Name + "." + string(BuildConfigEnvSuffixMultiple[2].Suffix): BuildConfigEnvSuffixMultiple[2].Value,
					BuildConfigEnvSuffixMultiple[2].Name + ".delim":                                             BuildConfigEnvSuffixMultiple[2].Delim,
				})
				h.AssertNotEq(t, len(warnings), 0)
				h.AssertNil(t, err)
			})
			it("should override `value` with the last read value when a `[[build.env]]` has same `name` with same `suffix`", func() {
				envMap, warnings, err := builder.ParseBuildConfigEnv(BuildConfigEnvDelimWithSameSuffixAndName, "")
				h.AssertEq(t, envMap, map[string]string{
					BuildConfigEnvDelimWithSameSuffixAndName[1].Name: BuildConfigEnvDelimWithSameSuffixAndName[1].Value,
				})
				h.AssertNotEq(t, len(warnings), 0)
				h.AssertNil(t, err)
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
