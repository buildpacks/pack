package config_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatih/color"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/config"
	h "github.com/buildpack/pack/testhelpers"
)

func TestConfig(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "config", testConfig, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testConfig(t *testing.T, when spec.G, it spec.S) {
	var tmpDir string

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "pack.config.test.")
		h.AssertNil(t, err)
	})

	it.After(func() {
		err := os.RemoveAll(tmpDir)
		h.AssertNil(t, err)
	})

	when("#New", func() {
		when("no config on disk", func() {
			it("writes the defaults to disk", func() {
				subject, err := config.New(tmpDir)
				h.AssertNil(t, err)

				b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
				h.AssertNil(t, err)
				h.AssertNotContains(t, string(b), `default-builder-image`)
				h.AssertEq(t, subject.DefaultBuilder, "")
			})

			when("path is missing", func() {
				it("creates the directory", func() {
					_, err := config.New(filepath.Join(tmpDir, "a", "b"))
					h.AssertNil(t, err)

					b, err := ioutil.ReadFile(filepath.Join(tmpDir, "a", "b", "config.toml"))
					h.AssertNil(t, err)
					h.AssertNotContains(t, string(b), `default-builder-image`)
				})
			})
		})

		when("a previous builder is set as the default builder", func() {
			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
default-builder-image = "some/builder"
`), 0666))
			})

			it("loads the saved value", func() {
				subject, err := config.New(tmpDir)
				h.AssertNil(t, err)

				b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
				h.AssertNil(t, err)
				h.AssertContains(t, string(b), `default-builder-image = "some/builder"`)
				h.AssertEq(t, subject.DefaultBuilder, "some/builder")
			})
		})
	})

	when("Config#SetDefaultBuilder", func() {
		var subject *config.Config
		it.Before(func() {
			h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
default-builder-image = "old/builder"
`), 0666))
			var err error
			subject, err = config.New(tmpDir)
			h.AssertNil(t, err)
		})

		it("sets the default-builder", func() {
			err := subject.SetDefaultBuilder("new/builder")
			h.AssertNil(t, err)
			b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
			h.AssertNil(t, err)
			h.AssertContains(t, string(b), `default-builder-image = "new/builder"`)
			h.AssertEq(t, subject.DefaultBuilder, "new/builder")
		})
	})

	when("Config#GetRunImage", func() {
		var subject *config.Config

		when("run image exists in config", func() {
			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
[[run-images]]
  image = "some/run-image"
  mirrors = ["some/run-image1", "some.registry/some/run-image"]
`), 0666))
				var err error
				subject, err = config.New(tmpDir)
				h.AssertNil(t, err)
			})

			it("returns the builder config", func() {
				runImage := subject.GetRunImage("some/run-image")
				h.AssertNotNil(t, runImage)
				h.AssertEq(t, runImage.Image, "some/run-image")
				h.AssertEq(t, len(runImage.Mirrors), 2)
				h.AssertSliceContains(t, runImage.Mirrors, "some/run-image1")
				h.AssertSliceContains(t, runImage.Mirrors, "some.registry/some/run-image")
			})
		})

		when("run image does not exist in config", func() {
			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
[[run-images]]
  image = "some-other/run-image"
  mirrors = ["some/run", "some.registry/some/run"]
`), 0666))
				var err error
				subject, err = config.New(tmpDir)
				h.AssertNil(t, err)
			})

			it("returns a nil pointer", func() {
				builder := subject.GetRunImage("some/builder")
				h.AssertNil(t, builder)
			})
		})
	})

	when("Config#SetRunImageMirrors", func() {
		var subject *config.Config

		when("run image exists in config", func() {
			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
[[run-images]]
  image = "some/run-image"
  mirrors = ["some/run", "some.registry/some/run"]
`), 0666))
				var err error
				subject, err = config.New(tmpDir)
				h.AssertNil(t, err)
			})

			it("updates the run image", func() {
				subject.SetRunImageMirrors("some/run-image", []string{"some-other/run"})

				reloadedConfig, err := config.New(tmpDir)
				h.AssertNil(t, err)

				image := reloadedConfig.GetRunImage("some/run-image")

				h.AssertNotNil(t, image)
				h.AssertEq(t, image.Image, "some/run-image")
				h.AssertEq(t, len(image.Mirrors), 1)
				h.AssertSliceContains(t, image.Mirrors, "some-other/run")
			})
		})

		when("run image does not exist in config", func() {
			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), nil, 0666))
				var err error
				subject, err = config.New(tmpDir)
				h.AssertNil(t, err)
			})

			it("adds the run image", func() {
				subject.SetRunImageMirrors("some/run-image", []string{"some-other/run"})

				reloadedConfig, err := config.New(tmpDir)
				h.AssertNil(t, err)

				image := reloadedConfig.GetRunImage("some/run-image")

				h.AssertNotNil(t, image)
				h.AssertEq(t, image.Image, "some/run-image")
				h.AssertEq(t, len(image.Mirrors), 1)
				h.AssertSliceContains(t, image.Mirrors, "some-other/run")
			})
		})
	})

	when("ImageByRegistry", func() {
		var images []string
		it.Before(func() {
			images = []string{
				"first.com/org/repo",
				"myorg/myrepo",
				"zonal.gcr.io/org/repo",
				"gcr.io/org/repo",
			}
		})
		when("repoName is dockerhub", func() {
			it("returns the dockerhub image", func() {
				name, err := config.ImageByRegistry("index.docker.io", images)
				h.AssertNil(t, err)
				h.AssertEq(t, name, "myorg/myrepo")
			})
		})
		when("registry is gcr.io", func() {
			it("returns the gcr.io image", func() {
				name, err := config.ImageByRegistry("gcr.io", images)
				h.AssertNil(t, err)
				h.AssertEq(t, name, "gcr.io/org/repo")
			})
			when("registry is zonal.gcr.io", func() {
				it("returns the gcr image", func() {
					name, err := config.ImageByRegistry("zonal.gcr.io", images)
					h.AssertNil(t, err)
					h.AssertEq(t, name, "zonal.gcr.io/org/repo")
				})
			})
			when("registry is missingzone.gcr.io", func() {
				it("returns first run image", func() {
					name, err := config.ImageByRegistry("missingzone.gcr.io", images)
					h.AssertNil(t, err)
					h.AssertEq(t, name, "first.com/org/repo")
				})
			})
		})

		when("one of the images is non-parsable", func() {
			it.Before(func() {
				images = []string{"as@ohd@as@op", "gcr.io/myorg/myrepo"}
			})
			it("skips over it", func() {
				name, err := config.ImageByRegistry("gcr.io", images)
				h.AssertNil(t, err)
				h.AssertEq(t, name, "gcr.io/myorg/myrepo")
			})
		})
	})
}
