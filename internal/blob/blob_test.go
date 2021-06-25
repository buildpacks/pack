package blob_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/blob"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBlob(t *testing.T) {
	spec.Run(t, "Buildpack", testBlob, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBlob(t *testing.T, when spec.G, it spec.S) {
	when("#Blob", func() {
		when("#Open", func() {
			var (
				blobDir  = filepath.Join("testdata", "blob")
				blobPath string
			)

			when("dir", func() {
				it.Before(func() {
					blobPath = blobDir
				})
				it("returns a tar reader", func() {
					assertBlob(t, blob.NewBlob(blobPath))
				})
			})

			when("tgz", func() {
				it.Before(func() {
					blobPath = h.CreateTGZ(t, blobDir, ".", -1)
				})

				it.After(func() {
					h.AssertNil(t, os.Remove(blobPath))
				})
				it("returns a tar reader", func() {
					assertBlob(t, blob.NewBlob(blobPath))
				})
			})

			when("tar", func() {
				it.Before(func() {
					blobPath = h.CreateTAR(t, blobDir, ".", -1)
				})

				it.After(func() {
					h.AssertNil(t, os.Remove(blobPath))
				})
				it("returns a tar reader", func() {
					assertBlob(t, blob.NewBlob(blobPath))
				})
			})

			when("RawOption is used", func() {
				when("file", func() {
					it.Before(func() {
						blobPath = filepath.Join(blobDir, "file.txt")
					})
					it("returns a file reader", func() {
						b := blob.NewBlob(blobPath, blob.RawOption)
						bReader, err := b.Open()

						h.AssertNil(t, err)
						bbuf := bytes.NewBuffer(nil)
						_, err = io.Copy(bbuf, bReader)
						h.AssertNil(t, err)

						h.AssertEq(t, bbuf.String(), "contents")
					})
				})

				when("dir", func() {
					it.Before(func() {
						blobPath = blobDir
					})
					it("returns a tar reader", func() {
						b := blob.NewBlob(blobPath, blob.RawOption)

						assertBlob(t, b)
					})
				})

				when("tgz", func() {
					it.Before(func() {
						blobPath = h.CreateTGZ(t, blobDir, ".", -1)
					})

					it.After(func() {
						h.AssertNil(t, os.Remove(blobPath))
					})
					it("returns a file reader", func() {
						b := blob.NewBlob(blobPath, blob.RawOption)

						// validate by checking that blob contents are in gzip format.
						assertBlob(t, b, hasGzip)
					})
				})

				when("tar", func() {
					it.Before(func() {
						blobPath = h.CreateTAR(t, blobDir, ".", -1)
					})

					it.After(func() {
						h.AssertNil(t, os.Remove(blobPath))
					})
					it("returns a fie reader", func() {
						b := blob.NewBlob(blobPath, blob.RawOption)

						// look for contents in tar archive
						assertBlob(t, b)
					})
				})
			})
		})
	})
}
