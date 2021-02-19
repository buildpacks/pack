package blob_test

import (
	"compress/gzip"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/onsi/gomega/ghttp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/logging"
	"github.com/buildpacks/pack/pkg/archive"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestDownloader(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Downloader", testDownloader, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testDownloader(t *testing.T, when spec.G, it spec.S) {
	when("#Download", func() {
		var (
			cacheDir string
			err      error
			subject  pack.Downloader
		)

		it.Before(func() {
			cacheDir, err = ioutil.TempDir("", "cache")
			h.AssertNil(t, err)
			subject = blob.NewDownloader(logging.New(ioutil.Discard), cacheDir)
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(cacheDir))
		})

		when("is path", func() {
			var (
				relPath string
			)

			it.Before(func() {
				relPath = filepath.Join("testdata", "blob")
			})

			when("is absolute", func() {
				it("return the absolute path", func() {
					absPath, err := filepath.Abs(relPath)
					h.AssertNil(t, err)

					b, err := subject.Download(context.TODO(), absPath)
					h.AssertNil(t, err)
					assertBlob(t, b)
				})
			})

			when("is relative", func() {
				it("resolves the absolute path", func() {
					b, err := subject.Download(context.TODO(), relPath)
					h.AssertNil(t, err)
					assertBlob(t, b)
				})
			})

			when("path is a file:// uri", func() {
				it("resolves the absolute path", func() {
					absPath, err := filepath.Abs(relPath)
					h.AssertNil(t, err)

					uri, err := paths.FilePathToURI(absPath, "")
					h.AssertNil(t, err)

					b, err := subject.Download(context.TODO(), uri)
					h.AssertNil(t, err)
					assertBlob(t, b)
				})
			})
		})

		when("is uri", func() {
			var (
				server *ghttp.Server
				uri    string
				tgz    string
			)

			it.Before(func() {
				server = ghttp.NewServer()
				uri = server.URL() + "/downloader/somefile.tgz"

				tgz = h.CreateTGZ(t, filepath.Join("testdata", "blob"), "./", 0777)
			})

			it.After(func() {
				os.Remove(tgz)
				server.Close()
			})

			when("uri is valid", func() {
				it.Before(func() {
					server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Add("ETag", "A")
						http.ServeFile(w, r, tgz)
					})

					server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(304)
					})
				})

				it("downloads from a 'http(s)://' URI", func() {
					b, err := subject.Download(context.TODO(), uri)
					h.AssertNil(t, err)
					assertBlob(t, b)
				})

				it("uses cache from a 'http(s)://' URI tgz", func() {
					b, err := subject.Download(context.TODO(), uri)
					h.AssertNil(t, err)
					assertBlob(t, b)

					b, err = subject.Download(context.TODO(), uri)
					h.AssertNil(t, err)
					assertBlob(t, b)
				})
			})

			when("Options are used", func() {
				when("RawDownload option", func() {
					var tgz string
					it.Before(func() {
						tgz = h.CreateTGZ(t, filepath.Join("testdata", "blob"), "./", 0777)
					})

					it.After(func() {
						os.Remove(tgz)
					})
					when("uri", func() {
						var (
							server *ghttp.Server
							uri    string
						)

						it.Before(func() {
							server = ghttp.NewServer()
							uri = server.URL() + "/downloader/somefile.tgz"

							server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
								w.Header().Add("ETag", "A")
								http.ServeFile(w, r, tgz)
							})

							server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
								w.WriteHeader(304)
							})
						})
						it.After(func() {
							server.Close()
						})

						it("downloads and reads URI contents as raw bytes", func() {
							b, err := subject.Download(context.TODO(), uri, blob.RawDownload)
							h.AssertNil(t, err)

							// validate by checking that blob contents are in gzip format.
							assertBlob(t, b, hasGzip)

						})

						it("downloads and reads cached request as raw bytes", func() {
							b, err := subject.Download(context.TODO(), uri, blob.RawDownload)
							h.AssertNil(t, err)

							assertBlob(t, b, hasGzip)

							// second download should use cache
							b, err = subject.Download(context.TODO(), uri, blob.RawDownload)
							h.AssertNil(t, err)

							// validate by checking that blob contents are in gzip format.
							assertBlob(t, b, hasGzip)
						})
					})

					when("file", func() {
						it("opens and reads raw bytes", func() {
							absPath, err := filepath.Abs(tgz)
							h.AssertNil(t, err)

							b, err := subject.Download(context.TODO(), absPath, blob.RawDownload)
							h.AssertNil(t, err)

							// validate by checking that blob contents are in gzip format.
							assertBlob(t, b, hasGzip)
						})

					})
					when("followed by non-raw download", func() {
						it("does not preform a second raw download", func() {
							absPath, err := filepath.Abs(tgz)
							h.AssertNil(t, err)

							b, err := subject.Download(context.TODO(), absPath, blob.RawDownload)
							h.AssertNil(t, err)

							// validate by checking that blob contents are in gzip format.
							assertBlob(t, b, hasGzip)

							// second non-raw download
							b, err = subject.Download(context.TODO(), absPath)
							h.AssertNil(t, err)
							assertBlob(t, b) //is not raw!
						})
					})
				})

				when("ValidateDownload option", func() {
					when("file", func() {
						var absPath string
						it.Before(func() {
							absPath, err = filepath.Abs(tgz)
							h.AssertNil(t, err)
						})
						it("validates file sha256's match", func() {
							_, err = subject.Download(context.TODO(), absPath, blob.ValidateDownload("f6d3b9d05f1a56deb4830eaf6529c91e92423f984bf5451f04765123e5ece6e6"))
							h.AssertNil(t, err)
						})
						when("sh256 values do not match", func() {
							it("returns an error", func() {
								_, err = subject.Download(context.TODO(), absPath, blob.ValidateDownload("bad-sha256"))
								h.AssertError(t, err, `validation failed, expected "sha256:bad-sha256", got "sha256:f6d3b9d05f1a56deb4830eaf6529c91e92423f984bf5451f04765123e5ece6e6"`)
							})
						})
					})

					when("uri", func() {
						var (
							server *ghttp.Server
							uri    string
						)

						it.Before(func() {
							server = ghttp.NewServer()
							uri = server.URL() + "/downloader/somefile.tgz"

							server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
								w.Header().Add("ETag", "A")
								http.ServeFile(w, r, tgz)
							})

							server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
								w.WriteHeader(304)
							})
						})
						it.After(func() {
							server.Close()
						})
						it("validates file sha256's match", func() {
							_, err := subject.Download(context.TODO(), uri, blob.ValidateDownload("f6d3b9d05f1a56deb4830eaf6529c91e92423f984bf5451f04765123e5ece6e6"))
							h.AssertNil(t, err)
						})
						when("sha256 values to not match", func() {
							it("returns an error", func() {
								_, err := subject.Download(context.TODO(), uri, blob.ValidateDownload("bad-sha256"))
								h.AssertError(t, err, `validation failed, expected "sha256:bad-sha256", got "sha256:f6d3b9d05f1a56deb4830eaf6529c91e92423f984bf5451f04765123e5ece6e6"`)
							})
						})
					})
					when("sha256 values do not match", func() {
						when("uri", func() {

						})
					})
				})
			})

			when("uri is invalid", func() {
				when("uri file is not found", func() {
					it.Before(func() {
						server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(404)
						})
					})

					it("should return error", func() {
						_, err := subject.Download(context.TODO(), uri)
						h.AssertError(t, err, "could not download")
						h.AssertError(t, err, "http status '404'")
					})
				})

				when("uri is unsupported", func() {
					it("should return error", func() {
						_, err := subject.Download(context.TODO(), "not-supported://file.tgz")
						h.AssertError(t, err, "unsupported protocol 'not-supported'")
					})
				})
			})
		})
	})
}

type blobFormatOption func(t *testing.T, r io.Reader) io.Reader

func hasGzip(t *testing.T, r io.Reader) io.Reader {
	t.Helper()

	gr, err := gzip.NewReader(r)
	h.AssertNil(t, err)

	return gr
}

func assertBlob(t *testing.T, b blob.Blob, formatOpts ...blobFormatOption) {
	t.Helper()
	r, err := b.Open()
	h.AssertNil(t, err)
	defer r.Close()

	var fr io.Reader = r
	for _, opt := range formatOpts {
		fr = opt(t, fr)
	}

	_, bytes, err := archive.ReadTarEntry(fr, "file.txt")
	h.AssertNil(t, err)

	h.AssertEq(t, string(bytes), "contents")
}
