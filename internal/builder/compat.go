package builder

import (
	"archive/tar"
	"os"
	"path"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
)

const (
	compatLifecycleDir = "/lifecycle"
)

func (b *Builder) compatLayer(order dist.Order, dest string) (string, error) {
	if b.lifecycle == nil {
		return "", nil
	}

	compatTar := path.Join(dest, "compat.tar")
	fh, err := os.Create(compatTar)
	if err != nil {
		return "", err
	}
	defer fh.Close()

	tw := tar.NewWriter(fh)
	defer tw.Close()

	if err := compatLifecycle(tw); err != nil {
		return "", err
	}

	return compatTar, nil
}

func compatLifecycle(tw *tar.Writer) error {
	return addSymlink(tw, compatLifecycleDir, lifecycleDir)
}

func addSymlink(tw *tar.Writer, name, linkName string) error {
	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Linkname: linkName,
		Typeflag: tar.TypeSymlink,
		Mode:     0644,
		ModTime:  archive.NormalizedDateTime,
	}); err != nil {
		return errors.Wrapf(err, "creating %s symlink", style.Symbol(name))
	}
	return nil
}
