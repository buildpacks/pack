package commands_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/fakes"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
	ilogging "github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestPackageCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "PackageCommand", testPackageCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPackageCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		logger *ilogging.LogWithWriters
		outBuf bytes.Buffer
	)

	it.Before(func() {
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
	})

	when("Package#Execute", func() {
		var fakeBuildpackPackager *fakes.FakeBuildpackPackager

		it.Before(func() {
			fakeBuildpackPackager = &fakes.FakeBuildpackPackager{}
		})

		when("valid package config", func() {
			it("reads package config from the configured path", func() {
				fakePackageConfigReader := fakes.NewFakePackageConfigReader()
				expectedPackageConfigPath := "/path/to/some/file"

				cmd := packageCommand(
					withPackageConfigReader(fakePackageConfigReader),
					withPackageConfigPath(expectedPackageConfigPath),
				)
				err := cmd.Execute()
				h.AssertNil(t, err)

				h.AssertEq(t, fakePackageConfigReader.ReadCalledWithArg, expectedPackageConfigPath)
			})

			it("creates package with correct image name", func() {
				cmd := packageCommand(
					withImageName("my-specific-image"),
					withBuildpackPackager(fakeBuildpackPackager),
				)
				err := cmd.Execute()
				h.AssertNil(t, err)

				receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
				h.AssertEq(t, receivedOptions.Name, "my-specific-image")
			})

			it("creates package with config returned by the reader", func() {
				myConfig := pubbldpkg.Config{
					Buildpack: dist.BuildpackURI{URI: "test"},
				}

				cmd := packageCommand(
					withBuildpackPackager(fakeBuildpackPackager),
					withPackageConfigReader(fakes.NewFakePackageConfigReader(whereReadReturns(myConfig, nil))),
				)
				err := cmd.Execute()
				h.AssertNil(t, err)

				receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
				h.AssertEq(t, receivedOptions.Config, myConfig)
			})
			when("file format", func() {
				when("extension is .cnb", func() {
					it("does not modify the name", func() {
						cmd := packageCommand(withBuildpackPackager(fakeBuildpackPackager))
						cmd.SetArgs([]string{"test.cnb", "-f", "file"})
						h.AssertNil(t, cmd.Execute())

						receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
						h.AssertEq(t, receivedOptions.Name, "test.cnb")
					})
				})
				when("extension is empty", func() {
					it("appends .cnb to the name", func() {
						cmd := packageCommand(withBuildpackPackager(fakeBuildpackPackager))
						cmd.SetArgs([]string{"test", "-f", "file"})
						h.AssertNil(t, cmd.Execute())

						receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
						h.AssertEq(t, receivedOptions.Name, "test.cnb")
					})
				})
				when("extension is something other than .cnb", func() {
					it("does not modify the name but shows a warning", func() {
						cmd := packageCommand(withBuildpackPackager(fakeBuildpackPackager), withLogger(logger))
						cmd.SetArgs([]string{"test.tar.gz", "-f", "file"})
						h.AssertNil(t, cmd.Execute())

						receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
						h.AssertEq(t, receivedOptions.Name, "test.tar.gz")
						h.AssertContains(t, outBuf.String(), "'.gz' is not a valid extension for a packaged buildpack. Packaged buildpacks must have a '.cnb' extension")
					})
				})
			})
			when("pull-policy", func() {
				var pullPolicyArgs = []string{
					"some-image-name",
					"--config", "/path/to/some/file",
					"--pull-policy",
				}

				it("pull-policy=never sets policy", func() {
					cmd := packageCommand(withBuildpackPackager(fakeBuildpackPackager))
					cmd.SetArgs(append(pullPolicyArgs, "never"))
					h.AssertNil(t, cmd.Execute())

					receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
					h.AssertEq(t, receivedOptions.PullPolicy, pubcfg.PullNever)
				})

				it("pull-policy=always sets policy", func() {
					cmd := packageCommand(withBuildpackPackager(fakeBuildpackPackager))
					cmd.SetArgs(append(pullPolicyArgs, "always"))
					h.AssertNil(t, cmd.Execute())

					receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
					h.AssertEq(t, receivedOptions.PullPolicy, pubcfg.PullAlways)
				})
			})
			when("no --pull-policy", func() {
				var pullPolicyArgs = []string{
					"some-image-name",
					"--config", "/path/to/some/file",
				}

				it("uses the default policy when no policy configured", func() {
					cmd := packageCommand(withBuildpackPackager(fakeBuildpackPackager))
					cmd.SetArgs(pullPolicyArgs)
					h.AssertNil(t, cmd.Execute())

					receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
					h.AssertEq(t, receivedOptions.PullPolicy, pubcfg.PullAlways)
				})
				it("uses the configured pull policy when policy configured", func() {
					cmd := packageCommand(
						withBuildpackPackager(fakeBuildpackPackager),
						withClientConfig(config.Config{PullPolicy: "never"}),
					)

					cmd.SetArgs([]string{
						"some-image-name",
						"--config", "/path/to/some/file",
					})

					err := cmd.Execute()
					h.AssertNil(t, err)

					receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
					h.AssertEq(t, receivedOptions.PullPolicy, pubcfg.PullNever)
				})
			})
		})

		when("no config path is specified", func() {
			it("creates a default config", func() {
				cmd := packageCommand(withBuildpackPackager(fakeBuildpackPackager))
				cmd.SetArgs([]string{"some-name"})
				h.AssertNil(t, cmd.Execute())

				receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions
				h.AssertEq(t, receivedOptions.Config.Buildpack.URI, ".")
			})
		})
	})

	when("invalid flags", func() {
		when("both --publish and --pull-policy never flags are specified", func() {
			it("errors with a descriptive message", func() {
				cmd := packageCommand()
				cmd.SetArgs([]string{
					"some-image-name", "--config", "/path/to/some/file",
					"--publish",
					"--pull-policy", "never",
				})

				err := cmd.Execute()
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "--publish and --pull-policy never cannot be used together. The --publish flag requires the use of remote images.")
			})
		})

		it("logs an error and exits when package toml is invalid", func() {
			expectedErr := errors.New("it went wrong")

			cmd := packageCommand(
				withLogger(logger),
				withPackageConfigReader(
					fakes.NewFakePackageConfigReader(whereReadReturns(pubbldpkg.Config{}, expectedErr)),
				),
			)

			err := cmd.Execute()
			h.AssertNotNil(t, err)

			h.AssertContains(t, outBuf.String(), fmt.Sprintf("ERROR: reading config: %s", expectedErr))
		})

		when("package-config is specified", func() {
			it("errors with a descriptive message", func() {
				cmd := packageCommand()
				cmd.SetArgs([]string{"some-name", "--package-config", "some-path"})

				err := cmd.Execute()
				h.AssertError(t, err, "unknown flag: --package-config")
			})
		})

		when("--pull-policy unknown-policy", func() {
			it("fails to run", func() {
				cmd := packageCommand()
				cmd.SetArgs([]string{
					"some-image-name",
					"--config", "/path/to/some/file",
					"--pull-policy",
					"unknown-policy",
				})

				h.AssertError(t, cmd.Execute(), "parsing pull policy")
			})
		})
	})
}

