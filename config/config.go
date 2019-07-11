package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

type Config struct {
	RunImages      []RunImage `toml:"run-images"`
	DefaultBuilder string     `toml:"default-builder-image,omitempty"`
}

type RunImage struct {
	Image   string   `toml:"image"`
	Mirrors []string `toml:"mirrors"`
}

func DefaultConfigPath() string {
	return filepath.Join(PackHome(), "config.toml")
}

func PackHome() string {
	packHome := os.Getenv("PACK_HOME")
	if packHome == "" {
		packHome = filepath.Join(os.Getenv("HOME"), ".pack")
	}
	return packHome
}

func Read(path string) (Config, error) {
	cfg := Config{}
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil && !os.IsNotExist(err) {
		return Config{}, errors.Wrapf(err, "failed to read config file at path %s", path)
	}

	return cfg, nil
}

func Write(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	w, err := os.Create(path)
	if err != nil {
		return err
	}
	defer w.Close()

	return toml.NewEncoder(w).Encode(cfg)
}

func SetRunImageMirrors(cfg Config, image string, mirrors []string) Config {
	for i := range cfg.RunImages {
		if cfg.RunImages[i].Image == image {
			cfg.RunImages[i].Mirrors = mirrors
			return cfg
		}
	}
	cfg.RunImages = append(cfg.RunImages, RunImage{Image: image, Mirrors: mirrors})
	return cfg
}
