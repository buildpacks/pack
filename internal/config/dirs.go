package config

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// dirResolver resolves the directories pack stores its files in. Its fields are
// injectable so that every platform's behavior can be tested on any OS.
type dirResolver struct {
	goos    string
	getenv  func(string) string
	homeDir func() (string, error)
	tempDir func() string
	stat    func(string) (os.FileInfo, error)
}

var defaultDirResolver = dirResolver{
	goos:    runtime.GOOS,
	getenv:  os.Getenv,
	homeDir: os.UserHomeDir,
	tempDir: os.TempDir,
	stat:    os.Stat,
}

// ConfigDir returns the directory for user settings, such as config.toml.
func ConfigDir() (string, error) {
	return defaultDirResolver.configDir()
}

// StateDir returns the directory for data that should persist between runs but isn't
// worth backing up, such as volume-keys.toml.
func StateDir() (string, error) {
	return defaultDirResolver.stateDir()
}

// DataDir returns the directory for user data files, such as shell completion scripts.
func DataDir() (string, error) {
	return defaultDirResolver.dataDir()
}

// CacheDir returns the directory for files that can be safely deleted and recreated,
// such as downloaded buildpacks and registry clones.
func CacheDir() (string, error) {
	return defaultDirResolver.cacheDir()
}

// RuntimeDir returns the directory for files that only need to last for a session,
// such as image indexes being composed with `pack manifest`.
func RuntimeDir() (string, error) {
	return defaultDirResolver.runtimeDir()
}

// legacyHome returns the single directory that holds all of pack's files, if one is in use:
// $PACK_HOME when set, otherwise ~/.pack when it exists, or always on Windows.
func (r dirResolver) legacyHome() (string, bool, error) {
	if packHome := r.getenv("PACK_HOME"); packHome != "" {
		return packHome, true, nil
	}

	home, err := r.homeDir()
	if err != nil {
		return "", false, errors.Wrap(err, "getting user home")
	}

	legacyHome := filepath.Join(home, ".pack")
	if r.goos == "windows" {
		return legacyHome, true, nil
	}

	_, err = r.stat(legacyHome)
	switch {
	case err == nil:
		return legacyHome, true, nil
	case os.IsNotExist(err):
		return "", false, nil
	default:
		return "", false, errors.Wrapf(err, "checking for %s", legacyHome)
	}
}

func (r dirResolver) configDir() (string, error) {
	return r.xdgDir("XDG_CONFIG_HOME", ".config")
}

func (r dirResolver) stateDir() (string, error) {
	return r.xdgDir("XDG_STATE_HOME", ".local", "state")
}

func (r dirResolver) dataDir() (string, error) {
	return r.xdgDir("XDG_DATA_HOME", ".local", "share")
}

func (r dirResolver) cacheDir() (string, error) {
	return r.xdgDir("XDG_CACHE_HOME", ".cache")
}

func (r dirResolver) runtimeDir() (string, error) {
	// XDG_RUNTIME_DIR has always overridden where pack stores image indexes, so it
	// takes precedence over the legacy home.
	if dir := r.getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "pack"), nil
	}

	if legacyHome, ok, err := r.legacyHome(); err != nil || ok {
		return legacyHome, err
	}

	// On macOS the temp dir is per-user and cleaned automatically, but on Linux it's
	// usually the shared /tmp, so the cache dir is used instead.
	if r.goos == "darwin" {
		return filepath.Join(r.tempDir(), "pack"), nil
	}
	return r.cacheDir()
}

// xdgDir returns the legacy home if one is in use, otherwise the pack subdirectory of the
// XDG base directory named by envVar. Empty and relative values of envVar are ignored, as
// the XDG Base Directory spec requires, in favor of defaultPath under the user's home.
func (r dirResolver) xdgDir(envVar string, defaultPath ...string) (string, error) {
	if legacyHome, ok, err := r.legacyHome(); err != nil || ok {
		return legacyHome, err
	}

	if dir := r.getenv(envVar); filepath.IsAbs(dir) {
		return filepath.Join(dir, "pack"), nil
	}

	home, err := r.homeDir()
	if err != nil {
		return "", errors.Wrap(err, "getting user home")
	}
	return filepath.Join(append(append([]string{home}, defaultPath...), "pack")...), nil
}
