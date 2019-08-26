package testhelpers

import (
	"archive/tar"
	"testing"

	"github.com/buildpack/pack/internal/archive"
)

type TarEntryAssertion func(*testing.T, *tar.Header, []byte)

func AssertOnTarEntry(t *testing.T, tarFile, entryPath string, assertFns ...TarEntryAssertion) {
	t.Helper()

	header, bytes, err := archive.ReadTarEntry(tarFile, entryPath)
	AssertNil(t, err)

	for _, fn := range assertFns {
		fn(t, header, bytes)
	}
}

func ContentEquals(expected string) TarEntryAssertion {
	return func(t *testing.T, header *tar.Header, contents []byte) {
		t.Helper()
		AssertEq(t, string(contents), expected)
	}
}

func SymlinksTo(expectedTarget string) TarEntryAssertion {
	return func(t *testing.T, header *tar.Header, _ []byte) {
		t.Helper()
		if header.Typeflag != tar.TypeSymlink {
			t.Fatalf("path '%s' is not a symlink, type flag is '%c'", header.Name, header.Typeflag)
		}

		if header.Linkname != expectedTarget {
			t.Fatalf("symlink '%s' does not point to '%s', instead it points to '%s'", header.Name, expectedTarget, header.Linkname)
		}
	}
}

func HasOwnerAndGroup(expectedUID int, expectedGID int) TarEntryAssertion {
	return func(t *testing.T, header *tar.Header, _ []byte) {
		t.Helper()
		if header.Uid != expectedUID {
			t.Fatalf("expected '%s' to have uid '%d', but got '%d'", header.Name, expectedUID, header.Uid)
		}
		if header.Gid != expectedGID {
			t.Fatalf("expected '%s' to have gid '%d', but got '%d'", header.Name, expectedGID, header.Gid)
		}
	}
}

func HasFileMode(expectedMode int64) TarEntryAssertion {
	return func(t *testing.T, header *tar.Header, _ []byte) {
		t.Helper()
		if header.Mode != expectedMode {
			t.Fatalf("expected '%s' to have mode '%o', but got '%o'", header.Name, expectedMode, header.Mode)
		}
	}
}

func IsDirectory() TarEntryAssertion {
	return func(t *testing.T, header *tar.Header, _ []byte) {
		t.Helper()
		if header.Typeflag != tar.TypeDir {
			t.Fatalf("expected '%s' to be a directory but was '%d'", header.Name, header.Typeflag)
		}
	}
}
