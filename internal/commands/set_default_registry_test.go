package commands_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestSetDefaultRegistry(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "SetDefaultRegistryCommand", testSetDefaultRegistryCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testSetDefaultRegistryCommand(t *testing.T, when spec.G, it spec.S) {
	when("#SetDefaultRegistry", func() {
		var (
			outBuf     bytes.Buffer
			logger     = ilogging.NewLogWithWriters(&outBuf, &outBuf)
			tmpDir     string
			configFile string
			assert     = h.NewAssertionManager(t)
		)

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "pack-home-*")
			assert.Nil(err)

			configFile = filepath.Join(tmpDir, "config.toml")
		})

		it.After(func() {
			_ = os.RemoveAll(tmpDir)
		})

		it("should set the default registry", func() {
			cfg := config.Config{
				Registries: []config.Registry{
					{
						Name: "myregistry",
						URL:  "https://github.com/buildpacks/registry-index",
						Type: "github",
					},
				},
			}
			command := commands.SetDefaultRegistry(logger, cfg, configFile)
			command.SetArgs([]string{"myregistry"})
			assert.Succeeds(command.Execute())

			cfg, err := config.Read(configFile)
			assert.Nil(err)

			assert.Equal(cfg.DefaultRegistryName, "myregistry")
		})

		it("should fail if no corresponding registry exists", func() {
			command := commands.SetDefaultRegistry(logger, config.Config{}, configFile)
			command.SetArgs([]string{"myregistry"})
			assert.Error(command.Execute())

			output := outBuf.String()
			h.AssertContains(t, output, "no registry with the name 'myregistry' exists")
		})
	})
}
