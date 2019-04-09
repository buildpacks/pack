package archive

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func WriteTarFile(sourceDir, dest string, uid, gid int) (string, error) {
	hasher := sha256.New()
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := io.MultiWriter(hasher, f)

	if WriteTarArchive(w, sourceDir, uid, gid) != nil {
		return "", err
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
	return "sha256:" + sha, nil
}

func WriteTarArchive(w io.Writer, srcDir string, uid, gid int) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	err := writeParentDirectoryHeaders(srcDir, tw, uid, gid)
	if err != nil {
		return err
	}

	return filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		var header *tar.Header
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(file)
			if err != nil {
				return err
			}
			header, err = tar.FileInfoHeader(fi, target)
			if err != nil {
				return err
			}
		} else {
			header, err = tar.FileInfoHeader(fi, fi.Name())
			if err != nil {
				return err
			}
		}
		header.Name = file
		header.ModTime = time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)
		header.Uid = uid
		header.Gid = gid
		header.Uname = ""
		header.Gname = ""

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeParentDirectoryHeaders(tarDir string, tw *tar.Writer, uid int, gid int) error {
	parent := filepath.Dir(tarDir)
	if parent == "." || parent == "/" {
		return nil
	} else {
		if err := writeParentDirectoryHeaders(parent, tw, uid, gid); err != nil {
			return err
		}

		info, err := os.Stat(parent)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, parent)
		if err != nil {
			return err
		}
		header.Name = parent
		header.ModTime = time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		return nil
	}
}

func AddTextToTar(tw *tar.Writer, name string, contents []byte) error {
	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(contents))}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(contents)
	return err
}

func AddFileToTar(tw *tar.Writer, name string, contents *os.File) error {
	fi, err := contents.Stat()
	if err != nil {
		return err
	}
	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(fi.Size())}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, contents)
	return err
}

func Untar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			return nil
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dest, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			_, err := os.Stat(filepath.Dir(path))
			if os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return err
				}
			}

			fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, hdr.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(fh, tr); err != nil {
				fh.Close()
				return err
			}
			fh.Close()
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, path); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown file type in tar %d", hdr.Typeflag)
		}
	}
}
