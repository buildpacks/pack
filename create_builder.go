package pack

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/img"
	"github.com/pkg/errors"
)

type BuilderConfig struct {
	RepoName   string
	Buildpacks []Buildpack                `toml:"buildpacks"`
	Groups     []lifecycle.BuildpackGroup `toml:"groups"`
	Stack      Stack
	NoPull     bool
}

type Buildpack struct {
	ID  string
	URI string
}

type BuilderFactory struct {
	FS FS
}

//go:generate mockgen -package mocks -destination mocks/fs.go github.com/buildpack/pack FS
type FS interface {
	CreateTGZFile(tarFile, srcDir, tarDir string, uid, gid int) error
	CreateTarReader(srcDir, tarDir string, uid, gid int) (io.Reader, chan error)
	Untar(r io.Reader, dest string) error
	CreateSingleFileTar(path, txt string) (io.Reader, error)
}

func (f *BuilderFactory) Create(config BuilderConfig) error {
	builderStore, err := repoStore(config.RepoName, true)
	if err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir("", "create-builder")
	if err != nil {
		return err
	}
	defer os.Remove(tmpDir)

	orderTar, err := f.orderLayer(tmpDir, config.Groups)
	if err != nil {
		return err
	}
	builderImage, _, err := img.Append(config.Stack.BuildImage, orderTar)
	if err != nil {
		return err
	}
	for _, buildpack := range config.Buildpacks {
		tarFile, err := f.buildpackLayer(tmpDir, buildpack)
		if err != nil {
			return err
		}
		builderImage, _, err = img.Append(builderImage, tarFile)
		if err != nil {
			return err
		}
	}

	return builderStore.Write(builderImage)
}

type order struct {
	Groups []lifecycle.BuildpackGroup `toml:"groups"`
}

func (f *BuilderFactory) orderLayer(dest string, groups []lifecycle.BuildpackGroup) (layerTar string, err error) {
	buildpackDir := filepath.Join(dest, "buildpack")
	err = os.Mkdir(buildpackDir, 0755)
	if err != nil {
		return "", err
	}

	orderFile, err := os.Create(filepath.Join(buildpackDir, "order.toml"))
	if err != nil {
		return "", err
	}
	defer orderFile.Close()
	err = toml.NewEncoder(orderFile).Encode(order{Groups: groups})
	if err != nil {
		return "", err
	}
	layerTar = filepath.Join(dest, "order.tar")
	if err := f.FS.CreateTGZFile(layerTar, buildpackDir, "/buildpacks", 0, 0); err != nil {
		return "", err
	}
	return layerTar, nil
}

func (f *BuilderFactory) buildpackLayer(dest string, buildpack Buildpack) (layerTar string, err error) {
	dir := strings.TrimPrefix(buildpack.URI, "file://")
	var data struct {
		BP struct {
			ID      string `toml:"id"`
			Version string `toml:"version"`
		} `toml:"buildpack"`
	}
	_, err = toml.DecodeFile(filepath.Join(dir, "buildpack.toml"), &data)
	if err != nil {
		return "", errors.Wrapf(err, "reading buildpack.toml from buildpack: %s", filepath.Join(dir, "buildpack.toml"))
	}
	bp := data.BP
	if buildpack.ID != bp.ID {
		return "", fmt.Errorf("buildpack ids did not match: %s != %s", buildpack.ID, bp.ID)
	}
	if bp.Version == "" {
		return "", fmt.Errorf("buildpack.toml must provide version: %s", filepath.Join(dir, "buildpack.toml"))
	}
	tarFile := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", buildpack.ID, bp.Version))
	if err := f.FS.CreateTGZFile(tarFile, dir, filepath.Join("/buildpacks", buildpack.ID, bp.Version), 0, 0); err != nil {
		return "", err
	}
	return tarFile, err
}
