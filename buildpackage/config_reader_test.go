package buildpackage_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/paths"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuildpackageConfigReader(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Buildpackage Config Reader", testBuildpackageConfigReader, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildpackageConfigReader(t *testing.T, when spec.G, it spec.S) {
	when("#Read", func() {
		var tmpDir string

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "buildpackage-config-test")
			h.AssertNil(t, err)
		})

		it.After(func() {
			os.RemoveAll(tmpDir)
		})

		it("returns correct config when provided toml file is valid", func() {
			configFile := filepath.Join(tmpDir, "package.toml")

			err := ioutil.WriteFile(configFile, []byte(validPackageToml), os.ModePerm)
			h.AssertNil(t, err)

			packageConfigReader := buildpackage.NewConfigReader()

			config, err := packageConfigReader.Read(configFile)
			h.AssertNil(t, err)

			h.AssertEq(t, config.Buildpack.URI, "https://example.com/bp/a.tgz")
			h.AssertEq(t, len(config.Dependencies), 1)
			h.AssertEq(t, config.Dependencies[0].URI, "https://example.com/bp/b.tgz")
		})

		it("returns an error when toml decode fails", func() {
			configFile := filepath.Join(tmpDir, "package.toml")

			err := ioutil.WriteFile(configFile, []byte(brokenPackageToml), os.ModePerm)
			h.AssertNil(t, err)

			packageConfigReader := buildpackage.NewConfigReader()

			_, err = packageConfigReader.Read(configFile)
			h.AssertNotNil(t, err)

			h.AssertError(t, err, "decoding toml")
		})

		it("expands relative file uris to absolute paths relative to config file", func() {
			configFile := filepath.Join(tmpDir, "package.toml")

			err := ioutil.WriteFile(configFile, []byte(relativePathsPackageToml), os.ModePerm)
			h.AssertNil(t, err)

			packageConfigReader := buildpackage.NewConfigReader()

			config, err := packageConfigReader.Read(configFile)
			h.AssertNil(t, err)

			expectedURI, err := paths.FilePathToURI(filepath.Join(tmpDir, "bp", "a"))
			h.AssertNil(t, err)
			h.AssertEq(t, config.Buildpack.URI, expectedURI)

			expectedURI, err = paths.FilePathToURI(filepath.Join(tmpDir, "bp", "b"))
			h.AssertNil(t, err)
			h.AssertEq(t, config.Dependencies[0].URI, expectedURI)
		})

		it("returns an error when buildpack uri is invalid", func() {
			configFile := filepath.Join(tmpDir, "package.toml")

			err := ioutil.WriteFile(configFile, []byte(invalidBPURIPackageToml), os.ModePerm)
			h.AssertNil(t, err)

			packageConfigReader := buildpackage.NewConfigReader()

			_, err = packageConfigReader.Read(configFile)
			h.AssertNotNil(t, err)
			h.AssertError(t, err, "getting absolute path for")
			h.AssertError(t, err, "https@@@@@@://example.com/bp/a.tgz")
		})

		it("returns an error when dependency uri is invalid", func() {
			configFile := filepath.Join(tmpDir, "package.toml")

			err := ioutil.WriteFile(configFile, []byte(invalidDepURIPackageToml), os.ModePerm)
			h.AssertNil(t, err)

			packageConfigReader := buildpackage.NewConfigReader()

			_, err = packageConfigReader.Read(configFile)
			h.AssertNotNil(t, err)
			h.AssertError(t, err, "getting absolute path for")
			h.AssertError(t, err, "https@@@@@@://example.com/bp/b.tgz")
		})

		it("returns an error when unknown array table is present", func() {
			configFile := filepath.Join(tmpDir, "package.toml")

			err := ioutil.WriteFile(configFile, []byte(invalidDepTablePackageToml), os.ModePerm)
			h.AssertNil(t, err)

			packageConfigReader := buildpackage.NewConfigReader()

			_, err = packageConfigReader.Read(configFile)
			h.AssertNotNil(t, err)
			h.AssertError(t, err, "unknown configuration element")
			h.AssertError(t, err, "dependenceis")
			h.AssertNotContains(t, err.Error(), ".image")
			h.AssertError(t, err, configFile)
		})

		it("returns an error when unknown buildpack key is present", func() {
			configFile := filepath.Join(tmpDir, "package.toml")

			err := ioutil.WriteFile(configFile, []byte(unknownBPKeyPackageToml), os.ModePerm)
			h.AssertNil(t, err)

			packageConfigReader := buildpackage.NewConfigReader()

			_, err = packageConfigReader.Read(configFile)
			h.AssertNotNil(t, err)
			h.AssertError(t, err, "unknown configuration element")
			h.AssertError(t, err, "buildpack.url")
			h.AssertError(t, err, configFile)
		})

		it("returns an error when multiple unknown keys are present", func() {
			configFile := filepath.Join(tmpDir, "package.toml")

			err := ioutil.WriteFile(configFile, []byte(multipleUnknownKeysPackageToml), os.ModePerm)
			h.AssertNil(t, err)

			packageConfigReader := buildpackage.NewConfigReader()

			_, err = packageConfigReader.Read(configFile)
			h.AssertNotNil(t, err)
			h.AssertError(t, err, "unknown configuration elements")
			h.AssertError(t, err, "'buildpack.url'")
			h.AssertError(t, err, "', '")
			h.AssertError(t, err, "'dependenceis'")
		})

		it("returns an error when both dependency options are configured", func() {
			configFile := filepath.Join(tmpDir, "package.toml")

			err := ioutil.WriteFile(configFile, []byte(conflictingDependencyKeysPackageToml), os.ModePerm)
			h.AssertNil(t, err)

			packageConfigReader := buildpackage.NewConfigReader()

			_, err = packageConfigReader.Read(configFile)
			h.AssertNotNil(t, err)
			h.AssertError(t, err, "dependency configured with both 'uri' and 'image'")
		})

		it("returns an error no buildpack is configured", func() {
			configFile := filepath.Join(tmpDir, "package.toml")

			err := ioutil.WriteFile(configFile, []byte(missingBuildpackPackageToml), os.ModePerm)
			h.AssertNil(t, err)

			packageConfigReader := buildpackage.NewConfigReader()

			_, err = packageConfigReader.Read(configFile)
			h.AssertNotNil(t, err)
			h.AssertError(t, err, "missing 'buildpack.uri' configuration")
		})
	})
}

