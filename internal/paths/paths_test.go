package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/pack/testhelpers"
)

func TestPaths(t *testing.T) {
	spec.Run(t, "Paths", testPaths, spec.Report(report.Terminal{}))
}

func testPaths(t *testing.T, when spec.G, it spec.S) {
	when("#FilterReservedNames", func() {
		when("volume contains a reserved name", func() {
			it("modifies the volume name", func() {
				volumeName := "auxauxaux"
				subject := FilterReservedNames(volumeName)
				expected := "a_u_xa_u_xa_u_x"
				if subject != expected {
					t.Fatalf("The volume should not contain reserved names")
				}
			})
		})

		when("volume does not contain reserved names", func() {
			it("does not modify the volume name", func() {
				volumeName := "lbtlbtlbt"
				subject := FilterReservedNames(volumeName)
				if subject != volumeName {
					t.Fatalf("The volume should not be modified")
				}
			})
		})
	})
	when("#FilePathToURI", func() {
		when("is windows", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")
			})

			when("path is absolute", func() {
				it("returns uri", func() {
					uri, err := FilePathToURI(`C:\some\file.txt`)
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

					uri, err := FilePathToURI(`some\file.tgz`)
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
					uri, err := FilePathToURI("/tmp/file.tgz")
					h.AssertNil(t, err)
					h.AssertEq(t, uri, "file:///tmp/file.tgz")
				})
			})

			when("path is relative", func() {
				it("returns uri", func() {
					h.SkipIf(t, runtime.GOOS == "windows", "Skipped on windows")

					cwd, err := os.Getwd()
					h.AssertNil(t, err)

					uri, err := FilePathToURI("some/file.tgz")
					h.AssertNil(t, err)

					h.AssertEq(t, uri, fmt.Sprintf("file://%s/some/file.tgz", cwd))
				})
			})
		})
	})

	when("#URIToFilePath", func() {
		when("is windows", func() {
			when("uri is drive", func() {
				it("returns path", func() {
					h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")

					path, err := URIToFilePath(`file:///c:/laptop/file.tgz`)
					h.AssertNil(t, err)

					h.AssertEq(t, path, `c:\laptop\file.tgz`)
				})
			})

			when("uri is network share", func() {
				it("returns path", func() {
					h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")

					path, err := URIToFilePath(`file://laptop/file.tgz`)
					h.AssertNil(t, err)

					h.AssertEq(t, path, `\\laptop\file.tgz`)
				})
			})
		})

		when("is *nix", func() {
			when("uri is valid", func() {
				it("returns path", func() {
					h.SkipIf(t, runtime.GOOS == "windows", "Skipped on windows")

					path, err := URIToFilePath(`file:///tmp/file.tgz`)
					h.AssertNil(t, err)

					h.AssertEq(t, path, `/tmp/file.tgz`)
				})
			})
		})
	})
}
