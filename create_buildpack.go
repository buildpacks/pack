package pack

import (
	"context"
	"github.com/BurntSushi/toml"
	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)

var (
	BuildpackLanguages = map[string]interface{}{
		"bash": createBashBuildpack,
		"go": createGolangBuildpack,
		"golang": createGolangBuildpack,
		"python": createPythonBuildpack,
		"py": createPythonBuildpack,
	}

	bashBinBuild = `
#!/usr/bin/env bash

set -eu

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

type CreateBuildpackOptions struct {
	// The base directory to generate assets
	Path string

	// The ID of the output buildpack artifact.
	ID string

	// The language to generate scaffolding for
	Language string

	// The stacks this buildpack will work with
	Stacks []dist.Stack
}

func (c *Client) CreateBuildpack(ctx context.Context, opts CreateBuildpackOptions) error {
	buildpackTOML := dist.BuildpackDescriptor{
		API: build.SupportedPlatformAPIVersions.Latest(),
		Stacks: opts.Stacks,
		Info: dist.BuildpackInfo{
			ID: opts.ID,
			Version: "0.0.0",
		},
	}

	f, err := os.Create(filepath.Join(opts.Path, "buildpack.toml"))
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(buildpackTOML); err != nil {
		return err
	}
	defer f.Close()

	if err := os.MkdirAll(filepath.Join(opts.Path, "bin"), 0755); err != nil {
		return err
	}

	createFunc := BuildpackLanguages[opts.Language]
	if createFunc == nil {
		return errors.Wrapf(err, "Unsupported language: %s", opts.Language)
	}

	return createFunc.(func(string) error)(opts.Path)
}

func createBashBuildpack(path string) error {
	bin := filepath.Join(path, "bin", "detect")
	f, err := os.Create(bin)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.WriteString(bashBinDetect); err != nil {
		return err
	}
	if err = os.Chmod(bin, 755); err != nil {
		return err
	}

	bin = filepath.Join(path, "bin", "build")
	f, err = os.Create(filepath.Join(path, "bin", "build"))
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.WriteString(bashBinBuild); err != nil {
		return err
	}

	return os.Chmod(bin, 755)
}

func createGolangBuildpack(path string) error {
	// TODO
	// include libbuildpack dependency
	return nil
}

func createPythonBuildpack(path string) error {
	return nil
}
