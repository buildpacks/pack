package commands_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/internal/commands"
	ifakes "github.com/buildpack/pack/internal/fakes"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCompletionCommand(t *testing.T) {
	spec.Run(t, "Commands", testCompletionCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testCompletionCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command    *cobra.Command
		logger     logging.Logger
		outBuf     bytes.Buffer
		packHome   string
		missingDir string
	)

	it.Before(func() {
		logger = ifakes.NewFakeLogger(&outBuf)
		// the CompletionCommand calls a method on its Parent(), so it needs to have
		// one.
		command = &cobra.Command{}
		command.AddCommand(commands.CompletionCommand(logger))
		command.SetArgs([]string{"completion"})
		var err error
		packHome, err = ioutil.TempDir("", "")
		h.AssertNil(t, err)
		missingDir = filepath.Join(packHome, "not-found")
	})

	it.After(func() {
		os.RemoveAll(packHome)
	})

	when("#CompletionCommand", func() {
		when("PackHome directory does exist", func() {
			it.Before(func() {
				os.Setenv("PACK_HOME", packHome)
			})

			it.After(func() {
				os.Unsetenv("PACK_HOME")
			})

			it("creates the completion file", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), filepath.Join(packHome, "completion"))
			})
		})

		when("PackHome directory does not exist", func() {
			it.Before(func() {
				os.Setenv("PACK_HOME", missingDir)
			})

			it.After(func() {
				os.Unsetenv("PACK_HOME")
			})

			it("creates the completion file", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), filepath.Join(missingDir, "completion"))
			})
		})
	})
}
