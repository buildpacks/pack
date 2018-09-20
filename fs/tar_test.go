package fs_test

import (
	"archive/tar"
	"compress/gzip"
	"github.com/buildpack/pack/fs"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFS(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "fs", testFS, spec.Report(report.Terminal{}))
}

func testFS(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir, src string
		fs fs.FS
	)

	it.Before(func(){
		var err error
		tmpDir, err = ioutil.TempDir("", "create-tar-test")
		if err != nil {
			t.Fatalf("failed to create tmp dir %s: %s", tmpDir, err)
		}
		src = filepath.Join("testdata", "dir-to-tar")
	})

	it.After(func(){
	  if err := os.RemoveAll(tmpDir); err != nil {
	  	t.Fatalf("failed to clean up tmp dir %s: %s", tmpDir, err)
	  }
	})

	it("writes a tar to the dest dir", func(){
		tarFile := filepath.Join(tmpDir, "some.tar")
		err := fs.CreateTGZFile(tarFile, src, "/dir-in-archive")
		if err != nil {
			t.Fatalf("CreateTGZFile failed: %s", err)
		}
		file, err := os.Open(tarFile)
		if err != nil {
			t.Fatalf("could not open tar file %s: %s", tarFile, err)
		}
		gzr, err := gzip.NewReader(file)
		tr := tar.NewReader(gzr)

		t.Log("handles regular files")
		header, err := tr.Next()
		if err != nil {
			t.Fatalf("Failed to get next file: %s", err)
		}
		if header.Name != "/dir-in-archive/some-file.txt" {
			t.Fatalf(`expected file with name /dir-in-archive/some-file.txt, got %s`, header.Name)
		}
		fileContents := make([]byte, header.Size, header.Size)
		tr.Read(fileContents)
		if string(fileContents) != "some-content" {
			t.Fatalf(`expected to some-file.txt to have "some-contents" got %s`, string(fileContents))
		}

		t.Log("handles symlinks")
		header, err = tr.Next()
		if err != nil {
			t.Fatalf("Failed to get next file: %s", err)
		}
		if header.Name != "/dir-in-archive/sub-dir/link-file" {
			t.Fatalf(`expected file with name /dir-in-archive/sub-dir/link-file, got %s`, header.Name)
		}

		if header.Linkname != "../some-file.txt" {
			t.Fatalf(`expected to link-file to have atrget "../some-file.txt" got %s`, header.Linkname)
		}
	})
}