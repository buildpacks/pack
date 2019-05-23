package archive_test

import (
	"archive/tar"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fatih/color"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/archive"
	h "github.com/buildpack/pack/testhelpers"
)

func TestArchive(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Archive", testArchive, spec.Report(report.Terminal{}))
}

func testArchive(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir, src string
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
		fh, err := os.Create(filepath.Join(tmpDir, "some.tar"))
		h.AssertNil(t, err)

		tw := tar.NewWriter(fh)

		err = archive.WriteDirToTar(tw, src, "/nested/dir/dir-in-archive", 1234, 2345)
		h.AssertNil(t, err)
		h.AssertNil(t, tw.Close())
		h.AssertNil(t, fh.Close())

		file, err := os.Open(filepath.Join(tmpDir, "some.tar"))
		h.AssertNil(t, err)
		defer file.Close()

		tr := tar.NewReader(file)

		verify := tarVerifier{t, tr, 1234, 2345}
		verify.nextFile("/nested/dir/dir-in-archive/some-file.txt", "some-content")
		verify.nextDirectory("/nested/dir/dir-in-archive/sub-dir", fileMode(t, filepath.Join(src, "sub-dir")))
		if runtime.GOOS != "windows" {
			verify.nextSymLink("/nested/dir/dir-in-archive/sub-dir/link-file", "../some-file.txt")
		}
	})
}

func fileMode(t *testing.T, path string) int64 {
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

	if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
		v.t.Fatalf(`expected %s to have been normalized, got: %s`, header.Name, header.ModTime.String())
	}

	if runtime.GOOS == "windows" {
		if header.Mode != 0777 {
			v.t.Fatal("files from windows should have mode 0777")
		}
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
	if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
		v.t.Fatalf(`expected %s to have been normalized, got: %s`, header.Name, header.ModTime.String())
	}
}
