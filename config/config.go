package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"
)

type Config struct {
	Stacks         []Stack `toml:"stacks"`
	DefaultStackID string  `toml:"default-stack-id"`
	configPath     string
}

type Stack struct {
	ID          string   `toml:"id"`
	BuildImages []string `toml:"build-images"`
	RunImages   []string `toml:"run-images"`
}

func New(path string) (*Config, error) {
	configPath := filepath.Join(path, "config.toml")
	config, err := previousConfig(path)
	if err != nil {
		return nil, err
	}

	if config.DefaultStackID == "" {
		config.DefaultStackID = "io.buildpacks.stacks.bionic"
	}
	appendStackIfMissing(config, Stack{
		ID:          "io.buildpacks.stacks.bionic",
		BuildImages: []string{"packs/build"},
		RunImages:   []string{"packs/run"},
	})

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
	w, err := os.OpenFile(c.configPath, os.O_CREATE|os.O_RDWR, 0644)
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

func appendStackIfMissing(config *Config, stack Stack) {
	for _, stk := range config.Stacks {
		if stk.ID == stack.ID {
			return
		}
	}
	config.Stacks = append(config.Stacks, stack)
}

func (c *Config) Get(stackID string) (*Stack, error) {
	if stackID == "" {
		stackID = c.DefaultStackID
	}
	for _, stack := range c.Stacks {
		if stack.ID == stackID {
			return &stack, nil
		}
	}
	return nil, fmt.Errorf(`Missing stack: stack with id "%s" not found in pack config.toml`, stackID)
}

func (c *Config) Add(stack Stack) error {
	if _, err := c.Get(stack.ID); err == nil {
		return fmt.Errorf(`stack "%s" already exists`, stack.ID)
	}
	c.Stacks = append(c.Stacks, stack)
	return c.save()
}

func ImageByRegistry(registry string, images []string) (string, error) {
	if len(images) == 0 {
		return "", errors.New("empty images")
	}
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
