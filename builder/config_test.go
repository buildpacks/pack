package builder_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/builder"
	h "github.com/buildpack/pack/testhelpers"
)

func TestConfig(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "testConfig", testConfig, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testConfig(t *testing.T, when spec.G, it spec.S) {
	when("#ReadConfig", func() {
		var (
			tmpDir            string
			builderConfigPath string
			err               error
		)

		it.Before(func() {
			tmpDir, err = ioutil.TempDir("", "config-test")
			h.AssertNil(t, err)
			builderConfigPath = filepath.Join(tmpDir, "builder.toml")
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		when("file is written properly", func() {
			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
  id = "some.buildpack"

[[order]]
[[order.group]]
  id = "some.buildpack"
`), 0666))
			})

			it("returns a builder config", func() {
				builderConfig, warns, err := builder.ReadConfig(builderConfigPath)
				h.AssertNil(t, err)
				h.AssertEq(t, len(warns), 0)
				h.AssertEq(t, builderConfig.Buildpacks[0].ID, "some.buildpack")
			})
		})

		when("an error occurs while reading", func() {
			it("bubbles up the error", func() {
				_, _, err := builder.ReadConfig(builderConfigPath)
				h.AssertError(t, err, "opening config file")
			})
		})

		when("detecting warnings", func() {
			when("'latest' field is used on buildpack", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
  id = "some.buildpack"
  latest = true

[[buildpacks]]
  id = "other.buildpack"
  latest = true

[[order]]
[[order.group]]
  id = "some.buildpack"
`), 0666))
				})

				it("returns a warning", func() {
					_, warns, err := builder.ReadConfig(builderConfigPath)
					h.AssertNil(t, err)

					h.AssertEq(t, len(warns), 1)
					h.AssertSliceContains(t, warns, "'latest' field on a buildpack is obsolete and will be ignored")
				})

			})

			when("'groups' field is used", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
  id = "some.buildpack"
  version = "some.buildpack.version"

[[groups]]
[[groups.buildpacks]]
  id = "some.buildpack"
  version = "some.buildpack.version"

[[order]]
[[order.group]]
  id = "some.buildpack"
`), 0666))
				})

				it("returns warnings", func() {
					_, warns, err := builder.ReadConfig(builderConfigPath)
					h.AssertNil(t, err)

					h.AssertEq(t, len(warns), 1)
					h.AssertSliceContains(t, warns, "'groups' field is obsolete in favor of 'order'")
				})
			})

			when("'order' is missing or empty", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
  id = "some.buildpack"
  version = "some.buildpack.version"
`), 0666))
				})

				it("returns warnings", func() {
					_, warns, err := builder.ReadConfig(builderConfigPath)
					h.AssertNil(t, err)

					h.AssertEq(t, len(warns), 1)
					h.AssertSliceContains(t, warns, "empty 'order' definition")
				})
			})
		})
	})
}
