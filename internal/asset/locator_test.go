package asset_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/asset"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestLocator(t *testing.T) {
	spec.Run(t, "TestLocator", testLocator, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLocator(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = h.NewAssertionManager(t)
	)
	when("#GetLocatorType", func() {
		when("URILocator", func() {
			it("returns correct locator type", func() {
				loc := asset.GetLocatorType("scheme:///info", "")
				assert.Equal(loc, asset.URILocator)
			})
		})

		when("FilepathLocator", func() {
			when("passed a localfilepath rooted at relative base dir", func() {
				var tmpDir string
				it.Before(func() {
					var err error
					tmpDir, err = ioutil.TempDir("", "filepath-locator-test")
					assert.Nil(err)

					err = ioutil.WriteFile(filepath.Join(tmpDir, "local-locator"), []byte("some-file-content"), os.ModePerm)
					assert.Nil(err)
				})
				it.After(func() {
					os.RemoveAll(tmpDir)
				})
				it("returns correct locator type", func() {
					loc := asset.GetLocatorType("local-locator", tmpDir)
					assert.Equal(loc, asset.FilepathLocator)
				})
			})

			when("passed an absolute filepath", func() {
				var tmpFile *os.File
				it.Before(func() {
					var err error
					tmpFile, err = ioutil.TempFile("", "abs-path-locator-file")
					assert.Nil(err)
				})
				it.After(func() {
					os.Remove(tmpFile.Name())
				})
				it("return correct locator type", func() {
					loc := asset.GetLocatorType(tmpFile.Name(), "")
					assert.Equal(loc, asset.FilepathLocator)
				})
			})
		})

		when("ImageLocator", func() {
			when("passed a valid imagename", func() {
				// Note that this is also a valid filepath, but the path does not exits.
				it("returns correct locator type", func() {
					loc := asset.GetLocatorType("some-repo/some-image", "")
					assert.Equal(loc, asset.ImageLocator)
				})
			})
		})

		when("unable to parse locator", func() {
			it("returns an invalid locator", func() {
				loc := asset.GetLocatorType(":::", "")
				assert.Equal(loc, asset.InvalidLocator)
			})
		})
	})
}
