package commands_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/buildpacks/pack/internal/config"

	pubcfg "github.com/buildpacks/pack/config"

	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/fakes"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestPackageBuildpackCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "PackageBuildpackCommand", testPackageBuildpackCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPackageBuildpackCommand(t *testing.T, when spec.G, it spec.S) {
	when("PackageBuildpack#Execute", func() {
		when("valid package config", func() {
			it("reads package config from the configured path", func() {
				fakePackageConfigReader := fakes.NewFakePackageConfigReader()
				expectedPackageConfigPath := "/path/to/some/file"

				packageBuildpackCommand := packageBuildpackCommand(
					withPackageConfigReader(fakePackageConfigReader),
					withPackageConfigPath(expectedPackageConfigPath),
				)

				err := packageBuildpackCommand.Execute()
				h.AssertNil(t, err)

				h.AssertEq(t, fakePackageConfigReader.ReadCalledWithArg, expectedPackageConfigPath)
			})

			it("creates package with correct image name", func() {
				fakeBuildpackPackager := &fakes.FakeBuildpackPackager{}

				packageBuildpackCommand := packageBuildpackCommand(
					withImageName("my-specific-image"),
					withBuildpackPackager(fakeBuildpackPackager),
				)

				err := packageBuildpackCommand.Execute()
				h.AssertNil(t, err)

				receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions

				h.AssertEq(t, receivedOptions.Name, "my-specific-image")
			})

			it("creates package with config returned by the reader", func() {
				fakeBuildpackPackager := &fakes.FakeBuildpackPackager{}

				myConfig := pubbldpkg.Config{
					Buildpack: dist.BuildpackURI{URI: "test"},
				}

				packageBuildpackCommand := packageBuildpackCommand(
					withBuildpackPackager(fakeBuildpackPackager),
					withPackageConfigReader(fakes.NewFakePackageConfigReader(whereReadReturns(myConfig, nil))),
				)

				err := packageBuildpackCommand.Execute()
				h.AssertNil(t, err)

				receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions

				h.AssertEq(t, receivedOptions.Config, myConfig)
			})

			when("--no-pull", func() {
				var (
					outBuf                bytes.Buffer
					cmd                   *cobra.Command
					args                  []string
					fakeBuildpackPackager *fakes.FakeBuildpackPackager
				)

				it.Before(func() {
					logger := logging.NewLogWithWriters(&outBuf, &outBuf)
					fakeBuildpackPackager = &fakes.FakeBuildpackPackager{}

					cmd = packageBuildpackCommand(withLogger(logger), withBuildpackPackager(fakeBuildpackPackager))
					args = []string{
						"some-image-name",
						"--config", "/path/to/some/file",
					}
				})

				it("logs warning and works", func() {
					args = append(args, "--no-pull")
					cmd.SetArgs(args)

					err := cmd.Execute()
					h.AssertContains(t, outBuf.String(), "Warning: Flag --no-pull has been deprecated")
					h.AssertNil(t, err)

					receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
					h.AssertEq(t, receivedOptions.PullPolicy, pubcfg.PullNever)
				})

				when("used together with --pull-policy always", func() {
					it("logs warning and disregards --no-pull", func() {
						args = append(args, "--no-pull", "--pull-policy", "always")
						cmd.SetArgs(args)

						err := cmd.Execute()
						h.AssertNil(t, err)

						output := outBuf.String()
						h.AssertContains(t, outBuf.String(), "Warning: Flag --no-pull has been deprecated")
						h.AssertContains(t, output, "Flag --no-pull ignored in favor of --pull-policy")

						receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
						h.AssertEq(t, receivedOptions.PullPolicy, pubcfg.PullAlways)
					})
				})

				when("used together with --pull-policy never", func() {
					it("logs warning and disregards --no-pull", func() {
						args = append(args, "--no-pull", "--pull-policy", "never")
						cmd.SetArgs(args)

						err := cmd.Execute()
						h.AssertNil(t, err)

						output := outBuf.String()
						h.AssertContains(t, outBuf.String(), "Warning: Flag --no-pull has been deprecated")
						h.AssertContains(t, output, "Flag --no-pull ignored in favor of --pull-policy")

						receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
						h.AssertEq(t, receivedOptions.PullPolicy, pubcfg.PullNever)
					})
				})
			})

			when("pull-policy", func() {
				var (
					outBuf                bytes.Buffer
					cmd                   *cobra.Command
					args                  []string
					fakeBuildpackPackager *fakes.FakeBuildpackPackager
				)

				it.Before(func() {
					logger := logging.NewLogWithWriters(&outBuf, &outBuf)
					fakeBuildpackPackager = &fakes.FakeBuildpackPackager{}

					cmd = packageBuildpackCommand(withLogger(logger), withBuildpackPackager(fakeBuildpackPackager))
					args = []string{
						"some-image-name",
						"--config", "/path/to/some/file",
					}
				})

				it("pull-policy=never sets policy", func() {
					args = append(args, "--pull-policy", "never")
					cmd.SetArgs(args)

					err := cmd.Execute()
					h.AssertNil(t, err)

					receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
					h.AssertEq(t, receivedOptions.PullPolicy, pubcfg.PullNever)
				})

				it("pull-policy=always sets policy", func() {
					args = append(args, "--pull-policy", "always")
					cmd.SetArgs(args)

					err := cmd.Execute()
					h.AssertNil(t, err)

					receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
					h.AssertEq(t, receivedOptions.PullPolicy, pubcfg.PullAlways)
				})
			})

			when("--os", func() {
				when("experimental enabled", func() {
					it("creates package with correct image name and os", func() {
						fakeBuildpackPackager := &fakes.FakeBuildpackPackager{}

						packageBuildpackCommand := packageBuildpackCommand(
							withBuildpackPackager(fakeBuildpackPackager),
							withExperimental(),
						)

						packageBuildpackCommand.SetArgs(
							[]string{
								"some-image-name",
								"--config", "/path/to/some/file",
								"--os", "windows",
							},
						)

						err := packageBuildpackCommand.Execute()
						h.AssertNil(t, err)

						receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions

						h.AssertEq(t, receivedOptions.OS, "windows")
					})
				})
			})
		})
	})

	when("invalid flags", func() {
		when("both --publish and --no-pull flags are specified", func() {
			it("errors with a descriptive message", func() {
				logger := logging.NewLogWithWriters(&bytes.Buffer{}, &bytes.Buffer{})
				configReader := fakes.NewFakePackageConfigReader()
				buildpackPackager := &fakes.FakeBuildpackPackager{}
				clientConfig := config.Config{}

				command := commands.PackageBuildpack(logger, clientConfig, buildpackPackager, configReader)
				command.SetArgs([]string{
					"some-image-name",
					"--config", "/path/to/some/file",
					"--publish",
					"--no-pull",
				})

				err := command.Execute()
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "The --publish and --no-pull flags cannot be used together. The --publish flag requires the use of remote images.")
			})
		})

		when("both --publish and --pull-policy never flags are specified", func() {
			it("errors with a descriptive message", func() {
				logger := logging.NewLogWithWriters(&bytes.Buffer{}, &bytes.Buffer{})
				configReader := fakes.NewFakePackageConfigReader()
				buildpackPackager := &fakes.FakeBuildpackPackager{}
				clientConfig := config.Config{}

				command := commands.PackageBuildpack(logger, clientConfig, buildpackPackager, configReader)
				command.SetArgs([]string{
					"some-image-name",
					"--config", "/path/to/some/file",
					"--publish",
					"--pull-policy",
					"never",
				})

				err := command.Execute()
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "--publish and --pull-policy never cannot be used together. The --publish flag requires the use of remote images.")
			})
		})

		it("logs an error and exits when package toml is invalid", func() {
			outBuf := &bytes.Buffer{}
			expectedErr := errors.New("it went wrong")

			packageBuildpackCommand := packageBuildpackCommand(
				withLogger(logging.NewLogWithWriters(outBuf, outBuf)),
				withPackageConfigReader(
					fakes.NewFakePackageConfigReader(whereReadReturns(pubbldpkg.Config{}, expectedErr)),
				),
			)

			err := packageBuildpackCommand.Execute()
			h.AssertNotNil(t, err)

			h.AssertContains(t, outBuf.String(), fmt.Sprintf("ERROR: reading config: %s", expectedErr))
		})

		when("package-config is specified", func() {
			it("errors with a descriptive message", func() {
				outBuf := &bytes.Buffer{}

				config := &packageCommandConfig{
					logger:              logging.NewLogWithWriters(outBuf, outBuf),
					packageConfigReader: fakes.NewFakePackageConfigReader(),
					buildpackPackager:   &fakes.FakeBuildpackPackager{},

					imageName:  "some-image-name",
					configPath: "/path/to/some/file",
				}

				cmd := commands.PackageBuildpack(config.logger, config.clientConfig, config.buildpackPackager, config.packageConfigReader)
				cmd.SetArgs([]string{config.imageName, "--package-config", config.configPath})

				err := cmd.Execute()
				h.AssertError(t, err, "unknown flag: --package-config")
			})
		})

		when("no config path is specified", func() {
			it("errors with a descriptive message", func() {
				config := &packageCommandConfig{
					logger:              logging.NewLogWithWriters(&bytes.Buffer{}, &bytes.Buffer{}),
					packageConfigReader: fakes.NewFakePackageConfigReader(),
					buildpackPackager:   &fakes.FakeBuildpackPackager{},

					imageName: "some-image-name",
				}

				cmd := commands.PackageBuildpack(config.logger, config.clientConfig, config.buildpackPackager, config.packageConfigReader)
				cmd.SetArgs([]string{config.imageName})

				err := cmd.Execute()
				h.AssertError(t, err, "Please provide a package config path")
			})
		})

		when("--pull-policy unknown-policy", func() {
			it("fails to run", func() {
				logger := logging.NewLogWithWriters(&bytes.Buffer{}, &bytes.Buffer{})
				configReader := fakes.NewFakePackageConfigReader()
				buildpackPackager := &fakes.FakeBuildpackPackager{}
				clientConfig := config.Config{}

				command := commands.PackageBuildpack(logger, clientConfig, buildpackPackager, configReader)
				command.SetArgs([]string{
					"some-image-name",
					"--config", "/path/to/some/file",
					"--pull-policy",
					"unknown-policy",
				})

				h.AssertError(t, command.Execute(), "parsing pull policy")
			})
		})

		when("--os flag is specified but experimental isn't set in the config", func() {
			it("errors with a descriptive message", func() {
				fakeBuildpackPackager := &fakes.FakeBuildpackPackager{}

				packageBuildpackCommand := packageBuildpackCommand(
					withBuildpackPackager(fakeBuildpackPackager),
				)

				packageBuildpackCommand.SetArgs(
					[]string{
						"some-image-name",
						"--config", "/path/to/some/file",
						"--os", "windows",
					},
				)

				err := packageBuildpackCommand.Execute()
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "Support for OS flag is currently experimental")
			})
		})
	})
}

