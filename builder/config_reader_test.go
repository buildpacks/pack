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
  id = "buildpack/1"
  version = "0.0.1"
  uri = "https://example.com/buildpack-1.tgz"

[[buildpacks]]
  image = "example.com/buildpack:2"

[[buildpacks]]
  uri = "https://example.com/buildpack-3.tgz"

[[order]]
[[order.group]]
  id = "buildpack/1"

[assets]
[[assets.package]]
  uri = "some-asset-file-uri"
[[assets.package]]
  image = "some-asset-image-tag"
`), 0666))
			})

			it("returns a builder config", func() {
				builderConfig, warns, err := builder.ReadConfig(builderConfigPath)
				h.AssertNil(t, err)
				h.AssertEq(t, len(warns), 0)

				h.AssertEq(t, builderConfig.Buildpacks[0].ID, "buildpack/1")
				h.AssertEq(t, builderConfig.Buildpacks[0].Version, "0.0.1")
				h.AssertEq(t, builderConfig.Buildpacks[0].URI, "https://example.com/buildpack-1.tgz")
				h.AssertEq(t, builderConfig.Buildpacks[0].ImageName, "")

				h.AssertEq(t, builderConfig.Buildpacks[1].ID, "")
				h.AssertEq(t, builderConfig.Buildpacks[1].URI, "")
				h.AssertEq(t, builderConfig.Buildpacks[1].ImageName, "example.com/buildpack:2")

				h.AssertEq(t, builderConfig.Buildpacks[2].ID, "")
				h.AssertEq(t, builderConfig.Buildpacks[2].URI, "https://example.com/buildpack-3.tgz")
				h.AssertEq(t, builderConfig.Buildpacks[2].ImageName, "")

				h.AssertEq(t, builderConfig.Order[0].Group[0].ID, "buildpack/1")
				h.AssertEq(t, builderConfig.Assets.Packages[0].URI, "some-asset-file-uri")
				h.AssertEq(t, builderConfig.Assets.Packages[1].ImageName, "some-asset-image-tag")
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

			when("unknown buildpack key is present", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
url = "noop-buildpack.tgz"
`), 0666))
				})

				it("returns an error", func() {
					_, _, err := builder.ReadConfig(builderConfigPath)
					h.AssertError(t, err, "unknown configuration element 'buildpacks.url'")
				})
			})

			when("unknown array table is present", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(builderConfigPath, []byte(`
[[buidlpack]]
uri = "noop-buildpack.tgz"
`), 0666))
				})

				it("returns an error", func() {
					_, _, err := builder.ReadConfig(builderConfigPath)
					h.AssertError(t, err, "unknown configuration element 'buidlpack'")
				})
			})
		})
	})

	when("#ValidateConfig()", func() {
		var (
			testID         = "testID"
			testRunImage   = "test-run-image"
			testBuildImage = "test-build-image"
		)

		it("returns error if no id", func() {
			config := builder.Config{
				Stack: builder.StackConfig{
					BuildImage: testBuildImage,
					RunImage:   testRunImage,
				}}
			h.AssertError(t, builder.ValidateConfig(config), "stack.id is required")
		})

		it("returns error if no build image", func() {
			config := builder.Config{
				Stack: builder.StackConfig{
					ID:       testID,
					RunImage: testRunImage,
				}}
			h.AssertError(t, builder.ValidateConfig(config), "stack.build-image is required")
		})

		it("returns error if no run image", func() {
			config := builder.Config{
				Stack: builder.StackConfig{
					ID:         testID,
					BuildImage: testBuildImage,
				}}
			h.AssertError(t, builder.ValidateConfig(config), "stack.run-image is required")
		})
	})
}
