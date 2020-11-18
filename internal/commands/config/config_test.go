package config_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	cmdConfig "github.com/buildpacks/pack/internal/commands/config"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestConfigCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "ConfigCommands", testConfigCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testConfigCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command      *cobra.Command
		logger       logging.Logger
		outBuf       bytes.Buffer
		tempPackHome string
		configPath   string
	)

	it.Before(func() {
		var err error

		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		tempPackHome, err = ioutil.TempDir("", "pack-home")
		h.AssertNil(t, err)
		configPath = filepath.Join(tempPackHome, "config.toml")

		command = cmdConfig.NewConfigCommand(logger, config.Config{}, configPath)
		command.SetOut(logging.GetWriterForLevel(logger, logging.InfoLevel))
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tempPackHome))
	})

	when("config", func() {
		it("prints help text", func() {
			command.SetArgs([]string{})
			h.AssertNil(t, command.Execute())
			output := outBuf.String()
			h.AssertContains(t, output, "Interact with pack config")
			h.AssertContains(t, output, "Usage:")
		})
	})

	when("trusted-builders", func() {
		it("prints list of trusted builders", func() {
			command.SetArgs([]string{"trusted-builders"})
			h.AssertNil(t, command.Execute())
			h.AssertContainsAllInOrder(t,
				outBuf,
				"gcr.io/buildpacks/builder:v1",
				"heroku/buildpacks:18",
				"paketobuildpacks/builder:base",
				"paketobuildpacks/builder:full",
				"paketobuildpacks/builder:tiny",
			)
		})

		it("works with alias of trusted-builders", func() {
			command.SetArgs([]string{"trusted-builders"})
			h.AssertNil(t, command.Execute())
			h.AssertContainsAllInOrder(t,
				outBuf,
				"gcr.io/buildpacks/builder:v1",
				"heroku/buildpacks:18",
				"paketobuildpacks/builder:base",
				"paketobuildpacks/builder:full",
				"paketobuildpacks/builder:tiny",
			)
		})
	})
}

type configManager struct {
	testObject *testing.T
	configPath string
}

func newConfigManager(t *testing.T, configPath string) configManager {
	return configManager{
		testObject: t,
		configPath: configPath,
	}
}

func (c configManager) configWithTrustedBuilders(trustedBuilders ...string) config.Config {
	c.testObject.Helper()

	cfg := config.Config{}
	for _, builderName := range trustedBuilders {
		cfg.TrustedBuilders = append(cfg.TrustedBuilders, config.TrustedBuilder{Name: builderName})
	}
	err := config.Write(cfg, c.configPath)
	h.AssertNil(c.testObject, err)

	return cfg
}
