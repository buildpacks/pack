// +build acceptance

package buildpacks

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/buildpacks/pack/pkg/archive"

	"github.com/pkg/errors"
)

const (
	defaultBasePath = "./"
	defaultUid      = 0
	defaultGid      = 0
	defaultMode     = 0755
)

func NewArchiveBuildpack(name string) archiveBuildpack {
	return archiveBuildpack{
		name: name,
	}
}

type archiveBuildpack struct {
	name string
}

func (a archiveBuildpack) Prepare(sourceDir, destination string) error {
	location, err := a.createTgz(sourceDir)
	if err != nil {
		return errors.Wrapf(err, "creating archive for buildpack %s", a)
	}

	err = os.Rename(location, filepath.Join(destination, a.FileName()))
	if err != nil {
		return errors.Wrapf(err, "renaming temporary archive for buildpack %s", a)
	}

	return nil
}

func (a archiveBuildpack) FileName() string {
	return fmt.Sprintf("%s.tgz", a)
}

func (a archiveBuildpack) String() string {
	return a.name
}

func (a archiveBuildpack) FullPathIn(parentFolder string) string {
	return filepath.Join(parentFolder, a.FileName())
}

func (a archiveBuildpack) createTgz(sourceDir string) (string, error) {
	tempFile, err := ioutil.TempFile("", "*.tgz")
	if err != nil {
		return "", errors.Wrap(err, "creating temporary archive")
	}
	defer tempFile.Close()

	gZipper := gzip.NewWriter(tempFile)
	defer gZipper.Close()

	tarWriter := tar.NewWriter(gZipper)
	defer tarWriter.Close()

	archiveSource := filepath.Join(sourceDir, a.name)
	err = archive.WriteDirToTar(
		tarWriter,
		archiveSource,
		defaultBasePath,
		defaultUid,
		defaultGid,
		defaultMode,
		true,
		false,
		nil,
	)
	if err != nil {
		return "", errors.Wrap(err, "writing to temporary archive")
	}

	return tempFile.Name(), nil
}

var (
	SimpleLayersParent       = &archiveBuildpack{name: "simple-layers-parent-buildpack"}
	SimpleLayers             = &archiveBuildpack{name: "simple-layers-buildpack"}
	SimpleLayersDifferentSha = &archiveBuildpack{name: "simple-layers-buildpack-different-sha"}
	InternetCapable          = &archiveBuildpack{name: "internet-capable-buildpack"}
	ReadVolume               = &archiveBuildpack{name: "read-volume-buildpack"}
	ReadWriteVolume          = &archiveBuildpack{name: "read-write-volume-buildpack"}
	ArchiveNotInBuilder      = &archiveBuildpack{name: "not-in-builder-buildpack"}
	Noop                     = &archiveBuildpack{name: "noop-buildpack"}
	Noop2                    = &archiveBuildpack{name: "noop-buildpack-2"}
	OtherStack               = &archiveBuildpack{name: "other-stack-buildpack"}
	ReadEnv                  = &archiveBuildpack{name: "read-env-buildpack"}
	NestedLevelOne           = &archiveBuildpack{name: "nested-level-1-buildpack"}
	NestedLevelTwo           = &archiveBuildpack{name: "nested-level-2-buildpack"}
)
