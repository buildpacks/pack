package builder

import (
	"archive/tar"
	"bytes"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/Masterminds/semver"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/style"
)

const (
	compatBuildpacksDir = "/buildpacks"
	compatLifecycleDir  = "/lifecycle"
	compatOrderPath     = "/buildpacks/order.toml"
	compatStackPath     = "/buildpacks/stack.toml"
)

type V1Order []V1Group

type V1Group struct {
	Buildpacks []BuildpackRef `toml:"buildpacks" json:"buildpacks"`
}

type v1OrderTOML struct {
	Groups []V1Group `toml:"groups" json:"groups"`
}

func (o V1Order) ToOrder() Order {
	var order Order
	for _, gp := range o {
		var buildpacks []BuildpackRef
		for _, bp := range gp.Buildpacks {
			buildpacks = append(buildpacks, bp)
		}

		order = append(order, OrderEntry{
			Group: buildpacks,
		})
	}

	return order
}

func (o Order) ToV1Order() V1Order {
	var order V1Order
	for _, gp := range o {
		var buildpacks []BuildpackRef
		for _, bp := range gp.Group {
			buildpacks = append(buildpacks, bp)
		}

		order = append(order, V1Group{
			Buildpacks: buildpacks,
		})
	}

	return order
}

func (b *Builder) compatLayer(dest string) (string, error) {
	compatTar := path.Join(dest, "compat.tar")
	fh, err := os.Create(compatTar)
	if err != nil {
		return "", err
	}
	defer fh.Close()

	tw := tar.NewWriter(fh)
	defer tw.Close()

	if b.lifecyclePath != "" {
		if err := compatLifecycle(tw); err != nil {
			return "", err
		}
	}

	if err := b.compatBuildpacks(tw); err != nil {
		return "", err
	}

	if err := b.compatStack(tw); err != nil {
		return "", errors.Wrapf(err, "failed to add %s to compat layer", style.Symbol(compatStackPath))
	}

	if b.replaceOrder {
		if err := b.compatOrder(tw); err != nil {
			return "", errors.Wrapf(err, "failed to add %s to compat layer", style.Symbol(compatOrderPath))
		}
	}
	return compatTar, nil
}

func compatLifecycle(tw *tar.Writer) error {
	return addSymlink(tw, compatLifecycleDir, lifecycleDir)
}

func (b *Builder) compatBuildpacks(tw *tar.Writer) error {
	now := time.Now()
	if err := tw.WriteHeader(b.rootOwnedDir(compatBuildpacksDir, now)); err != nil {
		return errors.Wrapf(err, "creating %s dir in layer", style.Symbol(buildpacksDir))
	}
	for _, bp := range b.additionalBuildpacks {
		compatDir := path.Join(compatBuildpacksDir, bp.EscapedID())
		if err := tw.WriteHeader(b.rootOwnedDir(compatDir, now)); err != nil {
			return errors.Wrapf(err, "creating %s dir in layer", style.Symbol(compatDir))
		}
		compatLink := path.Join(compatDir, bp.Version)
		bpDir := path.Join(buildpacksDir, bp.EscapedID(), bp.Version)
		if err := addSymlink(tw, compatLink, bpDir); err != nil {
			return err
		}

		if lifecycleVersion := b.GetLifecycleVersion(); lifecycleVersion != nil && lifecycleVersion.LessThan(semver.MustParse("0.4.0")) {
			if err := symlinkLatest(tw, bpDir, bp, b.metadata); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *Builder) compatStack(tw *tar.Writer) error {
	stackBuf := &bytes.Buffer{}
	if err := toml.NewEncoder(stackBuf).Encode(b.metadata.Stack); err != nil {
		return errors.Wrapf(err, "failed to marshal stack.toml")
	}
	return archive.AddFileToTar(tw, compatStackPath, stackBuf.String())
}

func (b *Builder) compatOrder(tw *tar.Writer) error {
	orderContents, err := b.orderFileContents()
	if err != nil {
		return err
	}
	return archive.AddFileToTar(tw, compatOrderPath, orderContents)
}

func addSymlink(tw *tar.Writer, name, linkName string) error {
	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Linkname: linkName,
		Typeflag: tar.TypeSymlink,
		Mode:     0644,
	}); err != nil {
		return errors.Wrapf(err, "creating %s symlink", style.Symbol(name))
	}
	return nil
}

// Deprecated: The 'latest' symlink is in place for backwards compatibility only. This should be removed as soon
// as we no longer support older releases that rely on it.
func symlinkLatest(tw *tar.Writer, baseTarDir string, bp buildpack.Buildpack, metadata Metadata) error {
	for _, b := range metadata.Buildpacks {
		if b.ID == bp.ID && b.Version == bp.Version && b.Latest {
			name := fmt.Sprintf("%s/%s/%s", compatBuildpacksDir, bp.EscapedID(), "latest")
			if err := addSymlink(tw, name, baseTarDir); err != nil {
				return errors.Wrapf(err, "creating latest symlink for buildpack '%s:%s'", bp.ID, bp.Version)
			}
			break
		}
	}
	return nil
}
