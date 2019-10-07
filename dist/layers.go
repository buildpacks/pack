package dist

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/archive"
)

const BuildpacksDir = "/cnb/buildpacks"

// Output:
//
// layer tar = {ID}.{V}.tar
//
// inside the layer = /cnbs/buildpacks/{ID}/{V}/*
func BuildpackLayer(dest string, uid, gid int, bp Buildpack) (string, error) {
	bpd := bp.Descriptor()
	layerTar := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", bpd.EscapedID(), bpd.Info.Version))

	fh, err := os.Create(layerTar)
	if err != nil {
		return "", fmt.Errorf("create file for tar: %s", err)
	}
	defer fh.Close()

	tw := tar.NewWriter(fh)
	defer tw.Close()

	ts := archive.NormalizedDateTime

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path.Join(BuildpacksDir, bpd.EscapedID()),
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return "", err
	}

	baseTarDir := path.Join(BuildpacksDir, bpd.EscapedID(), bpd.Info.Version)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     baseTarDir,
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return "", err
	}

	if err := embedBuildpackTar(tw, uid, gid, bp, baseTarDir); err != nil {
		return "", errors.Wrapf(err, "creating layer tar for buildpack '%s:%s'", bpd.Info.ID, bpd.Info.Version)
	}

	return layerTar, nil
}

func embedBuildpackTar(tw *tar.Writer, uid, gid int, bp Buildpack, baseTarDir string) error {
	var (
		err error
	)

	rc, err := bp.Open()
	if err != nil {
		return errors.Wrap(err, "read buildpack blob")
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "failed to get next tar entry")
		}

		header.Name = path.Clean(header.Name)
		if header.Name == "." || header.Name == "/" {
			continue
		}

		header.Name = path.Clean(path.Join(baseTarDir, header.Name))
		header.Uid = uid
		header.Gid = gid
		err = tw.WriteHeader(header)
		if err != nil {
			return errors.Wrapf(err, "failed to write header for '%s'", header.Name)
		}

		buf, err := ioutil.ReadAll(tr)
		if err != nil {
			return errors.Wrapf(err, "failed to read contents of '%s'", header.Name)
		}

		_, err = tw.Write(buf)
		if err != nil {
			return errors.Wrapf(err, "failed to write contents to '%s'", header.Name)
		}
	}

	return nil
}
