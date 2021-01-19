package pack

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
)

var (
	BuildpackLanguages = map[string]interface{}{
		"bash":   createBashBuildpack,
		"go":     createGolangBuildpack,
		"golang": createGolangBuildpack,
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

	// Defines the Buildpacks configuration.
	Config pubbldpkg.Config
}

func (c *Client) CreateBuildpack(ctx context.Context, opts CreateBuildpackOptions) error {
	if opts.Config.Platform.OS == "windows" && !c.experimental {
		return NewExperimentError("Windows buildpack create support is currently experimental.")
	}

	buildpackTOML := dist.BuildpackDescriptor{
		API:    build.SupportedPlatformAPIVersions.Latest(),
		Stacks: opts.Stacks,
		Info: dist.BuildpackInfo{
			ID:      opts.ID,
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
	c.logger.Infof("    %s  buildpack.toml", style.Key("create"))

	if err := os.MkdirAll(filepath.Join(opts.Path, "bin"), 0755); err != nil {
		return err
	}

	createFunc := BuildpackLanguages[opts.Language]
	if createFunc == nil {
		return errors.Wrapf(err, "Unsupported language: %s", opts.Language)
	}

	return createFunc.(func(string, pubbldpkg.Config, *Client) error)(opts.Path, opts.Config, c)
}

func createBashBuildpack(path string, config pubbldpkg.Config, c *Client) error {
	if err := createBinScript(path, "build", bashBinBuild); err != nil {
		return err
	}
	c.logger.Infof("    %s  bin/build", style.Key("create"))

	if err := createBinScript(path, "detect", bashBinDetect); err != nil {
		return err
	}
	c.logger.Infof("    %s  bin/build", style.Key("create"))

	return nil
}

func createGolangBuildpack(path string, config pubbldpkg.Config, c *Client) error {
	// TODO
	// include libbuildpack dependency
	return nil
}

func createBinScript(path, name, contents string) error {
	bin := filepath.Join(path, "bin", name)
	f, err := os.Create(bin)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.WriteString(contents); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		if err = os.Chmod(bin, 0755); err != nil {
			return err
		}
	}
	return nil
}
