package archive_test

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/archive"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestArchive(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Archive", testArchive, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testArchive(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "create-tar-test")
		if err != nil {
			t.Fatalf("failed to create tmp dir %s: %s", tmpDir, err)
		}
	})

	it.After(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatalf("failed to clean up tmp dir %s: %s", tmpDir, err)
		}
	})

	when("#ReadTarEntry", func() {
		var (
			err     error
			tarFile *os.File
		)
		it.Before(func() {
			tarFile, err = ioutil.TempFile(tmpDir, "file.tgz")
			h.AssertNil(t, err)
		})

		it.After(func() {
			_ = tarFile.Close()
		})

		when("tgz has the path", func() {
			it.Before(func() {
				err = archive.CreateSingleFileTar(tarFile.Name(), "file1", "file-1 content")
				h.AssertNil(t, err)
			})

			it("returns the file contents", func() {
				_, contents, err := archive.ReadTarEntry(tarFile, "file1")
				h.AssertNil(t, err)
				h.AssertEq(t, string(contents), "file-1 content")
			})
		})

		when("tgz has ./path", func() {
			it.Before(func() {
				err = archive.CreateSingleFileTar(tarFile.Name(), "./file1", "file-1 content")
				h.AssertNil(t, err)
			})

			it("returns the file contents", func() {
				_, contents, err := archive.ReadTarEntry(tarFile, "file1")
				h.AssertNil(t, err)
				h.AssertEq(t, string(contents), "file-1 content")
			})
		})
	})

	when("#WriteDirToTar", func() {
		var src string
		it.Before(func() {
			src = filepath.Join("testdata", "dir-to-tar")
		})

		when("mode is set to 0777", func() {
			it("writes a tar to the dest dir with 0777", func() {
				fh, err := os.Create(filepath.Join(tmpDir, "some.tar"))
				h.AssertNil(t, err)

				tw := tar.NewWriter(fh)

				err = archive.WriteDirToTar(tw, src, "/nested/dir/dir-in-archive", 1234, 2345, 0777, true, nil)
				h.AssertNil(t, err)
				h.AssertNil(t, tw.Close())
				h.AssertNil(t, fh.Close())

				file, err := os.Open(filepath.Join(tmpDir, "some.tar"))
				h.AssertNil(t, err)
				defer file.Close()

				tr := tar.NewReader(file)

				verify := tarVerifier{t, tr, 1234, 2345}
				verify.nextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content", int64(os.ModePerm))
				verify.nextDirectory("/nested/dir/dir-in-archive/sub-dir", int64(os.ModePerm))
				if runtime.GOOS != "windows" {
					verify.nextSymLink("/nested/dir/dir-in-archive/sub-dir/link-file", "../some-file.txt")
				}
			})
		})

		when("mode is set to -1", func() {
			it("writes a tar to the dest dir with preexisting file mode", func() {
				fh, err := os.Create(filepath.Join(tmpDir, "some.tar"))
				h.AssertNil(t, err)

				tw := tar.NewWriter(fh)

				err = archive.WriteDirToTar(tw, src, "/nested/dir/dir-in-archive", 1234, 2345, -1, true, nil)
				h.AssertNil(t, err)
				h.AssertNil(t, tw.Close())
				h.AssertNil(t, fh.Close())

				file, err := os.Open(filepath.Join(tmpDir, "some.tar"))
				h.AssertNil(t, err)
				defer file.Close()

				tr := tar.NewReader(file)

				verify := tarVerifier{t, tr, 1234, 2345}
				verify.nextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content", fileMode(t, filepath.Join(src, "some-file.txt")))
				verify.nextDirectory("/nested/dir/dir-in-archive/sub-dir", fileMode(t, filepath.Join(src, "sub-dir")))
				if runtime.GOOS != "windows" {
					verify.nextSymLink("/nested/dir/dir-in-archive/sub-dir/link-file", "../some-file.txt")
				}
			})
		})

		when("normalize mod time is false", func() {
			it("does not normalize mod times", func() {
				tarFile := filepath.Join(tmpDir, "some.tar")
				fh, err := os.Create(tarFile)
				h.AssertNil(t, err)

				tw := tar.NewWriter(fh)

				err = archive.WriteDirToTar(tw, src, "/foo", 1234, 2345, 0777, false, nil)
				h.AssertNil(t, err)
				h.AssertNil(t, tw.Close())
				h.AssertNil(t, fh.Close())

				h.AssertOnTarEntry(t, tarFile, "/foo/some-file.txt",
					h.DoesNotHaveModTime(archive.NormalizedDateTime),
				)
			})
		})

		when("normalize mod time is true", func() {
			it("normalizes mod times", func() {
				tarFile := filepath.Join(tmpDir, "some.tar")
				fh, err := os.Create(tarFile)
				h.AssertNil(t, err)

				tw := tar.NewWriter(fh)

				err = archive.WriteDirToTar(tw, src, "/foo", 1234, 2345, 0777, true, nil)
				h.AssertNil(t, err)
				h.AssertNil(t, tw.Close())
				h.AssertNil(t, fh.Close())

				h.AssertOnTarEntry(t, tarFile, "/foo/some-file.txt",
					h.HasModTime(archive.NormalizedDateTime),
				)
			})
		})

		when("is posix", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS == "windows", "Skipping on windows")
			})

			when("socket is present", func() {
				var (
					err        error
					tmpSrcDir  string
					fakeSocket net.Listener
				)

				it.Before(func() {
					tmpSrcDir, err = ioutil.TempDir("", "socket-test")
					h.AssertNil(t, err)

					fakeSocket, err = net.Listen(
						"unix",
						filepath.Join(tmpSrcDir, "fake-socket"),
					)

					err = ioutil.WriteFile(filepath.Join(tmpSrcDir, "fake-file"), []byte("some-content"), 0777)
					h.AssertNil(t, err)
				})

				it.After(func() {
					os.RemoveAll(tmpSrcDir)
					fakeSocket.Close()
				})

				it("silently ignore socket", func() {
					fh, err := os.Create(filepath.Join(tmpDir, "some.tar"))
					h.AssertNil(t, err)

					tw := tar.NewWriter(fh)

					err = archive.WriteDirToTar(tw, tmpSrcDir, "/nested/dir/dir-in-archive", 1234, 2345, 0777, true, nil)
					h.AssertNil(t, err)
					h.AssertNil(t, tw.Close())
					h.AssertNil(t, fh.Close())

					file, err := os.Open(filepath.Join(tmpDir, "some.tar"))
					h.AssertNil(t, err)
					defer file.Close()

					tr := tar.NewReader(file)

					verify := tarVerifier{t, tr, 1234, 2345}
					verify.nextFile(
						"/nested/dir/dir-in-archive/fake-file",
						"some-content",
						0777,
					)
					verify.noMoreFilesExist()
				})
			})
		})
	})

	when("#WriteZipToTar", func() {
		var src string
		it.Before(func() {
			src = filepath.Join("testdata", "zip-to-tar.zip")
		})

		when("mode is set to 0777", func() {
			it("writes a tar to the dest dir with 0777", func() {
				fh, err := os.Create(filepath.Join(tmpDir, "some.tar"))
				h.AssertNil(t, err)

				tw := tar.NewWriter(fh)

				err = archive.WriteZipToTar(tw, src, "/nested/dir/dir-in-archive", 1234, 2345, 0777, true, nil)
				h.AssertNil(t, err)
				h.AssertNil(t, tw.Close())
				h.AssertNil(t, fh.Close())

				file, err := os.Open(filepath.Join(tmpDir, "some.tar"))
				h.AssertNil(t, err)
				defer file.Close()

				tr := tar.NewReader(file)

				verify := tarVerifier{t, tr, 1234, 2345}
				verify.nextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content", 0777)
				verify.nextDirectory("/nested/dir/dir-in-archive/sub-dir", 0777)
				if runtime.GOOS != "windows" {
					verify.nextSymLink("/nested/dir/dir-in-archive/sub-dir/link-file", "../some-file.txt")
				}
			})
		})

		when("mode is set to -1", func() {
			it("writes a tar to the dest dir with preexisting file mode", func() {
				fh, err := os.Create(filepath.Join(tmpDir, "some.tar"))
				h.AssertNil(t, err)

				tw := tar.NewWriter(fh)

				err = archive.WriteZipToTar(tw, src, "/nested/dir/dir-in-archive", 1234, 2345, -1, true, nil)
				h.AssertNil(t, err)
				h.AssertNil(t, tw.Close())
				h.AssertNil(t, fh.Close())

				file, err := os.Open(filepath.Join(tmpDir, "some.tar"))
				h.AssertNil(t, err)
				defer file.Close()

				tr := tar.NewReader(file)

				verify := tarVerifier{t, tr, 1234, 2345}
				verify.nextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content", 0644)
				verify.nextDirectory("/nested/dir/dir-in-archive/sub-dir", 0755)
				if runtime.GOOS != "windows" {
					verify.nextSymLink("/nested/dir/dir-in-archive/sub-dir/link-file", "../some-file.txt")
				}
			})
		})

		when("normalize mod time is false", func() {
			it("does not normalize mod times", func() {
				tarFile := filepath.Join(tmpDir, "some.tar")
				fh, err := os.Create(tarFile)
				h.AssertNil(t, err)

				tw := tar.NewWriter(fh)

				err = archive.WriteZipToTar(tw, src, "/foo", 1234, 2345, 0777, false, nil)
				h.AssertNil(t, err)
				h.AssertNil(t, tw.Close())
				h.AssertNil(t, fh.Close())

				h.AssertOnTarEntry(t, tarFile, "/foo/some-file.txt",
					h.DoesNotHaveModTime(archive.NormalizedDateTime),
				)
			})
		})

		when("normalize mod time is true", func() {
			it("normalizes mod times", func() {
				tarFile := filepath.Join(tmpDir, "some.tar")
				fh, err := os.Create(tarFile)
				h.AssertNil(t, err)

				tw := tar.NewWriter(fh)

				err = archive.WriteZipToTar(tw, src, "/foo", 1234, 2345, 0777, true, nil)
				h.AssertNil(t, err)
				h.AssertNil(t, tw.Close())
				h.AssertNil(t, fh.Close())

				h.AssertOnTarEntry(t, tarFile, "/foo/some-file.txt",
					h.HasModTime(archive.NormalizedDateTime),
				)
			})
		})
	})
}

