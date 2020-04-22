package commands_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestReportCommand(t *testing.T) {
	spec.Run(t, "Commands", testReportCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testReportCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command         *cobra.Command
		logger          logging.Logger
		outBuf          bytes.Buffer
		packHome        string
		packConfigPath  string
		packMissingHome string
	)

	it.Before(func() {
		var err error
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		command = commands.Report(logger)

		packHome, err = ioutil.TempDir("", "")
		h.AssertNil(t, err)
		packConfigPath = filepath.Join(packHome, "config.toml")
		h.AssertNil(t, ioutil.WriteFile(packConfigPath, []byte(`
default-builder-image = "some/image"
experimental = true
`), 0666))
		packMissingHome, err = ioutil.TempDir("", "")
		h.AssertNil(t, err)
	})

	it.After(func() {
		os.RemoveAll(packHome)
		os.RemoveAll(packMissingHome)
	})

	when("#ReportCommand", func() {
		when("config.toml is present", func() {
			it.Before(func() {
				os.Setenv("PACK_HOME", packHome)
			})

			it.After(func() {
				os.Unsetenv("PACK_HOME")
			})

			it("presents output", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), `default-builder-image = "some/image"`)
				h.AssertContains(t, outBuf.String(), `experimental = true`)
			})
		})
		when("config.toml is not present", func() {
			it.Before(func() {
				os.Setenv("PACK_HOME", packMissingHome)
			})
			it.After(func() {
				os.Unsetenv("PACK_HOME")
			})
			it("logs a message", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), fmt.Sprintf("(no config file found at %s)", filepath.Join(packMissingHome, "config.toml")))
			})
		})
	})
}
