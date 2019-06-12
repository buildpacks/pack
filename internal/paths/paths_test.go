package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpack/pack/testhelpers"
)

func TestPaths(t *testing.T) {
	spec.Run(t, "Paths", testPaths, spec.Report(report.Terminal{}))
}

func testPaths(t *testing.T, when spec.G, it spec.S) {
	when("#FilePathToUri", func() {
		when("is windows", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")
			})

			when("path is absolute", func() {
				it("returns uri", func() {
					uri, err := FilePathToUri(`C:\some\file.txt`)
					h.AssertNil(t, err)
					h.AssertEq(t, uri, `file:///C:/some/file.txt`)
				})
			})

			when("path is relative", func() {
				var (
					err    error
					ogDir  string
					tmpDir string
				)
				it.Before(func() {
					ogDir, err = os.Getwd()
					h.AssertNil(t, err)

					tmpDir = os.TempDir()

					err = os.Chdir(tmpDir)
					h.AssertNil(t, err)
				})

				it.After(func() {
					err := os.Chdir(ogDir)
					h.AssertNil(t, err)
				})

				it("returns uri", func() {
					cwd, err := os.Getwd()
					h.AssertNil(t, err)

					uri, err := FilePathToUri(`some\file.tgz`)
					h.AssertNil(t, err)

					h.AssertEq(t, uri, fmt.Sprintf(`file:///%s/some/file.tgz`, filepath.ToSlash(cwd)))
				})
			})
		})

		when("is *nix", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS == "windows", "Skipped on windows")
			})

			when("path is absolute", func() {
				it("returns uri", func() {
					uri, err := FilePathToUri("/tmp/file.tgz")
					h.AssertNil(t, err)
					h.AssertEq(t, uri, "file:///tmp/file.tgz")
				})
			})

			when("path is relative", func() {
				it("returns uri", func() {
					h.SkipIf(t, runtime.GOOS == "windows", "Skipped on windows")

					cwd, err := os.Getwd()
					h.AssertNil(t, err)

					uri, err := FilePathToUri("some/file.tgz")
					h.AssertNil(t, err)

					h.AssertEq(t, uri, fmt.Sprintf("file://%s/some/file.tgz", cwd))
				})
			})
		})
	})

	when("#UriToFilePath", func() {
		when("is windows", func() {
			when("uri is drive", func() {
				it("returns path", func() {
					h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")

					path, err := UriToFilePath(`file:///c:/laptop/file.tgz`)
					h.AssertNil(t, err)

					h.AssertEq(t, path, `c:\laptop\file.tgz`)
				})
			})

			when("uri is network share", func() {
				it("returns path", func() {
					h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")

					path, err := UriToFilePath(`file://laptop/file.tgz`)
					h.AssertNil(t, err)

					h.AssertEq(t, path, `\\laptop\file.tgz`)
				})
			})
		})

		when("is *nix", func() {
			when("uri is valid", func() {
				it("returns path", func() {
					h.SkipIf(t, runtime.GOOS == "windows", "Skipped on windows")

					path, err := UriToFilePath(`file:///tmp/file.tgz`)
					h.AssertNil(t, err)

					h.AssertEq(t, path, `/tmp/file.tgz`)
				})
			})
		})
	})
}
