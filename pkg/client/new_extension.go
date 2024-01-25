package client

import (
	"context"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/dist"
)

var (
	bashBinGenerate = `#!/usr/bin/env bash
	
	set -eo pipefail
	
	# 1. GET ARGS
	output_dir=$CNB_OUTPUT_DIR
	
	# 2. GENERATE build.Dockerfile
	cat >> "${output_dir}/build.Dockerfile" <<EOL
	ARG base_image
	FROM \${base_image}
	
	RUN echo "Hello from build extension"
	EOL
	
	# 3. GENERATE run.Dockerfile
	cat >> "${output_dir}/run.Dockerfile" <<EOL
	ARG base_image
	FROM \${base_image}
	
	RUN echo "Hello from run extension"
	EOL
`
)

type NewExtensionOptions struct {
	// api compat version of the output extension artifact.
	API string

	// The base directory to generate assets
	Path string

	// The ID of the output extension artifact.
	ID string

	// version of the output extension artifact.
	Version string

	// the targets this buildpack will work with
	Targets []dist.Target
}

func (c *Client) NewExtension(ctx context.Context, opts NewExtensionOptions) error {
	err := createExtensionTOML(opts.Path, opts.ID, opts.Version, opts.API, opts.Targets, c)
	if err != nil {
		return err
	}
	return createBashExtension(opts.Path, c)
}

func createBashExtension(path string, c *Client) error {
	if err := createBinScript(path, "generate", bashBinGenerate, c); err != nil {
		return err
	}

	if err := createBinScript(path, "detect", bashBinDetect, c); err != nil {
		return err
	}

	return nil
}

func createExtensionTOML(path, id, version, apiStr string, targets []dist.Target, c *Client) error {
	api, err := api.NewVersion(apiStr)
	if err != nil {
		return err
	}

	extensionTOML := dist.ExtensionDescriptor{
		WithAPI:     api,
		WithTargets: targets,
		WithInfo: dist.ModuleInfo{
			ID:      id,
			Version: version,
		},
	}

	// The following line's comment is for gosec, it will ignore rule 301 in this case
	// G301: Expect directory permissions to be 0750 or less
	/* #nosec G301 */
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	extensionTOMLPath := filepath.Join(path, "extension.toml")
	_, err = os.Stat(extensionTOMLPath)
	if os.IsNotExist(err) {
		f, err := os.Create(extensionTOMLPath)
		if err != nil {
			return err
		}
		if err := toml.NewEncoder(f).Encode(extensionTOML); err != nil {
			return err
		}
		defer f.Close()
		if c != nil {
			c.logger.Infof("    %s  extension.toml", style.Symbol("create"))
		}
	}

	return nil
}