func fileMode(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat %s", path)
	}
	mode := int64(info.Mode() & os.ModePerm)
	return mode
}

type tarVerifier struct {
	t   *testing.T
	tr  *tar.Reader
	uid int
	gid int
}

func (v *tarVerifier) nextDirectory(name string, mode int64) {
	v.t.Helper()
	header, err := v.tr.Next()
	if err != nil {
		v.t.Fatalf("Failed to get next file: %s", err)
	}

	if header.Name != name {
		v.t.Fatalf(`expected dir with name %s, got %s`, name, header.Name)
	}
	if header.Typeflag != tar.TypeDir {
		v.t.Fatalf(`expected %s to be a Directory`, header.Name)
	}
	if header.Uid != v.uid {
		v.t.Fatalf(`expected %s to have uid %d but, got: %d`, header.Name, v.uid, header.Uid)
	}
	if header.Gid != v.gid {
		v.t.Fatalf(`expected %s to have gid %d but, got: %d`, header.Name, v.gid, header.Gid)
	}
	if header.Mode != mode {
		v.t.Fatalf(`expected %s to have mode %o but, got: %o`, header.Name, mode, header.Mode)
	}
	if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
		v.t.Fatalf(`expected %s to have been normalized, got: %s`, header.Name, header.ModTime.String())
	}
}

