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
			err      error
			tmpDir   string
			tgz      string
			cacheDir string
			subject  *Downloader
		)

		it.Before(func() {
			tmpDir, err = ioutil.TempDir("", "")
			h.AssertNil(t, err)

			cacheDir, err = ioutil.TempDir("", "")
			h.AssertNil(t, err)

			subject = NewDownloader(mocks.NewMockLogger(ioutil.Discard), cacheDir)

			tgz = h.CreateTgz(t, filepath.Join("testdata", "downloader", "dirA"), "./", 0777)
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tgz))
			h.AssertNil(t, os.RemoveAll(tmpDir))
			h.AssertNil(t, os.RemoveAll(cacheDir))
		})

		it("download from a relative directory", func() {
			out, err := subject.Download(filepath.Join("testdata", "downloader", "dirA"))
			h.AssertNil(t, err)
			h.AssertNotEq(t, out, "")
			h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
		})

		when("relative", func() {
			var (
				ogWd string
				err  error
			)

			it.Before(func() {
				ogWd, err = os.Getwd()
				h.AssertNil(t, err)

				err := os.Chdir(filepath.Dir(tgz))
				h.AssertNil(t, err)
			})

			it.After(func() {
				err := os.Chdir(ogWd)
				h.AssertNil(t, err)
			})

			it("download from tgz", func() {
				out, err := subject.Download(filepath.Base(tgz))
				h.AssertNil(t, err)
				h.AssertMatch(t, out, `\.tgz$`)
				h.AssertOnTarEntry(t, out, "file.txt", h.ContentEquals("some file contents"))
			})
		})

		it("download from an absolute directory", func() {
			absPath, err := filepath.Abs(filepath.Join("testdata", "downloader", "dirA"))
			h.AssertNil(t, err)

			out, err := subject.Download(absPath)
			h.AssertNil(t, err)
			h.AssertNotEq(t, out, "")
			h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
		})

		it("download from an absolute tgz", func() {
			absPath, err := filepath.Abs(tgz)
			h.AssertNil(t, err)

			out, err := subject.Download(absPath)
			h.AssertNil(t, err)
			h.AssertMatch(t, out, `\.tgz$`)
			h.AssertOnTarEntry(t, out, "file.txt", h.ContentEquals("some file contents"))
		})

		it("download from a 'file://' URI directory", func() {
			absPath, err := filepath.Abs(filepath.Join("testdata", "downloader", "dirA"))
			h.AssertNil(t, err)

			uri, err := paths.FilePathToUri(absPath)
			h.AssertNil(t, err)

			out, err := subject.Download(uri)
			h.AssertNil(t, err)
			h.AssertNotEq(t, out, "")
			h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
		})

		it("download from a 'file://' URI tgz", func() {
			absPath, err := filepath.Abs(tgz)
			h.AssertNil(t, err)

			uri, err := paths.FilePathToUri(absPath)
			h.AssertNil(t, err)

			out, err := subject.Download(uri)
			h.AssertNil(t, err)
			h.AssertMatch(t, out, `\.tgz$`)
			h.AssertOnTarEntry(t, out, "file.txt", h.ContentEquals("some file contents"))
		})

		it("download from a 'http(s)://' URI tgz", func() {
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

		it("use cache from a 'http(s)://' URI tgz", func() {
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
}
