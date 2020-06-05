package testhelpers

import (
	"archive/tar"
	"io"
	"testing"
	"time"
)

type TarVerifier struct {
	T   *testing.T
	Tr  *tar.Reader
	Uid int
	Gid int
}

func (v *TarVerifier) NextDirectory(name string, mode int64) {
	v.T.Helper()
	header, err := v.Tr.Next()
	if err != nil {
		v.T.Fatalf("Failed to get next file: %s", err)
	}

	if header.Name != name {
		v.T.Fatalf(`expected dir with name %s, got %s`, name, header.Name)
	}
	if header.Typeflag != tar.TypeDir {
		v.T.Fatalf(`expected %s to be a Directory`, header.Name)
	}
	if header.Uid != v.Uid {
		v.T.Fatalf(`expected %s to have Uid %d but, got: %d`, header.Name, v.Uid, header.Uid)
	}
	if header.Gid != v.Gid {
		v.T.Fatalf(`expected %s to have Gid %d but, got: %d`, header.Name, v.Gid, header.Gid)
	}
	if header.Mode != mode {
		v.T.Fatalf(`expected %s to have mode %o but, got: %o`, header.Name, mode, header.Mode)
	}
	if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
		v.T.Fatalf(`expected %s to have been normalized, got: %s`, header.Name, header.ModTime.String())
	}
}

func (v *TarVerifier) NoMoreFilesExist() {
	v.T.Helper()
	header, err := v.Tr.Next()
	if err == nil {
		v.T.Fatalf(`expected no more files but found: %s`, header.Name)
	} else if err != io.EOF {
		v.T.Error(err.Error())
	}
}

func (v *TarVerifier) NextFile(name, expectedFileContents string, expectedFileMode int64) {
	v.T.Helper()
	header, err := v.Tr.Next()
	if err != nil {
		v.T.Fatalf("Failed to get next file: %s", err)
	}

	if header.Name != name {
		v.T.Fatalf(`expected dir with name %s, got %s`, name, header.Name)
	}
	if header.Typeflag != tar.TypeReg {
		v.T.Fatalf(`expected %s to be a file`, header.Name)
	}
	if header.Uid != v.Uid {
		v.T.Fatalf(`expected %s to have Uid %d but, got: %d`, header.Name, v.Uid, header.Uid)
	}
	if header.Gid != v.Gid {
		v.T.Fatalf(`expected %s to have Gid %d but, got: %d`, header.Name, v.Gid, header.Gid)
	}

	fileContents := make([]byte, header.Size)
	v.Tr.Read(fileContents)
	if string(fileContents) != expectedFileContents {
		v.T.Fatalf(`expected to some-file.txt to have %s got %s`, expectedFileContents, string(fileContents))
	}

	if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
		v.T.Fatalf(`expected %s to have been normalized, got: %s`, header.Name, header.ModTime.String())
	}

	if header.Mode != expectedFileMode {
		v.T.Fatalf("files should have mode %o, got: %o", expectedFileMode, header.Mode)
	}
}

func (v *TarVerifier) NextSymLink(name, link string) {
	v.T.Helper()
	header, err := v.Tr.Next()
	if err != nil {
		v.T.Fatalf("Failed to get next file: %s", err)
	}

	if header.Name != name {
		v.T.Fatalf(`expected dir with name %s, got %s`, name, header.Name)
	}
	if header.Typeflag != tar.TypeSymlink {
		v.T.Fatalf(`expected %s to be a link got %s`, header.Name, string(header.Typeflag))
	}
	if header.Uid != v.Uid {
		v.T.Fatalf(`expected %s to have Uid %d but, got: %d`, header.Name, v.Uid, header.Uid)
	}
	if header.Gid != v.Gid {
		v.T.Fatalf(`expected %s to have Gid %d but, got: %d`, header.Name, v.Gid, header.Gid)
	}

	if header.Linkname != "../some-file.txt" {
		v.T.Fatalf(`expected to link-file to have target %s got: %s`, link, header.Linkname)
	}
	if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
		v.T.Fatalf(`expected %s to have been normalized, got: %s`, header.Name, header.ModTime.String())
	}
}