func (v *tarVerifier) noMoreFilesExist() {
	v.t.Helper()
	header, err := v.tr.Next()
	if err == nil {
		v.t.Fatalf(`expected no more files but found: %s`, header.Name)
	} else if err != io.EOF {
		v.t.Error(err.Error())
	}
}

func (v *tarVerifier) nextFile(name, expectedFileContents string, expectedFileMode int64) {
	v.t.Helper()
	header, err := v.tr.Next()
	if err != nil {
		v.t.Fatalf("Failed to get next file: %s", err)
	}

	if header.Name != name {
		v.t.Fatalf(`expected dir with name %s, got %s`, name, header.Name)
	}
	if header.Typeflag != tar.TypeReg {
		v.t.Fatalf(`expected %s to be a file`, header.Name)
	}
	if header.Uid != v.uid {
		v.t.Fatalf(`expected %s to have uid %d but, got: %d`, header.Name, v.uid, header.Uid)
	}
	if header.Gid != v.gid {
		v.t.Fatalf(`expected %s to have gid %d but, got: %d`, header.Name, v.gid, header.Gid)
	}

	fileContents := make([]byte, header.Size)
	v.tr.Read(fileContents)
	if string(fileContents) != expectedFileContents {
		v.t.Fatalf(`expected to some-file.txt to have %s got %s`, expectedFileContents, string(fileContents))
	}

	if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
		v.t.Fatalf(`expected %s to have been normalized, got: %s`, header.Name, header.ModTime.String())
	}

	if header.Mode != expectedFileMode {
		v.t.Fatalf("files should have mode %o, got: %o", expectedFileMode, header.Mode)
	}
}

func (v *tarVerifier) nextSymLink(name, link string) {
	v.t.Helper()
	header, err := v.tr.Next()
	if err != nil {
		v.t.Fatalf("Failed to get next file: %s", err)
	}

	if header.Name != name {
		v.t.Fatalf(`expected dir with name %s, got %s`, name, header.Name)
	}
	if header.Typeflag != tar.TypeSymlink {
		v.t.Fatalf(`expected %s to be a link got %s`, header.Name, string(header.Typeflag))
	}
	if header.Uid != v.uid {
		v.t.Fatalf(`expected %s to have uid %d but, got: %d`, header.Name, v.uid, header.Uid)
	}
	if header.Gid != v.gid {
		v.t.Fatalf(`expected %s to have gid %d but, got: %d`, header.Name, v.gid, header.Gid)
	}

	if header.Linkname != "../some-file.txt" {
		v.t.Fatalf(`expected to link-file to have target %s got: %s`, link, header.Linkname)
	}
	if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
		v.t.Fatalf(`expected %s to have been normalized, got: %s`, header.Name, header.ModTime.String())
	}
}