type packageCommandConfig struct {
	logger              *logging.LogWithWriters
	packageConfigReader *fakes.FakePackageConfigReader
	buildpackPackager   *fakes.FakeBuildpackPackager
	clientConfig        config.Config

	imageName  string
	configPath string
}

type packageCommandOption func(config *packageCommandConfig)

func packageBuildpackCommand(ops ...packageCommandOption) *cobra.Command {
	config := &packageCommandConfig{
		logger:              logging.NewLogWithWriters(&bytes.Buffer{}, &bytes.Buffer{}),
		packageConfigReader: fakes.NewFakePackageConfigReader(),
		buildpackPackager:   &fakes.FakeBuildpackPackager{},
		clientConfig:        config.Config{},

		imageName:  "some-image-name",
		configPath: "/path/to/some/file",
	}

	for _, op := range ops {
		op(config)
	}

	cmd := commands.PackageBuildpack(config.logger, config.clientConfig, config.buildpackPackager, config.packageConfigReader)
	cmd.SetArgs([]string{config.imageName, "--config", config.configPath})

	return cmd
}

func withLogger(logger *logging.LogWithWriters) packageCommandOption {
	return func(config *packageCommandConfig) {
		config.logger = logger
	}
}

func withPackageConfigReader(reader *fakes.FakePackageConfigReader) packageCommandOption {
	return func(config *packageCommandConfig) {
		config.packageConfigReader = reader
	}
}

func withBuildpackPackager(creator *fakes.FakeBuildpackPackager) packageCommandOption {
	return func(config *packageCommandConfig) {
		config.buildpackPackager = creator
	}
}

func withImageName(name string) packageCommandOption {
	return func(config *packageCommandConfig) {
		config.imageName = name
	}
}

func withPackageConfigPath(path string) packageCommandOption {
	return func(config *packageCommandConfig) {
		config.configPath = path
	}
}

func withExperimental() packageCommandOption {
	return func(config *packageCommandConfig) {
		config.clientConfig.Experimental = true
	}
}

func whereReadReturns(config pubbldpkg.Config, err error) func(*fakes.FakePackageConfigReader) {
	return func(r *fakes.FakePackageConfigReader) {
		r.ReadReturnConfig = config
		r.ReadReturnError = err
	}
}
