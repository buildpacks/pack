package commands_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
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

func TestTrustBuilder(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Commands", testTrustBuilderCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testTrustBuilderCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         logging.Logger
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		tempPackHome   string
		configPath     string
	)

	it.Before(func() {
		var err error

		mockController = gomock.NewController(t)
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		command = commands.TrustBuilder(logger, config.Config{})

		tempPackHome, err = ioutil.TempDir("", "pack-home")
		h.AssertNil(t, err)
		h.AssertNil(t, os.Setenv("PACK_HOME", tempPackHome))

		configPath = filepath.Join(tempPackHome, "config.toml")
	})

	it.After(func() {
		mockController.Finish()
		h.AssertNil(t, os.Unsetenv("PACK_HOME"))
		h.AssertNil(t, os.RemoveAll(tempPackHome))
	})

	when("#TrustBuilder", func() {
		when("no builder is provided", func() {
			it("prints usage", func() {
				command.SetArgs([]string{})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Usage:")
			})
		})

		when("builder is provided", func() {
			when("builder is not already trusted", func() {
				it("updates the config", func() {
					command.SetArgs([]string{"some-builder"})
					h.AssertNil(t, command.Execute())

					b, err := ioutil.ReadFile(configPath)
					h.AssertNil(t, err)
					h.AssertContains(t, string(b), `[[trusted-builders]]
  name = "some-builder"`)
				})
			})

			when("builder is already trusted", func() {
				it("does nothing", func() {
					command.SetArgs([]string{"some-already-trusted-builder"})
					h.AssertNil(t, command.Execute())
					oldContents, err := ioutil.ReadFile(configPath)
					h.AssertNil(t, err)

					command.SetArgs([]string{"some-already-trusted-builder"})
					h.AssertNil(t, command.Execute())

					newContents, err := ioutil.ReadFile(configPath)
					h.AssertNil(t, err)
					h.AssertEq(t, newContents, oldContents)
				})
			})
		})
	})
}
