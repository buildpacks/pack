package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"
)

type Config struct {
	RunImages      []RunImage `toml:"run-images"`
	DefaultBuilder string     `toml:"default-builder-image,omitempty"`
	configPath     string
}

type RunImage struct {
	Image   string   `toml:"image"`
	Mirrors []string `toml:"mirrors"`
}

func NewDefault() (*Config, error) {
	packHome := os.Getenv("PACK_HOME")
	if packHome == "" {
		packHome = filepath.Join(os.Getenv("HOME"), ".pack")
	}
	return New(packHome)
}

func New(path string) (*Config, error) {
	configPath := filepath.Join(path, "config.toml")
	config, err := previousConfig(path)
	if err != nil {
		return nil, err
	}

	config.configPath = configPath

	if err := config.save(); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) save() error {
	if err := os.MkdirAll(filepath.Dir(c.configPath), 0777); err != nil {
		return err
	}
	w, err := os.Create(c.configPath)
	if err != nil {
		return err
	}
	defer w.Close()

	return toml.NewEncoder(w).Encode(c)
}

func previousConfig(path string) (*Config, error) {
	configPath := filepath.Join(path, "config.toml")
	config := &Config{}
	_, err := toml.DecodeFile(configPath, config)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return config, nil
}

// Path returns the directory path where the config is stored as a toml file.
// That directory may also contain other `pack` related files.
func (c *Config) Path() string {
	return filepath.Dir(c.configPath)
}

func (c *Config) SetDefaultBuilder(builder string) error {
	c.DefaultBuilder = builder
	return c.save()
}

func (c *Config) GetRunImage(runImageTag string) *RunImage {
	for i := range c.RunImages {
		runImage := &c.RunImages[i]
		if runImage.Image == runImageTag {
			return runImage
		}
	}
	return nil
}

func (c *Config) SetRunImageMirrors(image string, mirrors []string) {
	if runImage := c.GetRunImage(image); runImage != nil {
		runImage.Mirrors = mirrors
	} else {
		c.RunImages = append(c.RunImages, RunImage{Image: image, Mirrors: mirrors})
	}
	c.save()
}

func ImageByRegistry(registry string, images []string) (string, error) {
	for _, i := range images {
		reg, err := Registry(i)
		if err != nil {
			continue
		}
		if registry == reg {
			return i, nil
		}
	}
	return images[0], nil
}

func Registry(imageName string) (string, error) {
	ref, err := name.ParseReference(imageName, name.WeakValidation)
	if err != nil {
		return "", err
	}
	return ref.Context().RegistryStr(), nil
}
