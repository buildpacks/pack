package pack

import (
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/onsi/gomega/ghttp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/internal/mocks"
	"github.com/buildpack/pack/internal/paths"
	h "github.com/buildpack/pack/testhelpers"
)

func TestDownloader(t *testing.T) {
	spec.Run(t, "Downloader", testDownloader, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testDownloader(t *testing.T, when spec.G, it spec.S) {
	when("#Download", func() {
		var (
			cacheDir   string
			err        error
			originalWd string
			subject    *Downloader
			tgz        string
			tmpDir     string
		)

		it.Before(func() {
			tmpDir, err = ioutil.TempDir("", "test-downloader")
			h.AssertNil(t, err)

			cacheDir, err = ioutil.TempDir(tmpDir, "cache")
			h.AssertNil(t, err)

			subject = NewDownloader(mocks.NewMockLogger(ioutil.Discard), cacheDir)

			testDataDir := filepath.Join("testdata", "downloader", "dirA")
			h.AssertNil(t, os.MkdirAll(filepath.Join(tmpDir, testDataDir), 0777))
			h.RecursiveCopy(t, testDataDir, filepath.Join(tmpDir, testDataDir))

			tgz = filepath.Join(tmpDir, "dirA.tgz")
			err = os.Rename(h.CreateTgz(t, testDataDir, "./", 0777), tgz)
			h.AssertNil(t, err)

			originalWd, err = os.Getwd()
			h.AssertNil(t, err)

			err := os.Chdir(tmpDir)
			h.AssertNil(t, err)
		})

		it.After(func() {
			h.AssertNil(t, os.Chdir(originalWd))
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		when("is path", func() {
			when("is absolute", func() {
				it("downloads from directory", func() {
					absPath, err := filepath.Abs(filepath.Join("testdata", "downloader", "dirA"))
					h.AssertNil(t, err)

					out, err := subject.Download(absPath)
					h.AssertNil(t, err)
					h.AssertNotEq(t, out, "")
					h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
				})

				it("downloads from tgz", func() {
					absPath, err := filepath.Abs(tgz)
					h.AssertNil(t, err)

					out, err := subject.Download(absPath)
					h.AssertNil(t, err)
					h.AssertMatch(t, out, `\.tgz$`)
					h.AssertOnTarEntry(t, out, "file.txt", h.ContentEquals("some file contents"))
				})
			})

			when("is relative", func() {
				it("downloads from directory", func() {
					out, err := subject.Download(filepath.Join("testdata", "downloader", "dirA"))
					h.AssertNil(t, err)
					h.AssertNotEq(t, out, "")
					h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
				})

				it("downloads from tgz", func() {
					out, err := subject.Download(filepath.Base(tgz))
					h.AssertNil(t, err)
					h.AssertMatch(t, out, `\.tgz$`)
					h.AssertOnTarEntry(t, out, "file.txt", h.ContentEquals("some file contents"))
				})
			})
		})

		when("is uri", func() {
			it("downloads from a 'file://' URI directory", func() {
				absPath, err := filepath.Abs(filepath.Join("testdata", "downloader", "dirA"))
				h.AssertNil(t, err)

				uri, err := paths.FilePathToUri(absPath)
				h.AssertNil(t, err)

				out, err := subject.Download(uri)
				h.AssertNil(t, err)
				h.AssertNotEq(t, out, "")
				h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
			})

			it("downloads from a 'file://' URI tgz", func() {
				absPath, err := filepath.Abs(tgz)
				h.AssertNil(t, err)

				uri, err := paths.FilePathToUri(absPath)
				h.AssertNil(t, err)

				out, err := subject.Download(uri)
				h.AssertNil(t, err)
				h.AssertMatch(t, out, `\.tgz$`)
				h.AssertOnTarEntry(t, out, "file.txt", h.ContentEquals("some file contents"))
			})

			it("downloads from a 'http(s)://' URI tgz", func() {
				server := ghttp.NewServer()
				server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
					http.ServeFile(w, r, tgz)
				})
				defer server.Close()

				out, err := subject.Download(server.URL() + "/downloader/somefile.tgz")
				h.AssertNil(t, err)
				h.AssertMatch(t, out, `\.tgz$`)
				h.AssertOnTarEntry(t, out, "file.txt", h.ContentEquals("some file contents"))
			})

			it("caches to versioned directory", func() {
				server := ghttp.NewServer()
				server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
					http.ServeFile(w, r, tgz)
				})
				defer server.Close()

				out, err := subject.Download(server.URL() + "/downloader/somefile.tgz")
				h.AssertNil(t, err)
				h.AssertContains(t, out, filepath.Join(cacheDir, cacheDirPrefix+cacheVersion))
			})

			it("uses cache from a 'http(s)://' URI tgz", func() {
				server := ghttp.NewServer()
				server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Add("ETag", "A")
					http.ServeFile(w, r, tgz)
				})
				server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(304)
				})
				defer server.Close()

				out, err := subject.Download(server.URL() + "/downloader/somefile.tgz")
				h.AssertNil(t, err)
				h.AssertMatch(t, out, `\.tgz$`)
				h.AssertOnTarEntry(t, out, "file.txt", h.ContentEquals("some file contents"))

				out, err = subject.Download(server.URL() + "/downloader/somefile.tgz")
				h.AssertNil(t, err)
				h.AssertMatch(t, out, `\.tgz$`)
				h.AssertOnTarEntry(t, out, "file.txt", h.ContentEquals("some file contents"))
			})
		})
	})
}
