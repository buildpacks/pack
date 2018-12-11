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
	DefaultBuilder string  `toml:"default-builder"`
	configPath     string
}

type Stack struct {
	ID          string   `toml:"id"`
	BuildImages []string `toml:"build-images"`
	RunImages   []string `toml:"run-images"`
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

	if config.DefaultStackID == "" {
		config.DefaultStackID = "io.buildpacks.stacks.bionic"
	}
	if config.DefaultBuilder == "" || config.DefaultBuilder == "packs/samples" {
		config.DefaultBuilder = "packs/samples:v3alpha2"
	}
	upgradeStack(config, Stack{
		ID:          "io.buildpacks.stacks.bionic",
		BuildImages: []string{"packs/build:v3alpha2"},
		RunImages:   []string{"packs/run:v3alpha2"},
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

func upgradeStack(config *Config, stack Stack) {
	for _, stk := range config.Stacks {
		if stk.ID == stack.ID {
			for index, value := range stk.BuildImages {
				if value == "packs/build" {
					stk.BuildImages[index] = stack.BuildImages[0]
				}
			}

			for index, value := range stk.RunImages {
				if value == "packs/run" {
					stk.RunImages[index] = stack.RunImages[0]
				}
			}
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

func (c *Config) Update(stackID string, stack Stack) error {
	for i, stk := range c.Stacks {
		if stk.ID == stackID {
			if len(stack.BuildImages) > 0 {
				stk.BuildImages = stack.BuildImages
			}
			if len(stack.RunImages) > 0 {
				stk.RunImages = stack.RunImages
			}
			c.Stacks[i] = stk
			return c.save()
		}
	}
	return fmt.Errorf(`Missing stack: stack with id "%s" not found in pack config.toml`, stackID)
}

func (c *Config) Delete(stackID string) error {
	if c.DefaultStackID == stackID {
		return fmt.Errorf(`%s cannot be deleted when it is the default stack. You can change your default stack by running "pack set-default-stack".`, stackID)
	}
	for i, s := range c.Stacks {
		if s.ID == stackID {
			c.Stacks = append(c.Stacks[:i], c.Stacks[i+1:]...)
			return c.save()
		}
	}
	return fmt.Errorf(`"%s" does not exist. Please pass in a valid stack ID.`, stackID)
}
func (c *Config) SetDefaultStack(stackID string) error {
	for _, s := range c.Stacks {
		if s.ID == stackID {
			c.DefaultStackID = stackID
			return c.save()
		}
	}
	return fmt.Errorf(`"%s" does not exist. Please pass in a valid stack ID.`, stackID)
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
