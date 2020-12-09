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

func TestReport(t *testing.T) {
	spec.Run(t, "ReportCommand", testReportCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testReportCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command           *cobra.Command
		logger            logging.Logger
		outBuf            bytes.Buffer
		tempPackHome      string
		packConfigPath    string
		tempPackEmptyHome string
		testVersion       = "1.2.3"
	)

	it.Before(func() {
		var err error
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

		tempPackHome, err = ioutil.TempDir("", "pack-home")
		h.AssertNil(t, err)

		packConfigPath = filepath.Join(tempPackHome, "config.toml")
		command = commands.Report(logger, testVersion, packConfigPath)
		command.SetArgs([]string{})
		h.AssertNil(t, ioutil.WriteFile(packConfigPath, []byte(`
default-builder-image = "some/image"
experimental = true
`), 0666))

		tempPackEmptyHome, err = ioutil.TempDir("", "")
		h.AssertNil(t, err)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tempPackHome))
		h.AssertNil(t, os.RemoveAll(tempPackEmptyHome))
	})

	when("#ReportCommand", func() {
		when("config.toml is present", func() {
			it("presents output", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), `default-builder-image = "[REDACTED]"`)
				h.AssertContains(t, outBuf.String(), `experimental = true`)
				h.AssertContains(t, outBuf.String(), `Version:  `+testVersion)

				h.AssertNotContains(t, outBuf.String(), `default-builder-image = "some/image"`)
			})

			it("doesn't sanitize output if explicit", func() {
				command.SetArgs([]string{"-e"})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), `default-builder-image = "some/image"`)
				h.AssertContains(t, outBuf.String(), `experimental = true`)
				h.AssertContains(t, outBuf.String(), `Version:  `+testVersion)

				h.AssertNotContains(t, outBuf.String(), `default-builder-image = "[REDACTED]"`)
			})
		})

		when("config.toml is not present", func() {
			it("logs a message", func() {
				command = commands.Report(logger, testVersion, filepath.Join(tempPackEmptyHome, "/config.toml"))
				command.SetArgs([]string{})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), fmt.Sprintf("(no config file found at %s)", filepath.Join(tempPackEmptyHome, "config.toml")))
			})
		})
	})
}
