package commands_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestConfigPruneInterval(t *testing.T) {
	spec.Run(t, "ConfigPruneIntervalCommand", testConfigPruneIntervalCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testConfigPruneIntervalCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command      *cobra.Command
		logger       logging.Logger
		outBuf       bytes.Buffer
		tempPackHome string
		configFile   string
		assert       = h.NewAssertionManager(t)
		cfg          = config.Config{}
	)

	it.Before(func() {
		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
		tempPackHome, _ = os.MkdirTemp("", "pack-home")
		configFile = filepath.Join(tempPackHome, "config.toml")

		command = commands.ConfigPruneInterval(logger, cfg, configFile)
	})

	it.After(func() {
		_ = os.RemoveAll(tempPackHome)
	})

	when("#ConfigPruneInterval", func() {
		when("no arguments are provided", func() {
			it("lists the current pruning interval", func() {
				command.SetArgs([]string{})

				err := command.Execute()

				assert.Nil(err)
				assert.Contains(outBuf.String(), "The current prune interval is")
			})
		})

		when("an argument is provided", func() {
			when("argument is valid", func() {
				it("sets the provided interval as the pruning interval", func() {
					interval := "5d"
					command.SetArgs([]string{interval})

					err := command.Execute()

					assert.Nil(err)
					assert.Contains(outBuf.String(), "Successfully set")
				})
			})

			when("argument is invalid", func() {
				it("returns an error", func() {
					interval := "invalid"
					command.SetArgs([]string{interval})

					err := command.Execute()

					assert.Error(err)
					assert.Contains(err.Error(), "invalid interval format")
				})
			})
		})

		when("--unset flag is provided", func() {
			it("unsets the pruning interval", func() {
				command.SetArgs([]string{"--unset"})

				err := command.Execute()

				assert.Nil(err)
				assert.Contains(outBuf.String(), "Successfully unset pruning interval")
			})
		})

		when("both interval and --unset flag are provided", func() {
			it("returns an error", func() {
				command.SetArgs([]string{"5d", "--unset"})

				err := command.Execute()

				assert.Error(err)
				assert.Contains(err.Error(), "prune interval and --unset cannot be specified simultaneously")
			})
		})
	})
}
