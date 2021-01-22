package pack

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
)

var (
	bashBinBuild = `
#!/usr/bin/env bash

set -euo pipefail

layers_dir="$1"
env_dir="$2/env"
plan_path="$3"

exit 0
`
	bashBinDetect = `
#!/usr/bin/env bash

exit 0
`
)

type NewBuildpackOptions struct {
	// The base directory to generate assets
	Path string

	// The ID of the output buildpack artifact.
	ID string

	// The stacks this buildpack will work with
	Stacks []dist.Stack
}

func (c *Client) NewBuildpack(ctx context.Context, opts NewBuildpackOptions) error {
	buildpackTOML := dist.BuildpackDescriptor{
		API:    build.SupportedPlatformAPIVersions.Latest(),
		Stacks: opts.Stacks,
		Info: dist.BuildpackInfo{
			ID:      opts.ID,
			Version: "0.0.0",
		},
	}

	buildpackTOMLPath := filepath.Join(opts.Path, "buildpack.toml")
	_, err := os.Stat(buildpackTOMLPath)
	if os.IsNotExist(err) {
		f, err := os.Create(buildpackTOMLPath)
		if err != nil {
			return err
		}
		if err := toml.NewEncoder(f).Encode(buildpackTOML); err != nil {
			return err
		}
		defer f.Close()
		c.logger.Infof("    %s  buildpack.toml", style.Key("create"))
	}

	return createBashBuildpack(opts.Path, c)
}

func createBashBuildpack(path string, c *Client) error {
	if err := createBinScript(path, "build", bashBinBuild, c); err != nil {
		return err
	}

	if err := createBinScript(path, "detect", bashBinDetect, c); err != nil {
		return err
	}

	return nil
}

func createBinScript(path, name, contents string, c *Client) error {
	binDir := filepath.Join(path, "bin")
	binFile := filepath.Join(binDir, name)

	_, err := os.Stat(binFile)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(binDir, 0755); err != nil {
			return err
		}

		err = ioutil.WriteFile(binFile, []byte(contents), 0755)
		if err != nil {
			return err
		}

		c.logger.Infof("    %s  bin/%s", style.Key("create"), name)
	}
	return nil
}
