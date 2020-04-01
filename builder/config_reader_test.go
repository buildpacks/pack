package builder_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/builder"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestConfig(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
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

				it("returns error when obsolete 'groups' field is used", func() {
					_, warns, err := builder.ReadConfig(builderConfigPath)
					h.AssertError(t, err, "parse contents of")
					h.AssertEq(t, len(warns), 0)
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

					h.AssertSliceContainsOnly(t, warns, "empty 'order' definition")
				})
			})

			when("'uri' is misspelled as url", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(builderConfigPath, []byte(`
[buildpack]
url = "noop-buildpack.tgz"
`), 0666))
				})

				it("returns errors when 'uri' is misspelled as 'url'", func() {
					_, _, err := builder.ReadConfig(builderConfigPath)
					h.AssertError(t, err, "unknown configuration elements 'buildpack'")
				})
			})
			when("'buildpack' is misspelled", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(builderConfigPath, []byte(`
[buidlpack]
uri = "noop-buildpack.tgz"
`), 0666))
				})

				it("returns errors when 'buildpack' is misspelled as 'buidlpack'", func() {
					_, _, err := builder.ReadConfig(builderConfigPath)
					h.AssertError(t, err, "unknown configuration elements 'buidlpack'")
				})
			})
		})
	})
}
