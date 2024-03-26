package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/dist"
)

// Config is a builder configuration file
type Config struct {
	Description     string           `toml:"description"`
	Buildpacks      ModuleCollection `toml:"buildpacks"`
	Extensions      ModuleCollection `toml:"extensions"`
	Order           dist.Order       `toml:"order"`
	OrderExtensions dist.Order       `toml:"order-extensions"`
	Stack           StackConfig      `toml:"stack"`
	Lifecycle       LifecycleConfig  `toml:"lifecycle"`
	Run             RunConfig        `toml:"run"`
	Build           BuildConfig      `toml:"build"`
	WithTargets     []dist.Target    `toml:"targets,omitempty"`
}

type MultiArchConfig struct {
	Config
	flagTargets     []dist.Target
	relativeBaseDir string
}

// ModuleCollection is a list of ModuleConfigs
type ModuleCollection []ModuleConfig

// ModuleConfig details the configuration of a Buildpack or Extension
type ModuleConfig struct {
	dist.ModuleInfo
	dist.ImageOrURI
}

func (c *ModuleConfig) DisplayString() string {
	if c.ModuleInfo.FullName() != "" {
		return c.ModuleInfo.FullName()
	}

	return c.ImageOrURI.DisplayString()
}

// StackConfig details the configuration of a Stack
type StackConfig struct {
	ID              string   `toml:"id"`
	BuildImage      string   `toml:"build-image"`
	RunImage        string   `toml:"run-image"`
	RunImageMirrors []string `toml:"run-image-mirrors,omitempty"`
}

// LifecycleConfig details the configuration of the Lifecycle
type LifecycleConfig struct {
	URI     string `toml:"uri"`
	Version string `toml:"version"`
}

// RunConfig set of run image configuration
type RunConfig struct {
	Images []RunImageConfig `toml:"images"`
}

// RunImageConfig run image id and mirrors
type RunImageConfig struct {
	Image   string   `toml:"image"`
	Mirrors []string `toml:"mirrors,omitempty"`
}

// BuildConfig build image configuration
type BuildConfig struct {
	Image string           `toml:"image"`
	Env   []BuildConfigEnv `toml:"env"`
}

type Suffix string

const (
	NONE     Suffix = ""
	DEFAULT  Suffix = "default"
	OVERRIDE Suffix = "override"
	APPEND   Suffix = "append"
	PREPEND  Suffix = "prepend"
)

type BuildConfigEnv struct {
	Name   string `toml:"name"`
	Value  string `toml:"value"`
	Suffix Suffix `toml:"suffix,omitempty"`
	Delim  string `toml:"delim,omitempty"`
}

// ReadConfig reads a builder configuration from the file path provided and returns the
// configuration along with any warnings encountered while parsing
func ReadConfig(path string) (config Config, warnings []string, err error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return Config{}, nil, errors.Wrap(err, "opening config file")
	}
	defer file.Close()

	config, err = parseConfig(file)
	if err != nil {
		return Config{}, nil, errors.Wrapf(err, "parse contents of '%s'", path)
	}

	if len(config.Order) == 0 {
		warnings = append(warnings, fmt.Sprintf("empty %s definition", style.Symbol("order")))
	}

	config.mergeStackWithImages()

	return config, warnings, nil
}

func (c *MultiArchConfig) Targets() []dist.Target {
	if len(c.flagTargets) != 0 {
		return c.flagTargets
	}

	return c.WithTargets
}

func ReadMultiArchConfig(path string, flagTargets []dist.Target) (config MultiArchConfig, warnings []string, err error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return MultiArchConfig{}, nil, errors.Wrap(err, "opening config file")
	}
	defer file.Close()

	config, err = parseMultiArchConfig(file)
	if err != nil {
		return MultiArchConfig{}, nil, errors.Wrapf(err, "parse contents of '%s'", path)
	}

	if len(config.Order) == 0 {
		warnings = append(warnings, fmt.Sprintf("empty %s definition", style.Symbol("order")))
	}

	config.mergeStackWithImages()
	config.flagTargets = flagTargets
	return config, warnings, nil
}

