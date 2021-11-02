package commands_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestListTrustedBuildersCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Commands", testListTrustedBuildersCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testListTrustedBuildersCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command      *cobra.Command
		logger       logging.Logger
		outBuf       bytes.Buffer
		tempPackHome string
	)

	it.Before(func() {
		var err error

		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
		command = commands.ListTrustedBuilders(logger, config.Config{})

		tempPackHome, err = ioutil.TempDir("", "pack-home")
		h.AssertNil(t, err)
		h.AssertNil(t, os.Setenv("PACK_HOME", tempPackHome))
	})

	it.After(func() {
		h.AssertNil(t, os.Unsetenv("PACK_HOME"))
		h.AssertNil(t, os.RemoveAll(tempPackHome))
	})

	when("#ListTrustedBuilders", func() {
		it("succeeds", func() {
			h.AssertNil(t, command.Execute())
		})

		it("shows header", func() {
			h.AssertNil(t, command.Execute())

			h.AssertContains(t, outBuf.String(), "Trusted Builders:")
		})

		it("shows suggested builders and locally trusted builder in alphabetical order", func() {
			builderName := "great-builder-" + h.RandString(8)

			h.AssertNil(t, command.Execute())
			h.AssertNotContains(t, outBuf.String(), builderName)
			h.AssertContainsAllInOrder(t,
				outBuf,
				"gcr.io/buildpacks/builder:v1",
				"heroku/buildpacks:18",
				"heroku/buildpacks:20",
				"paketobuildpacks/builder:base",
				"paketobuildpacks/builder:full",
				"paketobuildpacks/builder:tiny",
			)

			listTrustedBuildersCommand := commands.ListTrustedBuilders(
				logger,
				config.Config{
					TrustedBuilders: []config.TrustedBuilder{{Name: builderName}},
				},
			)

			outBuf.Reset()

			h.AssertNil(t, listTrustedBuildersCommand.Execute())

			h.AssertContainsAllInOrder(t,
				outBuf,
				"gcr.io/buildpacks/builder:v1",
				builderName,
				"heroku/buildpacks:18",
				"heroku/buildpacks:20",
				"paketobuildpacks/builder:base",
				"paketobuildpacks/builder:full",
				"paketobuildpacks/builder:tiny",
			)
		})
	})
}
