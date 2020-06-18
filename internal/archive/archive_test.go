package archive_test

import (
	"archive/tar"
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

				verify := h.NewTarVerifier(t, tr, 1234, 2345)
				verify.NextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content", int64(os.ModePerm))
				verify.NextDirectory("/nested/dir/dir-in-archive/sub-dir", int64(os.ModePerm))
				if runtime.GOOS != "windows" {
					verify.NextSymLink("/nested/dir/dir-in-archive/sub-dir/link-file", "../some-file.txt")
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

				verify := h.NewTarVerifier(t, tr, 1234, 2345)
				verify.NextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content", fileMode(t, filepath.Join(src, "some-file.txt")))
				verify.NextDirectory("/nested/dir/dir-in-archive/sub-dir", fileMode(t, filepath.Join(src, "sub-dir")))
				if runtime.GOOS != "windows" {
					verify.NextSymLink("/nested/dir/dir-in-archive/sub-dir/link-file", "../some-file.txt")
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

					verify := h.NewTarVerifier(t, tr, 1234, 2345)
					verify.NextFile(
						"/nested/dir/dir-in-archive/fake-file",
						"some-content",
						0777,
					)
					verify.NoMoreFilesExist()
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

				verify := h.NewTarVerifier(t, tr, 1234, 2345)
				verify.NextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content", 0777)
				verify.NextDirectory("/nested/dir/dir-in-archive/sub-dir", 0777)
				if runtime.GOOS != "windows" {
					verify.NextSymLink("/nested/dir/dir-in-archive/sub-dir/link-file", "../some-file.txt")
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

				verify := h.NewTarVerifier(t, tr, 1234, 2345)
				verify.NextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content", 0644)
				verify.NextDirectory("/nested/dir/dir-in-archive/sub-dir", 0755)
				if runtime.GOOS != "windows" {
					verify.NextSymLink("/nested/dir/dir-in-archive/sub-dir/link-file", "../some-file.txt")
				}
			})

			when("files are compressed in fat (MSDOS) format", func() {
				it.Before(func() {
					src = filepath.Join("testdata", "fat-zip-to-tar.zip")
				})

				it("writes a tar to the dest dir with 0777", func() {
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

					verify := h.NewTarVerifier(t, tr, 1234, 2345)
					verify.NextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content", 0777)
					verify.NoMoreFilesExist()
				})
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

	when("#IsZip", func() {
		when("file is a zip file", func() {
			it("returns true", func() {
				path := filepath.Join("testdata", "zip-to-tar.zip")

				file, err := os.Open(path)
				h.AssertNil(t, err)
				defer file.Close()

				isZip, err := archive.IsZip(file)
				h.AssertNil(t, err)
				h.AssertTrue(t, isZip)
			})
		})

		when("file is a jar file", func() {
			it("returns true", func() {
				path := filepath.Join("testdata", "jar-file.jar")

				file, err := os.Open(path)
				h.AssertNil(t, err)
				defer file.Close()

				isZip, err := archive.IsZip(file)
				h.AssertNil(t, err)
				h.AssertTrue(t, isZip)
			})
		})

		when("file is not a zip file", func() {
			when("file has some content", func() {
				it("returns false", func() {
					file, err := ioutil.TempFile(tmpDir, "file.txt")
					h.AssertNil(t, err)
					defer file.Close()

					err = ioutil.WriteFile(file.Name(), []byte("content"), os.ModePerm)
					h.AssertNil(t, err)

					isZip, err := archive.IsZip(file)
					h.AssertNil(t, err)
					h.AssertFalse(t, isZip)
				})
			})

			when("file doesn't have content", func() {
				it("returns false", func() {
					file, err := ioutil.TempFile(tmpDir, "file.txt")
					h.AssertNil(t, err)
					defer file.Close()

					isZip, err := archive.IsZip(file)
					h.AssertNil(t, err)
					h.AssertFalse(t, isZip)
				})
			})
		})

		when("reader is closed", func() {
			it("returns error", func() {
				file, err := ioutil.TempFile(tmpDir, "file.txt")
				h.AssertNil(t, err)
				err = file.Close()
				h.AssertNil(t, err)

				isZip, err := archive.IsZip(file)
				h.AssertError(t, err, os.ErrClosed.Error())
				h.AssertFalse(t, isZip)
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
