package pack

import (
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/buildpack/pack/internal/mocks"
	"github.com/onsi/gomega/ghttp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpack/pack/testhelpers"
)

func TestDownloader(t *testing.T) {
	spec.Run(t, "Downloader", testDownloader, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testDownloader(t *testing.T, when spec.G, it spec.S) {
	when("#Download", func() {
		var (
			err      error
			tmpDir   string
			cacheDir string
			subject  *Downloader
		)

		it.Before(func() {
			if runtime.GOOS == "windows" {
				t.Skip("do not run on windows")
			}

			tmpDir, err = ioutil.TempDir("", "")
			h.AssertNil(t, err)

			cacheDir, err = ioutil.TempDir("", "")
			h.AssertNil(t, err)

			subject = NewDownloader(mocks.NewMockLogger(ioutil.Discard), cacheDir)
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
			h.AssertNil(t, os.RemoveAll(cacheDir))
		})

		it("download from a relative directory", func() {
			out, err := subject.Download(filepath.Join("testdata", "downloader", "dirA"))
			h.AssertNil(t, err)
			h.AssertNotEq(t, out, "")
			h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
		})

		it("download from a relative tgz", func() {
			out, err := subject.Download(filepath.Join("testdata", "downloader", "dirA.tgz"))
			h.AssertNil(t, err)
			h.AssertNotEq(t, out, "")
			h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
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
			absPath, err := filepath.Abs(filepath.Join("testdata", "downloader", "dirA.tgz"))
			h.AssertNil(t, err)

			out, err := subject.Download(absPath)
			h.AssertNil(t, err)
			h.AssertNotEq(t, out, "")
			h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
		})

		it("download from a 'file://' URI directory", func() {
			absPath, err := filepath.Abs(filepath.Join("testdata", "downloader", "dirA"))
			h.AssertNil(t, err)

			out, err := subject.Download("file://" + absPath)
			h.AssertNil(t, err)
			h.AssertNotEq(t, out, "")
			h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
		})

		it("download from a 'file://' URI tgz", func() {
			absPath, err := filepath.Abs(filepath.Join("testdata", "downloader", "dirA.tgz"))
			h.AssertNil(t, err)

			out, err := subject.Download("file://" + absPath)
			h.AssertNil(t, err)
			h.AssertNotEq(t, out, "")
			h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
		})

		it("download from a 'http(s)://' URI tgz", func() {
			server := ghttp.NewServer()
			server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
				path := filepath.Join("testdata", r.URL.Path)
				http.ServeFile(w, r, path)
			})
			defer server.Close()

			out, err := subject.Download(server.URL() + "/downloader/dirA.tgz")
			h.AssertNil(t, err)
			h.AssertNotEq(t, out, "")
			h.AssertDirContainsFileWithContents(t, out, "file.txt", "some file contents")
		})
	})
}
