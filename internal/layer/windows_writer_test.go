package layer_test

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/buildpacks/pack/internal/layer"
	h "github.com/buildpacks/pack/testhelpers"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestWindowsWriter(t *testing.T) {
	spec.Run(t, "windows-writer", testWindowsWriter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testWindowsWriter(t *testing.T, when spec.G, it spec.S) {
	when("#WriteHeader", func() {
		when("duplicate parent directories", func() {
			it("only writes parents once", func() {
				var err error

				f, err := ioutil.TempFile("", "windows-writer.tar")
				h.AssertNil(t, err)
				defer func() { f.Close(); os.Remove(f.Name()) }()

				lw := layer.NewWindowsWriter(f)

				h.AssertNil(t, lw.WriteHeader(&tar.Header{
					Name:     "/cnb/lifecycle/first-file",
					Typeflag: tar.TypeReg,
				}))

				h.AssertNil(t, lw.WriteHeader(&tar.Header{
					Name:     "/cnb/sibling-dir",
					Typeflag: tar.TypeDir,
				}))

				h.AssertNil(t, lw.Close())

				f.Seek(0, 0)
				tr := tar.NewReader(f)

				th, _ := tr.Next()
				h.AssertEq(t, th.Name, "Files")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Hives")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Files/cnb")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Files/cnb/lifecycle")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Files/cnb/lifecycle/first-file")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeReg))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Files/cnb/sibling-dir")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				_, err = tr.Next()
				h.AssertTrue(t, err == io.EOF)
			})
		})

		when("header path begins without slash or dot", func() {
			it("writes entries", func() {
				var err error

				f, err := ioutil.TempFile("", "windows-writer.tar")
				h.AssertNil(t, err)
				defer func() { f.Close(); os.Remove(f.Name()) }()

				lw := layer.NewWindowsWriter(f)

				h.AssertNil(t, lw.WriteHeader(&tar.Header{
					Name:     "cnb/my-file",
					Typeflag: tar.TypeReg,
				}))

				h.AssertNil(t, lw.Close())

				f.Seek(0, 0)
				tr := tar.NewReader(f)

				th, _ := tr.Next()
				h.AssertEq(t, th.Name, "Files")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Hives")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Files/cnb")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Files/cnb/my-file")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeReg))

				_, err = tr.Next()
				h.AssertTrue(t, err == io.EOF)
			})
		})

		when("header path begins with slash", func() {
			it("writes entries", func() {
				var err error

				f, err := ioutil.TempFile("", "windows-writer.tar")
				h.AssertNil(t, err)
				defer func() { f.Close(); os.Remove(f.Name()) }()

				lw := layer.NewWindowsWriter(f)

				h.AssertNil(t, lw.WriteHeader(&tar.Header{
					Name:     "/cnb/my-file",
					Typeflag: tar.TypeReg,
				}))

				h.AssertNil(t, lw.Close())

				f.Seek(0, 0)

				tr := tar.NewReader(f)

				th, _ := tr.Next()
				h.AssertEq(t, th.Name, "Files")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Hives")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Files/cnb")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Files/cnb/my-file")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeReg))

				_, err = tr.Next()
				h.AssertTrue(t, err == io.EOF)
			})
		})

		when("header path begins with dot", func() {
			it("writes entries", func() {
				var err error

				f, err := ioutil.TempFile("", "windows-writer.tar")
				h.AssertNil(t, err)
				defer func() { f.Close(); os.Remove(f.Name()) }()

				lw := layer.NewWindowsWriter(f)

				h.AssertNil(t, lw.WriteHeader(&tar.Header{
					Name:     "./cnb/my-file",
					Typeflag: tar.TypeReg,
				}))

				h.AssertNil(t, lw.Close())

				f.Seek(0, 0)
				tr := tar.NewReader(f)

				th, _ := tr.Next()
				h.AssertEq(t, th.Name, "Files")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Hives")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Files/cnb")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

				th, _ = tr.Next()
				h.AssertEq(t, th.Name, "Files/cnb/my-file")
				h.AssertEq(t, th.Typeflag, byte(tar.TypeReg))

				_, err = tr.Next()
				h.AssertTrue(t, err == io.EOF)
			})
		})
	})

	when("#Close", func() {
		it("writes required parent dirs on empty image", func() {
			var err error

			f, err := ioutil.TempFile("", "windows-writer.tar")
			h.AssertNil(t, err)
			defer func() { f.Close(); os.Remove(f.Name()) }()

			lw := layer.NewWindowsWriter(f)

			err = lw.Close()
			h.AssertNil(t, err)

			f.Seek(0, 0)
			tr := tar.NewReader(f)

			th, _ := tr.Next()
			h.AssertEq(t, th.Name, "Files")
			h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

			th, _ = tr.Next()
			h.AssertEq(t, th.Name, "Hives")
			h.AssertEq(t, th.Typeflag, byte(tar.TypeDir))

			_, err = tr.Next()
			h.AssertTrue(t, err == io.EOF)
		})
	})
}
