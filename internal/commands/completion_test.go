package commands_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestCompletionCommand(t *testing.T) {
	spec.Run(t, "Commands", testCompletionCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testCompletionCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command  *cobra.Command
		logger   logging.Logger
		outBuf   bytes.Buffer
		dataDir  string
		assert   = h.NewAssertionManager(t)
	)

	it.Before(func() {
		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
		var err error
		dataDir, err = os.MkdirTemp("", "")
		assert.Nil(err)

		// the CompletionCommand calls a method on its Parent(), so it needs to have
		// one.
		command = &cobra.Command{}
		command.AddCommand(commands.CompletionCommand(logger, dataDir))
		command.SetArgs([]string{"completion"})
	})

	it.After(func() {
		os.RemoveAll(dataDir)
	})

	when("#CompletionCommand", func() {
		when("Shell flag is empty(default value)", func() {
			it("errors should not be occurred", func() {
				assert.Nil(command.Execute())
			})
		})

		when("data directory does not exist", func() {
			var missingDir string

			it.Before(func() {
				missingDir = filepath.Join(dataDir, "not-found")

				command = &cobra.Command{}
				command.AddCommand(commands.CompletionCommand(logger, missingDir))
				command.SetArgs([]string{"completion"})
			})

			it("should create completion file", func() {
				assert.Nil(command.Execute())
				assert.Contains(outBuf.String(), filepath.Join(missingDir, "completion.sh"))
			})

			it("should create the directory with 0700 permissions", func() {
				h.SkipIf(t, runtime.GOOS == "windows", "Windows doesn't support Unix permissions")

				assert.Nil(command.Execute())
				fi, err := os.Stat(missingDir)
				assert.Nil(err)
				assert.Equal(fi.Mode().Perm(), os.FileMode(0700))
			})
		})

		for _, test := range []struct {
			shell     string
			extension string
		}{
			{shell: "bash", extension: ".sh"},
			{shell: "fish", extension: ".fish"},
			{shell: "powershell", extension: ".ps1"},
			{shell: "zsh", extension: ".zsh"},
		} {
			shell := test.shell
			extension := test.extension

			when("shell is "+shell, func() {
				it("should create completion file ending in "+extension, func() {
					command.SetArgs([]string{"completion", "--shell", shell})
					assert.Nil(command.Execute())

					expectedFile := filepath.Join(dataDir, "completion"+extension)
					assert.Contains(outBuf.String(), expectedFile)
					assert.FileExists(expectedFile)
					assert.FileIsNotEmpty(expectedFile)
				})
			})
		}
	})
}
