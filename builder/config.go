package builder

import (
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/paths"
)

type Config struct {
	Description string            `toml:"description"`
	Buildpacks  []BuildpackConfig `toml:"buildpacks"`
	Order       OrderConfig       `toml:"order"`
	Stack       StackConfig       `toml:"stack"`
	Lifecycle   LifecycleConfig   `toml:"lifecycle"`
}

type OrderConfig []GroupConfig

func (o OrderConfig) ToMetadata() OrderMetadata {
	var order OrderMetadata
	for _, gp := range o {
		var buildpacks []BuildpackRefMetadata
		for _, bp := range gp.Group {
			buildpacks = append(buildpacks, BuildpackRefMetadata{
				ID:       bp.ID,
				Version:  bp.Version,
				Optional: bp.Optional,
			})
		}

		order = append(order, GroupMetadata{
			Buildpacks: buildpacks,
		})
	}

	return order
}

type GroupConfig struct {
	Group []BuildpackRefConfig `toml:"group"`
}

type BuildpackRefConfig struct {
	ID       string `toml:"id"`
	Version  string `toml:"version"`
	Optional bool   `toml:"optional,omitempty"`
}

type BuildpackConfig struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
	URI     string `toml:"uri"`
}

type StackConfig struct {
	ID              string   `toml:"id"`
	BuildImage      string   `toml:"build-image"`
	RunImage        string   `toml:"run-image"`
	RunImageMirrors []string `toml:"run-image-mirrors,omitempty"`
}

type LifecycleConfig struct {
	URI     string `toml:"uri"`
	Version string `toml:"version"`
}

// ReadConfig reads a builder configuration from the file path provided
func ReadConfig(path string) (Config, error) {
	builderDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return Config{}, err
	}

	file, err := os.Open(path)
	if err != nil {
		return Config{}, errors.Wrap(err, "opening config file")
	}
	defer file.Close()

	config, err := parseConfig(file, builderDir)
	if err != nil {
		return Config{}, errors.Wrapf(err, "parse contents of '%s'", path)
	}

	return config, nil
}

// parseConfig reads a builder configuration from reader and resolves relative buildpack paths using `relativeToDir`
func parseConfig(reader io.Reader, relativeToDir string) (Config, error) {
	builderConfig := Config{}
	if _, err := toml.DecodeReader(reader, &builderConfig); err != nil {
		return builderConfig, errors.Wrap(err, "decoding toml contents")
	}

	for i, bp := range builderConfig.Buildpacks {
		uri, err := transformRelativePath(bp.URI, relativeToDir)
		if err != nil {
			return Config{}, errors.Wrap(err, "transforming buildpack URI")
		}
		builderConfig.Buildpacks[i].URI = uri
	}

	if builderConfig.Lifecycle.URI != "" {
		uri, err := transformRelativePath(builderConfig.Lifecycle.URI, relativeToDir)
		if err != nil {
			return Config{}, errors.Wrap(err, "transforming lifecycle URI")
		}
		builderConfig.Lifecycle.URI = uri
	}

	return builderConfig, nil
}

func transformRelativePath(uri, relativeTo string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	if parsed.Scheme == "" {
		if !filepath.IsAbs(parsed.Path) {
			absPath := filepath.Join(relativeTo, parsed.Path)
			return paths.FilePathToUri(absPath)
		}
	}

	return uri, nil
}
