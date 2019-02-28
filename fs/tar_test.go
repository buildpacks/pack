package fs_test

import (
	"archive/tar"
	"github.com/fatih/color"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/fs"
)

func TestFS(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "fs", testFS, spec.Report(report.Terminal{}))
}

func testFS(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir, src string
		fs          fs.FS
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "create-tar-test")
		if err != nil {
			t.Fatalf("failed to create tmp dir %s: %s", tmpDir, err)
		}
		src = filepath.Join("testdata", "dir-to-tar")
	})

	it.After(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatalf("failed to clean up tmp dir %s: %s", tmpDir, err)
		}
	})

	it("writes a tar to the dest dir", func() {
		tarFile := filepath.Join(tmpDir, "some.tar")
		err := fs.CreateTarFile(tarFile, src, "/nested/dir/dir-in-archive", 1234, 2345)
		if err != nil {
			t.Fatalf("CreateTarFile failed: %s", err)
		}
		file, err := os.Open(tarFile)
		if err != nil {
			t.Fatalf("could not open tar file %s: %s", tarFile, err)
		}
		defer file.Close()
		tr := tar.NewReader(file)

		verify := tarVerifier{t, tr, 1234, 2345}
		verify.nextDirectory("/nested")
		verify.nextDirectory("/nested/dir")
		verify.nextDirectory("/nested/dir/dir-in-archive")
		verify.nextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content")
		verify.nextDirectory("/nested/dir/dir-in-archive/sub-dir")
		if runtime.GOOS != "windows" {
			verify.nextSymLink("/nested/dir/dir-in-archive/sub-dir/link-file", "../some-file.txt")
		}
	})
}

type tarVerifier struct {
	t   *testing.T
	tr  *tar.Reader
	uid int
	gid int
}

func (v *tarVerifier) nextDirectory(name string) {
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
	if header.Mode != 0755 {
		v.t.Fatalf(`expected %s to have mode %o but, got: %o`, header.Name, 0755, header.Mode)
	}
}

func (v *tarVerifier) nextFile(name, expectedFileContents string) {
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

	fileContents := make([]byte, header.Size, header.Size)
	v.tr.Read(fileContents)
	if string(fileContents) != expectedFileContents {
		v.t.Fatalf(`expected to some-file.txt to have %s got %s`, expectedFileContents, string(fileContents))
	}
}

func (v *tarVerifier) nextSymLink(name, link string) {
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
}