type packageCommandConfig struct {
	logger              *ilogging.LogWithWriters
	packageConfigReader *fakes.FakePackageConfigReader
	buildpackPackager   *fakes.FakeBuildpackPackager
	clientConfig        config.Config
	imageName           string
	configPath          string
}

type packageCommandOption func(config *packageCommandConfig)

func packageCommand(ops ...packageCommandOption) *cobra.Command {
	config := &packageCommandConfig{
		logger:              ilogging.NewLogWithWriters(&bytes.Buffer{}, &bytes.Buffer{}),
		packageConfigReader: fakes.NewFakePackageConfigReader(),
		buildpackPackager:   &fakes.FakeBuildpackPackager{},
		clientConfig:        config.Config{},
		imageName:           "some-image-name",
		configPath:          "/path/to/some/file",
	}

	for _, op := range ops {
		op(config)
	}

	cmd := commands.BuildpackPackage(config.logger, config.clientConfig, config.buildpackPackager, config.packageConfigReader)
	cmd.SetArgs([]string{config.imageName, "--config", config.configPath})

	return cmd
}

func withLogger(logger *ilogging.LogWithWriters) packageCommandOption {
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

func withClientConfig(clientCfg config.Config) packageCommandOption {
	return func(config *packageCommandConfig) {
		config.clientConfig = clientCfg
	}
}

func whereReadReturns(config pubbldpkg.Config, err error) func(*fakes.FakePackageConfigReader) {
	return func(r *fakes.FakePackageConfigReader) {
		r.ReadReturnConfig = config
		r.ReadReturnError = err
	}
}
