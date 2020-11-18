package config_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	cmdConfig "github.com/buildpacks/pack/internal/commands/config"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestUntrustBuilderCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Commands", testUntrustBuilderCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testUntrustBuilderCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		logger        logging.Logger
		outBuf        bytes.Buffer
		tempPackHome  string
		configPath    string
		configManager configManager
	)

	it.Before(func() {
		var err error

		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

		tempPackHome, err = ioutil.TempDir("", "pack-home")
		h.AssertNil(t, err)
		h.AssertNil(t, os.Setenv("PACK_HOME", tempPackHome))

		configPath = filepath.Join(tempPackHome, "config.toml")
		configManager = newConfigManager(t, configPath)
	})

	it.After(func() {
		h.AssertNil(t, os.Unsetenv("PACK_HOME"))
		h.AssertNil(t, os.RemoveAll(tempPackHome))
	})

	when("#UntrustBuilder", func() {
		when("no builder is provided", func() {
			it("prints usage", func() {
				cfg := configManager.configWithTrustedBuilders()
				command := cmdConfig.UntrustBuilder(logger, cfg)
				command.SetArgs([]string{})
				command.SetOut(&outBuf)

				err := command.Execute()
				h.AssertError(t, err, "accepts 1 arg(s), received 0")
				h.AssertContains(t, outBuf.String(), "Usage:")
			})
		})

		when("builder is already trusted", func() {
			it("removes builder from the config", func() {
				builderName := "some-builder"

				cfg := configManager.configWithTrustedBuilders(builderName)
				command := cmdConfig.UntrustBuilder(logger, cfg)
				command.SetArgs([]string{builderName})

				h.AssertNil(t, command.Execute())

				b, err := ioutil.ReadFile(configPath)
				h.AssertNil(t, err)
				h.AssertNotContains(t, string(b), builderName)

				h.AssertContains(t,
					outBuf.String(),
					fmt.Sprintf("Builder %s is no longer trusted", style.Symbol(builderName)),
				)
			})

			it("removes only the named builder when multiple builders are trusted", func() {
				untrustBuilder := "stop/trusting:me"
				stillTrustedBuilder := "very/safe/builder"

				cfg := configManager.configWithTrustedBuilders(untrustBuilder, stillTrustedBuilder)
				command := cmdConfig.UntrustBuilder(logger, cfg)
				command.SetArgs([]string{untrustBuilder})

				h.AssertNil(t, command.Execute())

				b, err := ioutil.ReadFile(configPath)
				h.AssertNil(t, err)
				h.AssertContains(t, string(b), stillTrustedBuilder)
				h.AssertNotContains(t, string(b), untrustBuilder)
			})
		})

		when("builder wasn't already trusted", func() {
			it("does nothing and reports builder wasn't trusted", func() {
				neverTrustedBuilder := "never/trusted-builder"
				stillTrustedBuilder := "very/safe/builder"

				cfg := configManager.configWithTrustedBuilders(stillTrustedBuilder)
				command := cmdConfig.UntrustBuilder(logger, cfg)
				command.SetArgs([]string{neverTrustedBuilder})

				h.AssertNil(t, command.Execute())

				b, err := ioutil.ReadFile(configPath)
				h.AssertNil(t, err)
				h.AssertContains(t, string(b), stillTrustedBuilder)
				h.AssertNotContains(t, string(b), neverTrustedBuilder)

				h.AssertContains(t,
					outBuf.String(),
					fmt.Sprintf("Builder %s wasn't trusted", style.Symbol(neverTrustedBuilder)),
				)
			})
		})

		when("builder is a suggested builder", func() {
			it("does nothing and reports that ", func() {
				builder := "paketobuildpacks/builder:base"
				command := cmdConfig.UntrustBuilder(logger, config.Config{})
				command.SetArgs([]string{builder})

				err := command.Execute()
				h.AssertError(t, err, fmt.Sprintf("Builder %s is a suggested builder, and is trusted by default", style.Symbol(builder)))
			})
		})
	})
}
