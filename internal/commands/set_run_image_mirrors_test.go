package commands_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"
)

func TestSetRunImageMirrors(t *testing.T) {
	spec.Run(t, "Commands", testSetRunImageMirrorsCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testSetRunImageMirrorsCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command      *cobra.Command
		logger       logging.Logger
		outBuf       bytes.Buffer
		cfg          config.Config
		tempPackHome string
	)

	it.Before(func() {
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		cfg = config.Config{}
		command = commands.SetRunImagesMirrors(logger, cfg)

		var err error
		tempPackHome, err = ioutil.TempDir("", "pack-home")
		h.AssertNil(t, err)
		h.AssertNil(t, os.Setenv("PACK_HOME", tempPackHome))
	})

	it.After(func() {
		h.AssertNil(t, os.Unsetenv("PACK_HOME"))
		h.AssertNil(t, os.RemoveAll(tempPackHome))
	})

	when("#SetRunImageMirrors", func() {
		var (
			runImage        string
			testMirror1     string
			testMirror2     string
			testRunImageCfg []config.RunImage
		)
		it.Before(func() {
			runImage = "test/image"
			testMirror1 = "example.com/some/run1"
			testMirror2 = "example.com/some/run2"
			testRunImageCfg = []config.RunImage{{
				Image:   runImage,
				Mirrors: []string{testMirror1, testMirror2},
			}}
		})

		when("mirrors are provided", func() {
			it("adds them as mirrors to the config", func() {
				command.SetArgs([]string{runImage, "-m", testMirror1, "-m", testMirror2})
				h.AssertNil(t, command.Execute())
				cfg := readConfig(t)
				h.AssertEq(t, cfg.RunImages, testRunImageCfg)
			})
		})

		when("no mirrors are provided", func() {
			it.Before(func() {
				cfg.RunImages = testRunImageCfg
				command = commands.SetRunImagesMirrors(logger, cfg)
			})

			it("removes all mirrors for the run image", func() {
				command.SetArgs([]string{runImage})
				h.AssertNil(t, command.Execute())

				cfg := readConfig(t)
				h.AssertEq(t, cfg.RunImages, []config.RunImage{})
			})
		})
	})
}

func readConfig(t *testing.T) config.Config {
	path, err := config.DefaultConfigPath()
	h.AssertNil(t, err)

	cfg, err := config.Read(path)
	h.AssertNil(t, err)
	return cfg
}