func (c *MultiArchConfig) BuilderConfigs(getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (configs []Config, err error) {
	targets := c.Targets()
	for _, target := range targets {
		if len(target.Distributions) == 0 {
			configCopy := *c
			cfg, err := configCopy.processTarget(target, dist.Distribution{}, "", getIndexManifest)
			if err != nil {
				return configs, err
			}
			configs = append(configs, cfg)
		} else {
			for _, distro := range target.Distributions {
				if len(distro.Versions) == 0 {
					configCopy := *c
					cfg, err := configCopy.processTarget(target, distro, "", getIndexManifest)
					if err != nil {
						return configs, err
					}
					configs = append(configs, cfg)
				} else {
					for _, version := range distro.Versions {
						configCopy := *c
						cfg, err := configCopy.processTarget(target, distro, version, getIndexManifest)
						if err != nil {
							return configs, err
						}
						configs = append(configs, cfg)
					}
				}
			}
		}
	}

	return configs, nil
}

func (c MultiArchConfig) processTarget(target dist.Target, distro dist.Distribution, version string, getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (config Config, err error) {
	config = Config{
		Buildpacks: make(ModuleCollection, len(c.Config.Buildpacks)),
		Extensions: make(ModuleCollection, len(c.Config.Extensions)),
		Order: make(dist.Order, len(c.Config.Order)),
		OrderExtensions: make(dist.Order, len(c.Config.OrderExtensions)),
		WithTargets: make([]dist.Target, len(c.Config.WithTargets)),
		Run: RunConfig{
			Images: make([]RunImageConfig, len(c.Config.Run.Images)),
		},
		Stack: StackConfig{
			RunImageMirrors: make([]string, len(c.Config.Stack.RunImageMirrors)),
		},
		Build: BuildConfig{
			Env: make([]BuildConfigEnv, len(c.Config.Build.Env)),
		},
	}
	processedTarget := buildpackage.ProcessTarget(target, distro, version)
	for i, bp := range c.Config.Buildpacks {
		if bp.URI != "" {
			//TODO: Delete below line
			fmt.Printf("MultiArchConfig processTarget BP from %s to %s \n", bp.URI, fmt.Sprintf("%s%s%s", style.Symbol(target.OS), "/" + style.Symbol(target.Arch), "/" + style.Symbol(target.ArchVariant)))
			if config.Buildpacks[i].URI, err = buildpackage.GetRelativeURI(bp.URI, c.relativeBaseDir, &processedTarget, getIndexManifest); err != nil {
				return config, err
			}
		}
	}

	for i, ext := range c.Config.Extensions {
		if ext.URI != "" {
			//TODO: Delete below line
			fmt.Printf("MultiArchConfig processTarget Ext from %s to %s \n", ext.URI, fmt.Sprintf("%s%s%s", style.Symbol(target.OS), "/" + style.Symbol(target.Arch), "/" + style.Symbol(target.ArchVariant)))
			if config.Extensions[i].URI, err = buildpackage.GetRelativeURI(ext.URI, c.relativeBaseDir, &processedTarget, getIndexManifest); err != nil {
				return config, err
			}
		}
	}

	if img := c.Config.Build.Image; img != "" {
		//TODO: Delete below line
		fmt.Printf("MultiArchConfig processTarget Build.Image from %s to %s \n", img, fmt.Sprintf("%s%s%s", style.Symbol(target.OS), "/" + style.Symbol(target.Arch), "/" + style.Symbol(target.ArchVariant)))
		if config.Build.Image, err = buildpackage.ParseURItoString(img, processedTarget, getIndexManifest); err != nil {
			return config, err
		}
	}

	for i, runImg := range c.Config.Run.Images {
		//TODO: Delete below line
		fmt.Printf("MultiArchConfig processTarget Run.Images[%d] from %s to %s \n", i, runImg, fmt.Sprintf("%s%s%s", style.Symbol(target.OS), "/" + style.Symbol(target.Arch), "/" + style.Symbol(target.ArchVariant)))
		config.Run.Images[i].Image, err = buildpackage.ParseURItoString(runImg.Image, processedTarget, getIndexManifest)
		if err != nil {
			for j, mirror := range runImg.Mirrors {
				//TODO: Delete below line
				fmt.Printf("MultiArchConfig processTarget Run.Images[%d].Mirrors[%d] from %s to %s \n",i, j, mirror, fmt.Sprintf("%s%s%s", style.Symbol(target.OS), "/" + style.Symbol(target.Arch), "/" + style.Symbol(target.ArchVariant)))
				if config.Run.Images[i].Mirrors[j], err = buildpackage.ParseURItoString(mirror, processedTarget, getIndexManifest); err == nil {
					break
				}
			}

			if err != nil {
				return config, err
			}
		}
	}

	if img := c.Config.Stack.BuildImage; img != "" {
		//TODO: Delete below line
		fmt.Printf("MultiArchConfig processTarget stack.BuildImage from %s to %s \n", img, fmt.Sprintf("%s%s%s", style.Symbol(target.OS), "/" + style.Symbol(target.Arch), "/" + style.Symbol(target.ArchVariant)))
		if config.Stack.BuildImage, err = buildpackage.ParseURItoString(img, processedTarget, getIndexManifest); err != nil {
			return config, err
		}
	}

	if img := c.Config.Stack.RunImage; img != "" {
		//TODO: Delete below line
		fmt.Printf("MultiArchConfig processTarget stacks.RunImage from %s to %s \n", img, fmt.Sprintf("%s%s%s", style.Symbol(target.OS), "/" + style.Symbol(target.Arch), "/" + style.Symbol(target.ArchVariant)))
		if config.Stack.RunImage, err = buildpackage.ParseURItoString(img, processedTarget, getIndexManifest); err != nil {
			for i, mirror := range config.Stack.RunImageMirrors {
				//TODO: Delete below line
				fmt.Printf("MultiArchConfig processTarget stacks.RunImage Mirror at %d from %s to %s \n", i, mirror, fmt.Sprintf("%s%s%s", style.Symbol(target.OS), "/" + style.Symbol(target.Arch), "/" + style.Symbol(target.ArchVariant)))
				if config.Stack.RunImageMirrors[i], err = buildpackage.ParseURItoString(mirror, processedTarget, getIndexManifest); err == nil {
					break
				}
			}

			if err != nil {
				return config, err
			}
		}
	}

	config.Order = c.Config.Order
	config.OrderExtensions = c.Config.OrderExtensions
	config.WithTargets = []dist.Target{processedTarget}
	return config, nil
}

func (c *MultiArchConfig) MultiArch() bool {
	targets := c.Targets()
	if len(targets) > 1 {
		return true
	}

	for _, target := range targets {
		if len(target.Distributions) > 1 {
			return true
		}

		for _, distro := range target.Distributions {
			if len(distro.Versions) > 1 {
				return true
			}
		}
	}

	return false
}

// ValidateConfig validates the config
func ValidateConfig(c Config) error {
	if c.Build.Image == "" && c.Stack.BuildImage == "" {
		return errors.New("build.image is required")
	} else if c.Build.Image != "" && c.Stack.BuildImage != "" && c.Build.Image != c.Stack.BuildImage {
		return errors.New("build.image and stack.build-image do not match")
	}

	if len(c.Run.Images) == 0 && (c.Stack.RunImage == "" || c.Stack.ID == "") {
		return errors.New("run.images are required")
	}

	for _, runImage := range c.Run.Images {
		if runImage.Image == "" {
			return errors.New("run.images.image is required")
		}
	}

	if c.Stack.RunImage != "" && c.Run.Images[0].Image != c.Stack.RunImage {
		return errors.New("run.images and stack.run-image do not match")
	}

	return nil
}

func (c *Config) mergeStackWithImages() {
	// RFC-0096
	if c.Build.Image != "" {
		c.Stack.BuildImage = c.Build.Image
	} else if c.Build.Image == "" && c.Stack.BuildImage != "" {
		c.Build.Image = c.Stack.BuildImage
	}

	if len(c.Run.Images) != 0 {
		// use the first run image as the "stack"
		c.Stack.RunImage = c.Run.Images[0].Image
		c.Stack.RunImageMirrors = c.Run.Images[0].Mirrors
	} else if len(c.Run.Images) == 0 && c.Stack.RunImage != "" {
		c.Run.Images = []RunImageConfig{{
			Image:   c.Stack.RunImage,
			Mirrors: c.Stack.RunImageMirrors,
		},
		}
	}
}

// parseConfig reads a builder configuration from file
func parseConfig(file *os.File) (Config, error) {
	builderConfig := Config{}
	tomlMetadata, err := toml.NewDecoder(file).Decode(&builderConfig)
	if err != nil {
		return Config{}, errors.Wrap(err, "decoding toml contents")
	}

	undecodedKeys := tomlMetadata.Undecoded()
	if len(undecodedKeys) > 0 {
		unknownElementsMsg := config.FormatUndecodedKeys(undecodedKeys)

		return Config{}, errors.Errorf("%s in %s",
			unknownElementsMsg,
			style.Symbol(file.Name()),
		)
	}

	return builderConfig, nil
}

// parseMultiArchConfig reads a builder configuration from file
func parseMultiArchConfig(file *os.File) (MultiArchConfig, error) {
	multiArchBuilderConfig := MultiArchConfig{}
	tomlMetadata, err := toml.NewDecoder(file).Decode(&multiArchBuilderConfig)
	if err != nil {
		return MultiArchConfig{}, errors.Wrap(err, "decoding MultiArchBuilder Toml")
	}

	undecodedKeys := tomlMetadata.Undecoded()
	if len(undecodedKeys) > 0 {
		unknownElementsMsg := config.FormatUndecodedKeys(undecodedKeys)

		return MultiArchConfig{}, errors.Errorf("%s in %s",
			unknownElementsMsg,
			style.Symbol(file.Name()),
		)
	}

	return multiArchBuilderConfig, nil
}

func ParseBuildConfigEnv(env []BuildConfigEnv, path string) (envMap map[string]string, warnings []string, err error) {
	envMap = map[string]string{}
	var appendOrPrependWithoutDelim = 0
	for _, v := range env {
		if name := v.Name; name == "" {
			return nil, nil, errors.Wrapf(errors.Errorf("env name should not be empty"), "parse contents of '%s'", path)
		}
		if val := v.Value; val == "" {
			warnings = append(warnings, fmt.Sprintf("empty value for key/name %s", style.Symbol(v.Name)))
		}
		suffixName, delimName, err := getBuildConfigEnvFileName(v)
		if err != nil {
			return envMap, warnings, err
		}
		if val, ok := envMap[suffixName]; ok {
			warnings = append(warnings, fmt.Sprintf(errors.Errorf("overriding env with name: %s and suffix: %s from %s to %s", style.Symbol(v.Name), style.Symbol(string(v.Suffix)), style.Symbol(val), style.Symbol(v.Value)).Error(), "parse contents of '%s'", path))
		}
		if val, ok := envMap[delimName]; ok {
			warnings = append(warnings, fmt.Sprintf(errors.Errorf("overriding env with name: %s and delim: %s from %s to %s", style.Symbol(v.Name), style.Symbol(v.Delim), style.Symbol(val), style.Symbol(v.Value)).Error(), "parse contents of '%s'", path))
		}
		if delim := v.Delim; delim != "" && delimName != "" {
			envMap[delimName] = delim
		}
		envMap[suffixName] = v.Value
	}

	for k := range envMap {
		name, suffix, err := getFilePrefixSuffix(k)
		if err != nil {
			continue
		}
		if _, ok := envMap[name+".delim"]; (suffix == "append" || suffix == "prepend") && !ok {
			warnings = append(warnings, fmt.Sprintf(errors.Errorf("env with name/key %s with suffix %s must to have a %s value", style.Symbol(name), style.Symbol(suffix), style.Symbol("delim")).Error(), "parse contents of '%s'", path))
			appendOrPrependWithoutDelim++
		}
	}
	if appendOrPrependWithoutDelim > 0 {
		return envMap, warnings, errors.Errorf("error parsing [[build.env]] in file '%s'", path)
	}
	return envMap, warnings, err
}

func getBuildConfigEnvFileName(env BuildConfigEnv) (suffixName, delimName string, err error) {
	suffix, err := getActionType(env.Suffix)
	if err != nil {
		return suffixName, delimName, err
	}
	if suffix == "" {
		suffixName = env.Name
	} else {
		suffixName = env.Name + suffix
	}
	if delim := env.Delim; delim != "" {
		delimName = env.Name + ".delim"
	}
	return suffixName, delimName, err
}

func getActionType(suffix Suffix) (suffixString string, err error) {
	const delim = "."
	switch suffix {
	case NONE:
		return "", nil
	case DEFAULT:
		return delim + string(DEFAULT), nil
	case OVERRIDE:
		return delim + string(OVERRIDE), nil
	case APPEND:
		return delim + string(APPEND), nil
	case PREPEND:
		return delim + string(PREPEND), nil
	default:
		return suffixString, errors.Errorf("unknown action type %s", style.Symbol(string(suffix)))
	}
}

func getFilePrefixSuffix(filename string) (prefix, suffix string, err error) {
	val := strings.Split(filename, ".")
	if len(val) <= 1 {
		return val[0], suffix, errors.Errorf("Suffix might be null")
	}
	if len(val) == 2 {
		suffix = val[1]
	} else {
		suffix = strings.Join(val[1:], ".")
	}
	return val[0], suffix, err
}
