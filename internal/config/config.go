package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
)

type Config struct {
	// Deprecated: Use DefaultRegistryName instead. See https://github.com/buildpacks/pack/issues/747.
	DefaultRegistry     string            `toml:"default-registry-url,omitempty"`
	DefaultRegistryName string            `toml:"default-registry,omitempty"`
	DefaultBuilder      string            `toml:"default-builder-image,omitempty"`
	PullPolicy          string            `toml:"pull-policy,omitempty"`
	Experimental        bool              `toml:"experimental,omitempty"`
	RunImages           []RunImage        `toml:"run-images"`
	TrustedBuilders     []TrustedBuilder  `toml:"trusted-builders,omitempty"`
	Registries          []Registry        `toml:"registries,omitempty"`
	LifecycleImage      string            `toml:"lifecycle-image,omitempty"`
	RegistryMirrors     map[string]string `toml:"registry-mirrors,omitempty"`
	LayoutRepositoryDir string            `toml:"layout-repo-dir,omitempty"`
}

type VolumeConfig struct {
	VolumeKeys map[string]string `toml:"volume-keys,omitempty"`
}

type Registry struct {
	Name string `toml:"name"`
	Type string `toml:"type"`
	URL  string `toml:"url"`
}

type RunImage struct {
	Image   string   `toml:"image"`
	Mirrors []string `toml:"mirrors"`
}

type TrustedBuilder struct {
	Name string `toml:"name"`
}

const OfficialRegistryName = "official"

func DefaultRegistry() Registry {
	return Registry{
		OfficialRegistryName,
		"github",
		"https://github.com/buildpacks/registry-index",
	}
}

func DefaultConfigPath() (string, error) {
	home, err := PackHome()
	if err != nil {
		return "", errors.Wrap(err, "getting pack home")
	}
	return filepath.Join(home, "config.toml"), nil
}

func DefaultVolumeKeysPath() (string, error) {
	home, err := PackHome()
	if err != nil {
		return "", errors.Wrap(err, "getting pack home")
	}
	return filepath.Join(home, "volume-keys.toml"), nil
}

func PackHome() (string, error) {
	packHome := os.Getenv("PACK_HOME")
	if packHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.Wrap(err, "getting user home")
		}
		packHome = filepath.Join(home, ".pack")
	}
	return packHome, nil
}

func Read(path string) (Config, error) {
	cfg := Config{}
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil && !os.IsNotExist(err) {
		return Config{}, errors.Wrapf(err, "failed to read config file at path %s", path)
	}
	return cfg, nil
}

func ReadVolumeKeys(path string) (VolumeConfig, error) {
	cfg := VolumeConfig{}
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil && !os.IsNotExist(err) {
		return VolumeConfig{}, errors.Wrapf(err, "failed to read config file at path %s", path)
	}
	return cfg, nil
}

// Write atomically replaces the file at path with cfg encoded as TOML.
//
// The config is encoded into a temporary file in the same directory which is
// then renamed over path. Truncating and rewriting path in place is not safe
// when several pack processes write it concurrently (for example, parallel
// `pack build` runs updating volume-keys.toml): readers could observe a
// partially written file and writers could interleave, leaving a corrupted
// file behind that makes every subsequent build fail.
// See https://github.com/buildpacks/pack/issues/2695.
func Write(cfg interface{}, path string) error {
	dir := filepath.Dir(path)
	if err := MkdirAll(dir); err != nil {
		return err
	}

	existing, err := os.Stat(path)
	if err == nil {
		// Renaming over path only needs write access to the directory, so an
		// existing file the user made read-only would silently be replaced.
		// Fail as the previous in-place rewrite did instead. O_WRONLY without
		// O_TRUNC checks write permission without modifying the file.
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		f.Close()
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	// Removing the temp file is a no-op once it has been renamed to path.
	defer os.Remove(tmp.Name())

	// os.CreateTemp creates the file with mode 0600. Keep the mode of an
	// existing file so that rewriting it does not change its permissions;
	// new files stay 0600, as the process umask cannot be queried portably
	// and a restrictive mode is the safe default for files under PACK_HOME.
	if existing != nil {
		if err := tmp.Chmod(existing.Mode().Perm()); err != nil {
			tmp.Close()
			return err
		}
	}

	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func MkdirAll(path string) error {
	return os.MkdirAll(path, 0750)
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

func GetRegistries(cfg Config) []Registry {
	return append(cfg.Registries, DefaultRegistry())
}

func GetRegistry(cfg Config, registryName string) (Registry, error) {
	if registryName == "" && cfg.DefaultRegistryName != "" {
		registryName = cfg.DefaultRegistryName
	}
	if registryName == "" && cfg.DefaultRegistryName == "" {
		registryName = OfficialRegistryName
	}
	if registryName != "" {
		for _, registry := range GetRegistries(cfg) {
			if registry.Name == registryName {
				return registry, nil
			}
		}
	}
	return Registry{}, errors.Errorf("registry %s is not defined in your config file", style.Symbol(registryName))
}

const DefaultLifecycleImageRepo = "docker.io/buildpacksio/lifecycle"
