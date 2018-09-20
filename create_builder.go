package pack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/img"
	"github.com/pkg/errors"
)

type CreateBuilderFlags struct {
	RepoName        string
	BuilderTomlPath string
	NoPull          bool
}

type Buildpack struct {
	ID  string
	URI string
}

type Builder struct {
	Buildpacks []Buildpack                `toml:"buildpacks"`
	Groups     []lifecycle.BuildpackGroup `toml:"groups"`
}

type BuilderFactory struct {
	DefaultStack Stack
}

func (f *BuilderFactory) Create(flags CreateBuilderFlags) error {
	if !flags.NoPull {
		if out, err := exec.Command("docker", "pull", f.DefaultStack.BuildImage).CombinedOutput(); err != nil {
			fmt.Println(string(out))
			return err
		}
	}
	builderStore, err := repoStore(flags.RepoName, true)
	if err != nil {
		return err
	}

	baseImage, err := readImage(f.DefaultStack.BuildImage, true)
	if err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir("", "buildpack")
	if err != nil {
		return err
	}
	defer os.Remove(tmpDir)

	builder := Builder{}
	_, err = toml.DecodeFile(flags.BuilderTomlPath, &builder)
	if err != nil {
		return err
	}

	buildpackDir, err := f.buildpackDir(tmpDir, &builder)
	if err != nil {
		return err
	}
	if err := createTarFile(filepath.Join(tmpDir, "buildpacks.tar"), buildpackDir, "/buildpacks"); err != nil {
		return err
	}
	builderImage, _, err := img.Append(baseImage, filepath.Join(tmpDir, "buildpacks.tar"))
	if err != nil {
		return err
	}

	return builderStore.Write(builderImage)
}

type order struct {
	Groups []lifecycle.BuildpackGroup `toml:"groups"`
}

func (f *BuilderFactory) buildpackDir(dest string, builder *Builder) (string, error) {
	buildpackDir := filepath.Join(dest, "buildpack")
	err := os.Mkdir(buildpackDir, 0755)
	if err != nil {
		return "", err
	}
	for _, buildpack := range builder.Buildpacks {
		dir := strings.TrimPrefix(buildpack.URI, "file://")
		var data struct {
			BP struct {
				ID      string `toml:"id"`
				Version string `toml:"version"`
			} `toml:"buildpack"`
		}
		_, err := toml.DecodeFile(filepath.Join(dir, "buildpack.toml"), &data)
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
		err = recursiveCopy(dir, filepath.Join(buildpackDir, buildpack.ID, bp.Version))
		if err != nil {
			return "", err
		}
	}

	orderFile, err := os.Create(filepath.Join(buildpackDir, "order.toml"))
	if err != nil {
		return "", err
	}
	defer orderFile.Close()
	err = toml.NewEncoder(orderFile).Encode(order{Groups: builder.Groups})
	if err != nil {
		return "", err
	}
	return buildpackDir, nil
}

func recursiveCopy(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destFile := filepath.Join(dest, relPath)
		if info.IsDir() {
			err := os.MkdirAll(destFile, info.Mode())
			if err != nil {
				return err
			}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			os.Symlink(destFile, target)
		}
		if info.Mode().IsRegular() {
			s, err := os.Open(path)
			if err != nil {
				return err
			}
			defer s.Close()

			d, err := os.OpenFile(destFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, info.Mode())
			if err != nil {
				return err
			}
			defer d.Close()
			if _, err := io.Copy(d, s); err != nil {
				return err
			}
		}
		return nil
	})
}

// TODO share between here and exporter.
func createTarFile(tarFile, fsDir, tarDir string) error {
	fh, err := os.Create(tarFile)
	if err != nil {
		return fmt.Errorf("create file for tar: %s", err)
	}
	defer fh.Close()
	gzw := gzip.NewWriter(fh)
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	return filepath.Walk(fsDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode().IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(fsDir, file)
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
		header.Name = filepath.Join(tarDir, relPath)

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
