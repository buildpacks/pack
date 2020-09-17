package commands_test

import (
	"bytes"
	"testing"

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

func TestAddBuildpackRegistry(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testAddBuildpackRegistryCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAddBuildpackRegistryCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command *cobra.Command
		logger  logging.Logger
		outBuf  bytes.Buffer
		cfg     config.Config
	)

	when("AddBuildpackRegistry", func() {
		it.Before(func() {
			logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
			cfg = config.Config{}

			command = commands.AddBuildpackRegistry(logger, cfg)
		})

		it("fails with missing args", func() {
			err := command.Execute()
			h.AssertError(t, err, "accepts 3 arg")
		})

		it("should validate type", func() {
			command = commands.AddBuildpackRegistry(logger, cfg)
			command.SetArgs([]string{"bp", "https://github.com/buildpacks/registry-index/", "bogus"})
			command.Execute()

			output := outBuf.String()
			h.AssertContains(t, output, "'bogus' is not a valid type.  Supported types are 'github' or 'git'.")
		})

		it("should throw error when registry already exists", func() {
			cfg = config.Config{
				Registries: []config.Registry{
					{
						Name: "bp",
						Type: "github",
						URL:  "https://github.com/buildpacks/registry-index/",
					},
				},
			}
			command = commands.AddBuildpackRegistry(logger, cfg)
			command.SetArgs([]string{"bp", "https://github.com/buildpacks/registry-index/", "github"})
			command.Execute()

			output := outBuf.String()
			h.AssertContains(t, output, "Buildpack registry 'bp' already exists.  First run 'remove-buildpack-registry bp' and try again.")
		})
	})
}
