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

	"github.com/buildpacks/pack/internal/commands"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
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
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
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

		when("Shell flag is empty(default value)", func() {
			it("errors should not be occurred", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Completion File for bash is created")
			})
		})

		when("Shell flag is zsh", func() {
			it.Before(func() {
				command.SetArgs([]string{"completion", "--shell", "zsh"})
			})

			it.After(func() {
				command.SetArgs([]string{"completion"})
			})

			it("errors should not be occurred", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Completion File for zsh is created")
			})
		})

		when("Shell flag is not bash or zsh", func() {
			it.Before(func() {
				command.SetArgs([]string{"completion", "--shell", "fish"})
			})

			it.After(func() {
				command.SetArgs([]string{"completion"})
			})

			it("errors should be occurred", func() {
				h.AssertError(t, command.Execute(), "fish is unsupported shell")
				h.AssertNotContains(t, outBuf.String(), "Completion File for fish is created")
			})
		})
	})
}
