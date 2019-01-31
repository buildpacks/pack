package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildpack/pack/style"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"
)

type Config struct {
	Stacks         []Stack    `toml:"stacks"`
	RunImages      []RunImage `toml:"run-images"`
	DefaultStackID string     `toml:"default-stack-id"`
	DefaultBuilder string     `toml:"default-builder"`
	configPath     string
}

type Stack struct {
	ID          string   `toml:"id"`
	BuildImage  string   `toml:"build-image"`
	BuildImages []string `toml:"build-images,omitempty"` // Deprecated
	RunImages   []string `toml:"run-images"`
}

type RunImage struct {
	Image   string   `toml:"tag"`
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

	config.migrate()

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

func (c *Config) migrate() {
	if c.DefaultStackID == "" {
		c.DefaultStackID = "io.buildpacks.stacks.bionic"
	}
	if c.DefaultBuilder == "" || c.DefaultBuilder == "packs/samples" {
		c.DefaultBuilder = "packs/samples:v3alpha2"
	}

	initialStack := Stack{
		ID:         "io.buildpacks.stacks.bionic",
		BuildImage: "packs/build:v3alpha2",
		RunImages:  []string{"packs/run:v3alpha2"},
	}

	c.migrateBuildImagesToSingularBuildImage()

	s, err := c.GetStack(initialStack.ID)
	if err == nil {
		migrateInitialImages(s, initialStack.BuildImage, initialStack.RunImages[0])
	} else {
		c.Stacks = append(c.Stacks, initialStack)
	}
}

// TODO: Eventually remove this, once most users are likely migrated
func (c *Config) migrateBuildImagesToSingularBuildImage() {
	for s := range c.Stacks {
		stack := &c.Stacks[s]
		if stack.BuildImage == "" && len(stack.BuildImages) > 0 {
			stack.BuildImage = stack.BuildImages[0]
		}
		stack.BuildImages = nil
	}
}

func migrateInitialImages(stack *Stack, buildImage, runImage string) {
	if stack.BuildImage == "packs/build" {
		stack.BuildImage = buildImage
	}

	for index, value := range stack.RunImages {
		if value == "packs/run" {
			stack.RunImages[index] = runImage
		}
	}
}

func (c *Config) GetStack(stackID string) (*Stack, error) {
	if stackID == "" {
		stackID = c.DefaultStackID
	}
	for s := range c.Stacks {
		stack := &c.Stacks[s]
		if stack.ID == stackID {
			return stack, nil
		}
	}
	return nil, missingStackError(stackID)
}

func (c *Config) AddStack(stack Stack) error {
	if _, err := c.GetStack(stack.ID); err == nil {
		return fmt.Errorf("stack %s already exists", style.Symbol(stack.ID))
	}
	c.Stacks = append(c.Stacks, stack)
	return c.save()
}

func (c *Config) UpdateStack(stackID string, newStack Stack) error {
	stk, err := c.GetStack(stackID)
	if err != nil {
		return err
	}

	if newStack.BuildImage == "" && len(newStack.RunImages) == 0 {
		return errors.New("no build image or run image(s) specified")
	}

	if newStack.BuildImage != "" {
		stk.BuildImage = newStack.BuildImage
	}
	if len(newStack.RunImages) > 0 {
		stk.RunImages = newStack.RunImages
	}
	return c.save()
}

func (c *Config) DeleteStack(stackID string) error {
	if c.DefaultStackID == stackID {
		return fmt.Errorf(`%s cannot be deleted when it is the default stack. You can change your default stack by running "pack set-default-stack".`, stackID)
	}
	for i, s := range c.Stacks {
		if s.ID == stackID {
			c.Stacks = append(c.Stacks[:i], c.Stacks[i+1:]...)
			return c.save()
		}
	}
	return missingStackError(stackID)
}

func (c *Config) SetDefaultStack(stackID string) error {
	_, err := c.GetStack(stackID)
	if err != nil {
		return err
	}

	c.DefaultStackID = stackID
	return c.save()
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

func missingStackError(stackID string) error {
	return fmt.Errorf(`stack %s does not exist`, style.Symbol(stackID))
}
