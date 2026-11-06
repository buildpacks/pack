package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/pack/testhelpers"
)

func TestDirs(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "dirs", testDirs, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testDirs(t *testing.T, when spec.G, it spec.S) {
	var (
		home     string
		tempDir  string
		env      map[string]string
		resolver dirResolver
	)

	it.Before(func() {
		home = filepath.Join(t.TempDir(), "home")
		h.AssertNil(t, os.MkdirAll(home, 0700))
		tempDir = filepath.Join(t.TempDir(), "tmp")
		// The spec requires XDG_* values to be absolute, so build them from an absolute
		// path on every OS.
		env = map[string]string{}
		resolver = dirResolver{
			goos:    "linux",
			getenv:  func(key string) string { return env[key] },
			homeDir: func() (string, error) { return home, nil },
			tempDir: func() string { return tempDir },
			stat:    os.Stat,
		}
	})

	type dirs struct {
		Config, State, Data, Cache, Runtime string
	}

	resolveAll := func() dirs {
		t.Helper()
		var (
			d   dirs
			err error
		)
		d.Config, err = resolver.configDir()
		h.AssertNil(t, err)
		d.State, err = resolver.stateDir()
		h.AssertNil(t, err)
		d.Data, err = resolver.dataDir()
		h.AssertNil(t, err)
		d.Cache, err = resolver.cacheDir()
		h.AssertNil(t, err)
		d.Runtime, err = resolver.runtimeDir()
		h.AssertNil(t, err)
		return d
	}

	sameDir := func(dir string) dirs {
		return dirs{Config: dir, State: dir, Data: dir, Cache: dir, Runtime: dir}
	}

	when("PACK_HOME is set", func() {
		it("uses it for everything", func() {
			packHome := filepath.Join(home, "custom-pack-home")
			env["PACK_HOME"] = packHome
			env["XDG_CONFIG_HOME"] = filepath.Join(home, "xdg-config")

			h.AssertEq(t, resolveAll(), sameDir(packHome))
		})

		it("uses it even when ~/.pack exists", func() {
			packHome := filepath.Join(home, "custom-pack-home")
			env["PACK_HOME"] = packHome
			h.AssertNil(t, os.Mkdir(filepath.Join(home, ".pack"), 0700))

			h.AssertEq(t, resolveAll(), sameDir(packHome))
		})
	})

	when("~/.pack exists", func() {
		it.Before(func() {
			h.AssertNil(t, os.Mkdir(filepath.Join(home, ".pack"), 0700))
		})

		it("uses it for everything", func() {
			env["XDG_CONFIG_HOME"] = filepath.Join(home, "xdg-config")
			env["XDG_CACHE_HOME"] = filepath.Join(home, "xdg-cache")

			h.AssertEq(t, resolveAll(), sameDir(filepath.Join(home, ".pack")))
		})
	})

	when("checking for ~/.pack fails", func() {
		it("returns the error", func() {
			resolver.stat = func(string) (os.FileInfo, error) { return nil, errors.New("some-stat-error") }

			_, err := resolver.configDir()
			h.AssertError(t, err, "some-stat-error")
		})
	})

	when("getting the user's home fails", func() {
		it("returns the error", func() {
			resolver.homeDir = func() (string, error) { return "", errors.New("some-home-error") }

			_, err := resolver.configDir()
			h.AssertError(t, err, "some-home-error")
		})
	})

	when("neither PACK_HOME nor ~/.pack is in use", func() {
		when("no XDG variables are set", func() {
			it("uses the XDG defaults", func() {
				d := resolveAll()
				h.AssertEq(t, d.Config, filepath.Join(home, ".config", "pack"))
				h.AssertEq(t, d.State, filepath.Join(home, ".local", "state", "pack"))
				h.AssertEq(t, d.Data, filepath.Join(home, ".local", "share", "pack"))
				h.AssertEq(t, d.Cache, filepath.Join(home, ".cache", "pack"))
			})

			it("doesn't create ~/.pack or any other directory", func() {
				resolveAll()

				entries, err := os.ReadDir(home)
				h.AssertNil(t, err)
				h.AssertEq(t, len(entries), 0)
			})
		})

		when("XDG variables are set", func() {
			it("uses a pack subdirectory of each", func() {
				env["XDG_CONFIG_HOME"] = filepath.Join(home, "xdg-config")
				env["XDG_STATE_HOME"] = filepath.Join(home, "xdg-state")
				env["XDG_DATA_HOME"] = filepath.Join(home, "xdg-data")
				env["XDG_CACHE_HOME"] = filepath.Join(home, "xdg-cache")
				env["XDG_RUNTIME_DIR"] = filepath.Join(home, "xdg-runtime")

				h.AssertEq(t, resolveAll(), dirs{
					Config:  filepath.Join(home, "xdg-config", "pack"),
					State:   filepath.Join(home, "xdg-state", "pack"),
					Data:    filepath.Join(home, "xdg-data", "pack"),
					Cache:   filepath.Join(home, "xdg-cache", "pack"),
					Runtime: filepath.Join(home, "xdg-runtime", "pack"),
				})
			})
		})

		when("XDG variables are empty", func() {
			it("treats them as unset", func() {
				for _, key := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR"} {
					env[key] = ""
				}

				h.AssertEq(t, resolveAll(), dirs{
					Config:  filepath.Join(home, ".config", "pack"),
					State:   filepath.Join(home, ".local", "state", "pack"),
					Data:    filepath.Join(home, ".local", "share", "pack"),
					Cache:   filepath.Join(home, ".cache", "pack"),
					Runtime: filepath.Join(home, ".cache", "pack"),
				})
			})
		})

		when("XDG variables are relative paths", func() {
			it("ignores them", func() {
				for _, key := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR"} {
					env[key] = filepath.Join("relative", key)
				}

				h.AssertEq(t, resolveAll(), dirs{
					Config:  filepath.Join(home, ".config", "pack"),
					State:   filepath.Join(home, ".local", "state", "pack"),
					Data:    filepath.Join(home, ".local", "share", "pack"),
					Cache:   filepath.Join(home, ".cache", "pack"),
					Runtime: filepath.Join(home, ".cache", "pack"),
				})
			})
		})
	})

	when("#runtimeDir", func() {
		when("XDG_RUNTIME_DIR is set", func() {
			it.Before(func() {
				env["XDG_RUNTIME_DIR"] = filepath.Join(home, "xdg-runtime")
			})

			it("uses a pack subdirectory of it", func() {
				dir, err := resolver.runtimeDir()
				h.AssertNil(t, err)
				h.AssertEq(t, dir, filepath.Join(home, "xdg-runtime", "pack"))
			})

			it("takes precedence over PACK_HOME", func() {
				env["PACK_HOME"] = filepath.Join(home, "custom-pack-home")

				dir, err := resolver.runtimeDir()
				h.AssertNil(t, err)
				h.AssertEq(t, dir, filepath.Join(home, "xdg-runtime", "pack"))
			})

			it("takes precedence over ~/.pack", func() {
				h.AssertNil(t, os.Mkdir(filepath.Join(home, ".pack"), 0700))

				dir, err := resolver.runtimeDir()
				h.AssertNil(t, err)
				h.AssertEq(t, dir, filepath.Join(home, "xdg-runtime", "pack"))
			})
		})

		when("XDG_RUNTIME_DIR isn't set", func() {
			it("uses the cache directory on Linux", func() {
				resolver.goos = "linux"
				env["XDG_CACHE_HOME"] = filepath.Join(home, "xdg-cache")

				dir, err := resolver.runtimeDir()
				h.AssertNil(t, err)
				h.AssertEq(t, dir, filepath.Join(home, "xdg-cache", "pack"))
			})

			it("uses a pack subdirectory of the temp directory on macOS", func() {
				resolver.goos = "darwin"

				dir, err := resolver.runtimeDir()
				h.AssertNil(t, err)
				h.AssertEq(t, dir, filepath.Join(tempDir, "pack"))
			})

			it("uses ~/.pack on Windows", func() {
				resolver.goos = "windows"

				dir, err := resolver.runtimeDir()
				h.AssertNil(t, err)
				h.AssertEq(t, dir, filepath.Join(home, ".pack"))
			})

			it("uses PACK_HOME when set", func() {
				resolver.goos = "darwin"
				env["PACK_HOME"] = filepath.Join(home, "custom-pack-home")

				dir, err := resolver.runtimeDir()
				h.AssertNil(t, err)
				h.AssertEq(t, dir, filepath.Join(home, "custom-pack-home"))
			})
		})
	})

	when("on macOS", func() {
		it("uses the XDG defaults rather than ~/Library", func() {
			resolver.goos = "darwin"

			d := resolveAll()
			h.AssertEq(t, d.Config, filepath.Join(home, ".config", "pack"))
			h.AssertEq(t, d.Cache, filepath.Join(home, ".cache", "pack"))
		})
	})

	when("on Windows", func() {
		it.Before(func() {
			resolver.goos = "windows"
		})

		it("uses ~/.pack for everything even when it doesn't exist", func() {
			env["XDG_CONFIG_HOME"] = filepath.Join(home, "xdg-config")

			h.AssertEq(t, resolveAll(), sameDir(filepath.Join(home, ".pack")))
		})

		it("uses PACK_HOME when set", func() {
			packHome := filepath.Join(home, "custom-pack-home")
			env["PACK_HOME"] = packHome

			h.AssertEq(t, resolveAll(), sameDir(packHome))
		})
	})
}
