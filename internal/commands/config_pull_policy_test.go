package commands_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/logging"
	fetcher_mock "github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestConfigPullPolicy(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "ConfigPullPolicyCommand", testConfigPullPolicyCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testConfigPullPolicyCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command                *cobra.Command
		logger                 logging.Logger
		outBuf                 bytes.Buffer
		tempPackHome           string
		configFile             string
		imageJSONData          string
		assert                 = h.NewAssertionManager(t)
		cfg                    = config.Config{}
		imagePullPolicyHandler *fetcher_mock.MockImagePullPolicyHandler
	)

	it.Before(func() {
		var err error
		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
		imagePullPolicyHandler = fetcher_mock.NewMockPullPolicyManager(logger)
		tempPackHome, err = os.MkdirTemp("", "pack-home")
		h.AssertNil(t, err)
		h.AssertNil(t, os.Setenv("PACK_HOME", tempPackHome))
		configFile = filepath.Join(tempPackHome, "config.toml")
		jsonFilePath := filepath.Join("testdata", "example_image.json")
		data, err := os.ReadFile(jsonFilePath)
		h.AssertNil(t, err)
		imageJSONData = string(data)

		// Create the .pack directory and image.json file
		packDir := filepath.Join(tempPackHome, ".pack")
		h.AssertNil(t, os.Mkdir(packDir, 0755))

		imageJSONFile := filepath.Join(packDir, "image.json")
		h.AssertNil(t, os.WriteFile(imageJSONFile, []byte(imageJSONData), 0644))

		command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
		command.SetOut(logging.GetWriterForLevel(logger, logging.InfoLevel))
	})

	it.After(func() {
		h.AssertNil(t, os.Unsetenv("PACK_HOME"))
		h.AssertNil(t, os.RemoveAll(tempPackHome))
	})

	when("#ConfigPullPolicy", func() {
		when("list", func() {
			when("no policy is specified", func() {
				it("lists default pull policy", func() {
					command.SetArgs([]string{})

					h.AssertNil(t, command.Execute())

					assert.Contains(outBuf.String(), "always")
				})
			})
			when("policy set to always in config", func() {
				it("lists always as pull policy", func() {
					cfg.PullPolicy = "always"
					command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
					command.SetArgs([]string{})

					h.AssertNil(t, command.Execute())

					assert.Contains(outBuf.String(), "always")
				})
			})

			when("policy set to never in config", func() {
				it("lists never as pull policy", func() {
					cfg.PullPolicy = "never"
					command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
					command.SetArgs([]string{})

					h.AssertNil(t, command.Execute())

					assert.Contains(outBuf.String(), "never")
				})
			})

			when("policy set to if-not-present in config", func() {
				it("lists if-not-present as pull policy", func() {
					cfg.PullPolicy = "if-not-present"
					command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
					command.SetArgs([]string{})

					h.AssertNil(t, command.Execute())

					assert.Contains(outBuf.String(), "if-not-present")
				})
			})

			when("policy set to hourly in config", func() {
				it("lists hourly as pull policy", func() {
					cfg.PullPolicy = "hourly"
					command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
					command.SetArgs([]string{})

					h.AssertNil(t, command.Execute())

					assert.Contains(outBuf.String(), "hourly")
				})
			})

			when("policy set to daily in config", func() {
				it("lists daily as pull policy", func() {
					cfg.PullPolicy = "daily"
					command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
					command.SetArgs([]string{})

					h.AssertNil(t, command.Execute())

					assert.Contains(outBuf.String(), "daily")
				})
			})

			when("policy set to weekly in config", func() {
				it("lists weekly as pull policy", func() {
					cfg.PullPolicy = "weekly"
					command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
					command.SetArgs([]string{})

					h.AssertNil(t, command.Execute())

					assert.Contains(outBuf.String(), "weekly")
				})
			})

			when("policy set to interval=1d2h30m in config", func() {
				it("lists interval=1d2h30m as pull policy", func() {
					cfg.PullPolicy = "interval=1d2h30m"
					command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
					command.SetArgs([]string{})

					h.AssertNil(t, command.Execute())

					assert.Contains(outBuf.String(), "interval=1d2h30m")
				})
			})
		})
		when("set", func() {
			when("policy provided is the same as configured pull policy", func() {
				it("provides a helpful message", func() {
					cfg.PullPolicy = "if-not-present"
					command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
					command.SetArgs([]string{"if-not-present"})

					h.AssertNil(t, command.Execute())

					output := outBuf.String()
					h.AssertEq(t, strings.TrimSpace(output), `Pull policy is already set to 'if-not-present'`)
				})
				it("it does not change the configured policy", func() {
					command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
					command.SetArgs([]string{"never"})
					assert.Succeeds(command.Execute())

					readCfg, err := config.Read(configFile)
					assert.Nil(err)
					assert.Equal(readCfg.PullPolicy, "never")

					command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)
					command.SetArgs([]string{"never"})
					assert.Succeeds(command.Execute())

					readCfg, err = config.Read(configFile)
					assert.Nil(err)
					assert.Equal(readCfg.PullPolicy, "never")
				})
			})

			when("invalid policy is specified", func() {
				it("does not write invalid policy to config", func() {
					command.SetArgs([]string{"never"})
					assert.Succeeds(command.Execute())

					command.SetArgs([]string{"invalid-policy"})
					err := command.Execute()
					h.AssertError(t, err, `invalid pull policy invalid-policy`)

					readCfg, err := config.Read(configFile)
					assert.Nil(err)
					assert.Equal(readCfg.PullPolicy, "never")
				})
			})

			when("valid policy is specified", func() {
				it("sets the policy in config", func() {
					command.SetArgs([]string{"never"})
					assert.Succeeds(command.Execute())

					readCfg, err := config.Read(configFile)
					assert.Nil(err)
					assert.Equal(readCfg.PullPolicy, "never")
				})
			})
		})
		when("unset", func() {
			it("removes set policy and resets to default pull policy", func() {
				command.SetArgs([]string{"never"})
				command = commands.ConfigPullPolicy(logger, cfg, configFile, imagePullPolicyHandler)

				command.SetArgs([]string{"--unset"})
				assert.Succeeds(command.Execute())

				cfg, err := config.Read(configFile)
				assert.Nil(err)
				assert.Equal(cfg.PullPolicy, "")
			})
		})
		when("--unset and policy to set is provided", func() {
			it("errors", func() {
				command.SetArgs([]string{
					"never",
					"--unset",
				})
				err := command.Execute()
				h.AssertError(t, err, `pull policy and --unset cannot be specified simultaneously`)
			})
		})
	})
}
