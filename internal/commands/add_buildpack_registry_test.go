package commands_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestAddBuildpackRegistry(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testAddBuildpackRegistryCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAddBuildpackRegistryCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		outBuf   bytes.Buffer
		logger   = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		packHome string
		assert   = h.NewAssertionManager(t)
	)

	when("AddBuildpackRegistry", func() {
		it.Before(func() {
			tmpDir, err := ioutil.TempDir("", "pack-home-*")
			assert.Nil(err)

			packHome = tmpDir
			os.Setenv("PACK_HOME", packHome)
		})

		it.After(func() {
			_ = os.RemoveAll(packHome)
			os.Unsetenv("PACK_HOME")
		})

		when("default is true", func() {
			it("sets newly added registry as the default", func() {
				command := commands.AddBuildpackRegistry(logger, config.Config{})
				command.SetArgs([]string{"bp", "https://github.com/buildpacks/registry-index/", "--default"})
				assert.Succeeds(command.Execute())

				configPath, err := config.DefaultConfigPath()
				assert.Nil(err)

				cfg, err := config.Read(configPath)
				assert.Nil(err)

				assert.Equal(cfg.DefaultRegistryName, "bp")
			})
		})

		when("validation", func() {
			it("fails with missing args", func() {
				command := commands.AddBuildpackRegistry(logger, config.Config{})
				command.SetOut(ioutil.Discard)
				command.SetArgs([]string{})
				err := command.Execute()
				assert.ErrorContains(err, "accepts 2 arg")
			})

			it("should validate type", func() {
				command := commands.AddBuildpackRegistry(logger, config.Config{})
				command.SetArgs([]string{"bp", "https://github.com/buildpacks/registry-index/", "--type=bogus"})
				assert.Error(command.Execute())

				output := outBuf.String()
				h.AssertContains(t, output, "'bogus' is not a valid type.  Supported types are 'github' or 'git'.")
			})

			it("should throw error when registry already exists", func() {
				command := commands.AddBuildpackRegistry(logger, config.Config{
					Registries: []config.Registry{
						{
							Name: "bp",
							Type: "github",
							URL:  "https://github.com/buildpacks/registry-index/",
						},
					},
				})
				command.SetArgs([]string{"bp", "https://github.com/buildpacks/registry-index/"})
				assert.Error(command.Execute())

				output := outBuf.String()
				h.AssertContains(t, output, "Buildpack registry 'bp' already exists.  First run 'remove-buildpack-registry bp' and try again.")
			})
		})
	})
}
