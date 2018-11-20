package fs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/idtools"
)

type FS struct {
}

func (f *FS) CreateTGZFile(tarFile, srcDir, tarDir string, uid, gid int) error {
	fh, err := os.Create(tarFile)
	if err != nil {
		return fmt.Errorf("create file for tar: %s", err)
	}
	defer fh.Close()
	gzw := gzip.NewWriter(fh)
	defer gzw.Close()
	rc, err := f.CreateTarReader(srcDir, tarDir, uid, gid)
	if err != nil {
		return fmt.Errorf("create tar for tgz: %s", err)
	}
	defer rc.Close()
	_, err = io.Copy(gzw, rc)
	return err
}

func (*FS) CreateTarReader(srcDir, tarDir string, uid, gid int) (io.ReadCloser, error) {
	name := filepath.Base(srcDir)
	tarOptions := &archive.TarOptions{
		IncludeFiles: []string{name},
		RebaseNames: map[string]string{
			name: tarDir,
		},
	}
	if uid > 0 && gid > 0 {
		tarOptions.ChownOpts = &idtools.Identity{
			UID: uid,
			GID: gid,
		}
	}
	return archive.TarWithOptions(filepath.Dir(srcDir), tarOptions)
}

func (*FS) CreateSingleFileTar(path, txt string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: path, Size: int64(len(txt)), Mode: 0666}); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(txt)); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return bytes.NewReader(buf.Bytes()), nil
}

func (*FS) AddTextToTar(tw *tar.Writer, name string, contents []byte) error {
	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(contents))}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(contents)
	return err
}

func (*FS) AddFileToTar(tw *tar.Writer, name string, contents *os.File) error {
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

func (*FS) Untar(r io.Reader, dest string) error {
	return archive.Untar(r, dest, &archive.TarOptions{NoLchown: true})
}
