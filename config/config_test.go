package config_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/heroku/color"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/config"
	h "github.com/buildpack/pack/testhelpers"
)

func TestConfig(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "config", testConfig, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testConfig(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir     string
		configPath string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "pack.config.test.")
		h.AssertNil(t, err)
		configPath = filepath.Join(tmpDir, "config.toml")
	})

	it.After(func() {
		err := os.RemoveAll(tmpDir)
		h.AssertNil(t, err)
	})

	when("#Read", func() {
		when("no config on disk", func() {
			it("returns an empty config", func() {
				subject, err := config.Read(configPath)
				h.AssertNil(t, err)
				h.AssertEq(t, subject.DefaultBuilder, "")
				h.AssertEq(t, len(subject.RunImages), 0)
			})
		})
	})

	when("#Write", func() {
		when("no config on disk", func() {
			it("writes config to disk", func() {
				h.AssertNil(t, config.Write(config.Config{
					DefaultBuilder: "some/builder",
					RunImages: []config.RunImage{
						{
							Image:   "some/run",
							Mirrors: []string{"example.com/some/run", "example.com/some/mirror"},
						},
						{
							Image:   "other/run",
							Mirrors: []string{"example.com/other/run", "example.com/other/mirror"},
						},
					},
				}, configPath))
				b, err := ioutil.ReadFile(configPath)
				h.AssertNil(t, err)
				h.AssertContains(t, string(b), `default-builder-image = "some/builder"`)
				h.AssertContains(t, string(b), `[[run-images]]
  image = "some/run"
  mirrors = ["example.com/some/run", "example.com/some/mirror"]`)

				h.AssertContains(t, string(b), `[[run-images]]
  image = "other/run"
  mirrors = ["example.com/other/run", "example.com/other/mirror"]`)
			})
		})

		when("config on disk", func() {
			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(configPath, []byte("some-old-contents"), 0777))
			})

			it("replaces the file", func() {
				h.AssertNil(t, config.Write(config.Config{
					DefaultBuilder: "some/builder",
				}, configPath))
				b, err := ioutil.ReadFile(configPath)
				h.AssertNil(t, err)
				h.AssertContains(t, string(b), `default-builder-image = "some/builder"`)
				h.AssertNotContains(t, string(b), "some-old-contents")
			})
		})

		when("directories are missing", func() {
			it("creates the directories", func() {
				missingDirConfigPath := filepath.Join(tmpDir, "not", "yet", "created", "config.toml")
				h.AssertNil(t, config.Write(config.Config{
					DefaultBuilder: "some/builder",
				}, missingDirConfigPath))

				b, err := ioutil.ReadFile(missingDirConfigPath)
				h.AssertNil(t, err)
				h.AssertContains(t, string(b), `default-builder-image = "some/builder"`)
			})
		})
	})

	when("#MkdirAll", func() {
		when("the directory doesn't exist yet", func() {
			it("creates the directory", func() {
				path := filepath.Join(tmpDir, "a-new-dir")
				err := config.MkdirAll(path)
				h.AssertNil(t, err)
				fi, err := os.Stat(path)
				h.AssertNil(t, err)
				h.AssertEq(t, fi.Mode().IsDir(), true)
			})
		})

		when("the directory already exists", func() {
			it("doesn't error", func() {
				err := config.MkdirAll(tmpDir)
				h.AssertNil(t, err)
				fi, err := os.Stat(tmpDir)
				h.AssertNil(t, err)
				h.AssertEq(t, fi.Mode().IsDir(), true)
			})
		})
	})

	when("#SetRunImageMirrors", func() {
		when("run image exists in config", func() {
			it("replaces the mirrors", func() {
				cfg := config.SetRunImageMirrors(
					config.Config{
						RunImages: []config.RunImage{
							{
								Image:   "some/run-image",
								Mirrors: []string{"old/mirror", "other/mirror"},
							},
						},
					},
					"some/run-image",
					[]string{"some-other/run"},
				)

				h.AssertEq(t, len(cfg.RunImages), 1)
				h.AssertEq(t, cfg.RunImages[0].Image, "some/run-image")
				h.AssertEq(t, len(cfg.RunImages[0].Mirrors), 1)
				h.AssertSliceContains(t, cfg.RunImages[0].Mirrors, "some-other/run")
			})
		})

		when("run image does not exist in config", func() {
			it("adds the run image", func() {
				cfg := config.SetRunImageMirrors(
					config.Config{},
					"some/run-image",
					[]string{"some-other/run"},
				)

				h.AssertEq(t, len(cfg.RunImages), 1)
				h.AssertEq(t, cfg.RunImages[0].Image, "some/run-image")
				h.AssertEq(t, len(cfg.RunImages[0].Mirrors), 1)
				h.AssertSliceContains(t, cfg.RunImages[0].Mirrors, "some-other/run")
			})
		})
	})
}
