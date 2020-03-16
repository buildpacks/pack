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
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/fakes"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestPackageBuildpackCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Commands", testPackageBuildpackCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPackageBuildpackCommand(t *testing.T, when spec.G, it spec.S) {
	when("PackageBuildpack#Execute", func() {
		it("reads package config from the configured path", func() {
			fakePackageConfigReader := fakes.NewFakePackageConfigReader()
			expectedConfigPath := "/path/to/some/file"

			packageBuildpackCommand := packageBuildpackCommand(
				withConfigReader(fakePackageConfigReader),
				withConfigPath(expectedConfigPath),
			)

			err := packageBuildpackCommand.Execute()
			h.AssertNil(t, err)

			h.AssertEq(t, fakePackageConfigReader.ReadCalledWithArg, expectedConfigPath)
		})

		it("logs an error and exits when package toml is invalid", func() {
			outBuf := &bytes.Buffer{}
			expectedErr := errors.New("it went wrong")

			packageBuildpackCommand := packageBuildpackCommand(
				withLogger(logging.NewLogWithWriters(outBuf, outBuf)),
				withConfigReader(
					fakes.NewFakePackageConfigReader(whereReadReturns(pubbldpkg.Config{}, expectedErr)),
				),
			)

			err := packageBuildpackCommand.Execute()
			h.AssertNotNil(t, err)

			h.AssertContains(t, outBuf.String(), fmt.Sprintf("ERROR: reading config: %s", expectedErr))
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

			h.AssertEq(t, receivedOptions.ImageName, "my-specific-image")
		})

		it("creates package with config returned by the reader", func() {
			fakeBuildpackPackager := &fakes.FakeBuildpackPackager{}

			myConfig := pubbldpkg.Config{
				Buildpack: dist.BuildpackURI{URI: "test"},
			}

			packageBuildpackCommand := packageBuildpackCommand(
				withBuildpackPackager(fakeBuildpackPackager),
				withConfigReader(fakes.NewFakePackageConfigReader(whereReadReturns(myConfig, nil))),
			)

			err := packageBuildpackCommand.Execute()
			h.AssertNil(t, err)

			receivedOptions := fakeBuildpackPackager.CreateCalledWithOptions

			h.AssertEq(t, receivedOptions.Config, myConfig)
		})
	})
}

type packageCommandConfig struct {
	logger            *logging.LogWithWriters
	configReader      *fakes.FakePackageConfigReader
	buildpackPackager *fakes.FakeBuildpackPackager

	imageName  string
	configPath string
}

type packageCommandOption func(config *packageCommandConfig)

func packageBuildpackCommand(ops ...packageCommandOption) *cobra.Command {
	config := &packageCommandConfig{
		logger:            logging.NewLogWithWriters(&bytes.Buffer{}, &bytes.Buffer{}),
		configReader:      fakes.NewFakePackageConfigReader(),
		buildpackPackager: &fakes.FakeBuildpackPackager{},

		imageName:  "some-image-name",
		configPath: "/path/to/some/file",
	}

	for _, op := range ops {
		op(config)
	}

	cmd := commands.PackageBuildpack(config.logger, config.buildpackPackager, config.configReader)
	cmd.SetArgs([]string{config.imageName, "--package-config", config.configPath})

	return cmd
}

func withLogger(logger *logging.LogWithWriters) packageCommandOption {
	return func(config *packageCommandConfig) {
		config.logger = logger
	}
}

func withConfigReader(reader *fakes.FakePackageConfigReader) packageCommandOption {
	return func(config *packageCommandConfig) {
		config.configReader = reader
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

func withConfigPath(path string) packageCommandOption {
	return func(config *packageCommandConfig) {
		config.configPath = path
	}
}

func whereReadReturns(config pubbldpkg.Config, err error) func(*fakes.FakePackageConfigReader) {
	return func(r *fakes.FakePackageConfigReader) {
		r.ReadReturnConfig = config
		r.ReadReturnError = err
	}
}