const validPackageToml = `
[buildpack]
uri = "https://example.com/bp/a.tgz"

[[dependencies]]
uri = "https://example.com/bp/b.tgz"
`

const brokenPackageToml = `
[buildpack # missing closing bracket
uri = "https://example.com/bp/a.tgz"

[dependencies]] # missing opening bracket
uri = "https://example.com/bp/b.tgz"
`

const relativePathsPackageToml = `
[buildpack]
uri = "bp/a"

[[dependencies]]
uri = "bp/b"
`

const invalidBPURIPackageToml = `
[buildpack]
uri = "https@@@@@@://example.com/bp/a.tgz"
`

const invalidDepURIPackageToml = `
[buildpack]
uri = "noop-buildpack.tgz"

[[dependencies]]
uri = "https@@@@@@://example.com/bp/b.tgz"
`

const invalidDepTablePackageToml = `
[buildpack]
uri = "noop-buildpack.tgz"

[[dependenceis]] # Notice: this is misspelled
image = "some/package-dep"
`

const unknownBPKeyPackageToml = `
[buildpack]
url = "noop-buildpack.tgz"
`

const multipleUnknownKeysPackageToml = `
[buildpack]
url = "noop-buildpack.tgz"

[[dependenceis]] # Notice: this is misspelled
image = "some/package-dep"
`

const conflictingDependencyKeysPackageToml = `
[buildpack]
uri = "noop-buildpack.tgz"

[[dependencies]]
uri = "bp/b"
image = "some/package-dep"
`

const missingBuildpackPackageToml = `
[[dependencies]]
uri = "bp/b"
`
