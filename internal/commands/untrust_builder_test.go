package commands_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/internal/style"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestUntrustBuilder(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Commands", testUntrustBuilderCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testUntrustBuilderCommand(t *testing.T, when spec.G, it spec.S) {
	var (
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
		h.AssertNil(t, os.Setenv("PACK_HOME", tempPackHome))

		configPath = filepath.Join(tempPackHome, "config.toml")
	})

	it.After(func() {
		h.AssertNil(t, os.Unsetenv("PACK_HOME"))
		h.AssertNil(t, os.RemoveAll(tempPackHome))
	})

	when("#UntrustBuilder", func() {
		when("no builder is provided", func() {
			it("prints usage", func() {
				command := untrustBuilderCommandWithTrustedBuilders(t, logger, configPath)
				command.SetArgs([]string{})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Usage:")
			})
		})

		when("builder is already trusted", func() {
			it("removes builder from the config", func() {
				builderName := "some-builder"

				command := untrustBuilderCommandWithTrustedBuilders(t, logger, configPath, builderName)
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

				command := untrustBuilderCommandWithTrustedBuilders(t,
					logger,
					configPath,
					untrustBuilder,
					stillTrustedBuilder,
				)
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

				command := untrustBuilderCommandWithTrustedBuilders(t, logger, configPath, stillTrustedBuilder)
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
	})
}

//nolint:whitespace // A leading line of whitespace is left after a method declaration with multi-line arguments
func untrustBuilderCommandWithTrustedBuilders(
	t *testing.T,
	logger logging.Logger,
	configPath string,
	trustedBuilders ...string,
) *cobra.Command {

	t.Helper()

	cfg := config.Config{}
	for _, builderName := range trustedBuilders {
		cfg.TrustedBuilders = append(cfg.TrustedBuilders, config.TrustedBuilder{Name: builderName})
	}
	err := config.Write(cfg, configPath)
	h.AssertNil(t, err)

	return commands.UntrustBuilder(logger, cfg)
}
