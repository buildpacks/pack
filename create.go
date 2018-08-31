package pack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/buildpack/packs/img"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
)

type Create struct {
	BPDir       string
	BaseImage   string
	DetectImage string
	BuildImage  string
	Publish     bool
}

func (c *Create) Run() error {
	useDaemon := !c.Publish
	tmpDir, err := ioutil.TempDir("", "pack.create.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	baseImage, err := readImage(c.BaseImage, useDaemon)
	if err != nil {
		return err
	}
	if baseImage == nil {
		return fmt.Errorf("base-image not found: %s", c.BaseImage)
	}

	if err := createTarFile(filepath.Join(tmpDir, "buildpacks.tar"), c.BPDir, "/buildpacks"); err != nil {
		return err
	}
	newImage, _, err := img.Append(baseImage, filepath.Join(tmpDir, "buildpacks.tar"))
	if err != nil {
		return err
	}

	configFile, err := newImage.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	config.Cmd = []string{}
	config.User = "packs"
	config.Entrypoint = []string{"/packs/detector"}
	config.Env = append(
		config.Env,
		"PACK_BP_PATH=/buildpacks",
		"PACK_BP_ORDER_PATH=/buildpacks/order.toml",
		"PACK_BP_GROUP_PATH=./group.toml",
		"PACK_DETECT_INFO_PATH=./detect.toml",
		"PACK_STACK_NAME=",
	)
	newImage, err = mutate.Config(newImage, config)
	if err != nil {
		return err
	}

	detectStore, err := repoStore(c.DetectImage, useDaemon)
	if err != nil {
		return err
	}
	if err := detectStore.Write(newImage); err != nil {
		return err
	}

	config.Entrypoint = []string{"/packs/builder"}
	config.Env = append(
		config.Env,
		"PACK_METADATA_PATH=/launch/config/metadata.toml",
	)
	newImage, err = mutate.Config(newImage, config)
	if err != nil {
		return err
	}

	buildStore, err := repoStore(c.BuildImage, useDaemon)
	if err != nil {
		return err
	}
	if err := buildStore.Write(newImage); err != nil {
		return err
	}

	return nil
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
