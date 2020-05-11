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
	"github.com/buildpacks/pack/internal/dist"
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

	when("#Stack.Validate()", func() {
		var (
			testID         = "testID"
			testRunImage   = "test-run-image"
			testBuildImage = "test-build-image"
		)

		it("returns error if no id", func() {
			config := builder.StackConfig{
				BuildImage: testBuildImage,
				RunImage:   testRunImage,
			}
			h.AssertError(t, config.Validate(), "stack.id is required")
		})

		it("returns error if no build image", func() {
			config := builder.StackConfig{
				ID:       testID,
				RunImage: testRunImage,
			}
			h.AssertError(t, config.Validate(), "stack.build-image is required")
		})

		it("returns error if no run image", func() {
			config := builder.StackConfig{
				ID:         testID,
				BuildImage: testBuildImage,
			}
			h.AssertError(t, config.Validate(), "stack.run-image is required")
		})
	})

	when("Buildpacks", func() {
		var bpCollection builder.BuildpackCollection
		var bp1, bp2, imageBP builder.BuildpackConfig

		it.Before(func() {
			bp1 = builder.BuildpackConfig{ImageOrURI: dist.ImageOrURI{BuildpackURI: dist.BuildpackURI{URI: "test1"}}}
			bp2 = builder.BuildpackConfig{ImageOrURI: dist.ImageOrURI{BuildpackURI: dist.BuildpackURI{URI: "test2"}}}

			imageBP = builder.BuildpackConfig{ImageOrURI: dist.ImageOrURI{ImageRef: dist.ImageRef{ImageName: "testref"}}}

			bpCollection = builder.BuildpackCollection{bp1, bp2, imageBP}
		})

		it("returns Buildpacks", func() {
			buildpacks := bpCollection.Buildpacks()
			h.AssertEq(t, buildpacks[0].URI, bp1.URI)
			h.AssertEq(t, buildpacks[1].URI, bp2.URI)
			h.AssertEq(t, len(buildpacks), 2)
		})
	})

	when("#Packages", func() {
		var bpCollection builder.BuildpackCollection
		var bp1, imageBP builder.BuildpackConfig

		it.Before(func() {
			bp1 = builder.BuildpackConfig{ImageOrURI: dist.ImageOrURI{BuildpackURI: dist.BuildpackURI{URI: "test1"}}}

			imageBP = builder.BuildpackConfig{ImageOrURI: dist.ImageOrURI{ImageRef: dist.ImageRef{ImageName: "testref"}}}

			bpCollection = builder.BuildpackCollection{bp1, imageBP}
		})

		it("returns Packages", func() {
			buildpacks := bpCollection.Packages()
			h.AssertEq(t, buildpacks[0].ImageName, imageBP.ImageName)
			h.AssertEq(t, len(buildpacks), 1)
		})
	})
}
