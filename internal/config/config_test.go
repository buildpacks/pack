package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/config"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestConfig(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "config", testConfig, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testConfig(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir     string
		configPath string
	)

	it.Before(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "pack.config.test.")
		h.AssertNil(t, err)
		configPath = filepath.Join(tmpDir, "config.toml")
	})

	it.After(func() {
		err := os.RemoveAll(tmpDir)
		h.AssertNil(t, err)
	})

	when("#Read", func() {
		when("no config on disk", func() {
			it("returns an empty config", func() {
				subject, err := config.Read(configPath)
				h.AssertNil(t, err)
				h.AssertEq(t, subject.DefaultBuilder, "")
				h.AssertEq(t, len(subject.RunImages), 0)
				h.AssertEq(t, subject.Experimental, false)
				h.AssertEq(t, len(subject.RegistryMirrors), 0)
				h.AssertEq(t, subject.LayoutRepositoryDir, "")
			})
		})
	})

	when("#Write", func() {
		when("no config on disk", func() {
			it("writes config to disk", func() {
				h.AssertNil(t, config.Write(config.Config{
					DefaultBuilder: "some/builder",
					RunImages: []config.RunImage{
						{
							Image:   "some/run",
							Mirrors: []string{"example.com/some/run", "example.com/some/mirror"},
						},
						{
							Image:   "other/run",
							Mirrors: []string{"example.com/other/run", "example.com/other/mirror"},
						},
					},
					TrustedBuilders: []config.TrustedBuilder{
						{Name: "some-trusted-builder"},
					},
					RegistryMirrors: map[string]string{
						"index.docker.io": "10.0.0.1",
					},
				}, configPath))

				b, err := os.ReadFile(configPath)
				h.AssertNil(t, err)
				h.AssertContains(t, string(b), `default-builder-image = "some/builder"`)
				h.AssertContains(t, string(b), `[[run-images]]
  image = "some/run"
  mirrors = ["example.com/some/run", "example.com/some/mirror"]`)

				h.AssertContains(t, string(b), `[[run-images]]
  image = "other/run"
  mirrors = ["example.com/other/run", "example.com/other/mirror"]`)

				h.AssertContains(t, string(b), `[[trusted-builders]]
  name = "some-trusted-builder"`)

				h.AssertContains(t, string(b), `[registry-mirrors]
  "index.docker.io" = "10.0.0.1"`)
			})
		})

		when("config on disk", func() {
			it.Before(func() {
				h.AssertNil(t, os.WriteFile(configPath, []byte("some-old-contents"), 0777))
			})

			it("replaces the file", func() {
				h.AssertNil(t, config.Write(config.Config{
					DefaultBuilder: "some/builder",
				}, configPath))
				b, err := os.ReadFile(configPath)
				h.AssertNil(t, err)
				h.AssertContains(t, string(b), `default-builder-image = "some/builder"`)
				h.AssertNotContains(t, string(b), "some-old-contents")
			})
		})

		when("the file is written concurrently", func() {
			// Regression test for https://github.com/buildpacks/pack/issues/2695:
			// concurrent `pack build` runs rewrite volume-keys.toml at the same time.
			// Readers must never observe a partially written file, and the file left
			// behind must always be valid TOML.
			it("never exposes a partial or corrupted file", func() {
				// On Windows, renaming over a file that another goroutine has open
				// for reading can fail with "Access is denied", so concurrent
				// writers may get an error there instead of silently corrupting the file.
				h.SkipIf(t, runtime.GOOS == "windows", "renaming over an open file can fail on windows")
				volumeKeysPath := filepath.Join(tmpDir, "volume-keys.toml")
				newCfg := func(writer int) config.VolumeConfig {
					// Hundreds of entries make each write span multiple write(2)
					// calls, which is what lets unsynchronized writers interleave.
					keys := map[string]string{}
					for i := 0; i < 500; i++ {
						keys[fmt.Sprintf("example.com/some/image:tag-%d-%d", writer, i)] = "somerandomvolumekey"
					}
					return config.VolumeConfig{VolumeKeys: keys}
				}
				h.AssertNil(t, config.Write(newCfg(0), volumeKeysPath))

				// Errors are collected rather than asserted inside goroutines,
				// because t.FailNow must only be called from the test goroutine.
				var (
					wg   sync.WaitGroup
					mu   sync.Mutex
					errs []error
				)
				record := func(err error) {
					if err != nil {
						mu.Lock()
						errs = append(errs, err)
						mu.Unlock()
					}
				}
				for w := 1; w <= 8; w++ {
					wg.Add(2)
					go func(w int) {
						defer wg.Done()
						for i := 0; i < 20; i++ {
							record(config.Write(newCfg(w), volumeKeysPath))
						}
					}(w)
					go func() {
						defer wg.Done()
						for i := 0; i < 20; i++ {
							_, err := config.ReadVolumeKeys(volumeKeysPath)
							record(err)
						}
					}()
				}
				wg.Wait()

				h.AssertNil(t, errors.Join(errs...))
				cfg, err := config.ReadVolumeKeys(volumeKeysPath)
				h.AssertNil(t, err)
				h.AssertEq(t, len(cfg.VolumeKeys), 500)
			})
		})

		when("encoding fails", func() {
			it("leaves the existing file untouched", func() {
				h.AssertNil(t, os.WriteFile(configPath, []byte("some-old-contents"), 0600))

				// TOML cannot encode a channel, so Encode returns an error.
				err := config.Write(map[string]interface{}{"bad": make(chan int)}, configPath)
				h.AssertNotNil(t, err)

				b, err := os.ReadFile(configPath)
				h.AssertNil(t, err)
				h.AssertEq(t, string(b), "some-old-contents")
				assertOnlyEntry(t, tmpDir, "config.toml")
			})
		})

		when("writing succeeds", func() {
			it("leaves no temporary files behind", func() {
				h.AssertNil(t, config.Write(config.Config{DefaultBuilder: "some/builder"}, configPath))
				h.AssertNil(t, config.Write(config.Config{DefaultBuilder: "other/builder"}, configPath))
				assertOnlyEntry(t, tmpDir, "config.toml")
			})

			it("keeps the permissions of the existing file", func() {
				h.SkipIf(t, runtime.GOOS == "windows", "unix permission bits are not supported on windows")
				h.AssertNil(t, os.WriteFile(configPath, []byte("some-old-contents"), 0600))
				// Chmod explicitly so the result does not depend on the process umask.
				h.AssertNil(t, os.Chmod(configPath, 0640))

				h.AssertNil(t, config.Write(config.Config{DefaultBuilder: "some/builder"}, configPath))

				fi, err := os.Stat(configPath)
				h.AssertNil(t, err)
				h.AssertEq(t, fi.Mode().Perm(), os.FileMode(0640))
			})
		})

		when("directories are missing", func() {
			it("creates the directories", func() {
				missingDirConfigPath := filepath.Join(tmpDir, "not", "yet", "created", "config.toml")
				h.AssertNil(t, config.Write(config.Config{
					DefaultBuilder: "some/builder",
				}, missingDirConfigPath))

				b, err := os.ReadFile(missingDirConfigPath)
				h.AssertNil(t, err)
				h.AssertContains(t, string(b), `default-builder-image = "some/builder"`)
			})
		})
	})

	when("#MkdirAll", func() {
		when("the directory doesn't exist yet", func() {
			it("creates the directory", func() {
				path := filepath.Join(tmpDir, "a-new-dir")
				err := config.MkdirAll(path)
				h.AssertNil(t, err)
				fi, err := os.Stat(path)
				h.AssertNil(t, err)
				h.AssertEq(t, fi.Mode().IsDir(), true)
			})
		})

		when("the directory already exists", func() {
			it("doesn't error", func() {
				err := config.MkdirAll(tmpDir)
				h.AssertNil(t, err)
				fi, err := os.Stat(tmpDir)
				h.AssertNil(t, err)
				h.AssertEq(t, fi.Mode().IsDir(), true)
			})
		})
	})

	when("#SetRunImageMirrors", func() {
		when("run image exists in config", func() {
			it("replaces the mirrors", func() {
				cfg := config.SetRunImageMirrors(
					config.Config{
						RunImages: []config.RunImage{
							{
								Image:   "some/run-image",
								Mirrors: []string{"old/mirror", "other/mirror"},
							},
						},
					},
					"some/run-image",
					[]string{"some-other/run"},
				)

				h.AssertEq(t, len(cfg.RunImages), 1)
				h.AssertEq(t, cfg.RunImages[0].Image, "some/run-image")
				h.AssertSliceContainsOnly(t, cfg.RunImages[0].Mirrors, "some-other/run")
			})
		})

		when("run image does not exist in config", func() {
			it("adds the run image", func() {
				cfg := config.SetRunImageMirrors(
					config.Config{},
					"some/run-image",
					[]string{"some-other/run"},
				)

				h.AssertEq(t, len(cfg.RunImages), 1)
				h.AssertEq(t, cfg.RunImages[0].Image, "some/run-image")
				h.AssertSliceContainsOnly(t, cfg.RunImages[0].Mirrors, "some-other/run")
			})
		})
	})

	when("#GetRegistry", func() {
		it("should return a default registry", func() {
			cfg := config.Config{}

			registry, err := config.GetRegistry(cfg, "")

			h.AssertNil(t, err)
			h.AssertEq(t, registry, config.Registry{
				Name: "official",
				Type: "github",
				URL:  "https://github.com/buildpacks/registry-index",
			})
		})

		it("should return the corresponding registry", func() {
			cfg := config.Config{
				Registries: []config.Registry{
					{
						Name: "registry",
						Type: "github",
						URL:  "https://github.com/registry/buildpack-registry",
					},
				},
			}

			registry, err := config.GetRegistry(cfg, "registry")

			h.AssertNil(t, err)
			h.AssertEq(t, registry, config.Registry{
				Name: "registry",
				Type: "github",
				URL:  "https://github.com/registry/buildpack-registry",
			})
		})

		it("should return the first matched registry", func() {
			cfg := config.Config{
				Registries: []config.Registry{
					{
						Name: "duplicate registry",
						Type: "github",
						URL:  "https://github.com/duplicate1/buildpack-registry",
					},
					{
						Name: "duplicate registry",
						Type: "github",
						URL:  "https://github.com/duplicate2/buildpack-registry",
					},
				},
			}

			registry, err := config.GetRegistry(cfg, "duplicate registry")

			h.AssertNil(t, err)
			h.AssertEq(t, registry, config.Registry{
				Name: "duplicate registry",
				Type: "github",
				URL:  "https://github.com/duplicate1/buildpack-registry",
			})
		})

		it("should return an error when mismatched", func() {
			cfg := config.Config{}
			_, err := config.GetRegistry(cfg, "missing")
			h.AssertError(t, err, "registry 'missing' is not defined in your config file")
		})
	})
	when("#DefaultConfigPath", func() {
		it.Before(func() {
			h.AssertNil(t, os.Setenv("PACK_HOME", tmpDir))
		})

		it.After(func() {
			h.AssertNil(t, os.Unsetenv("PACK_HOME"))
		})

		it("returns config path", func() {
			cfgPath, err := config.DefaultConfigPath()
			h.AssertNil(t, err)
			h.AssertEq(t, cfgPath, filepath.Join(tmpDir, "config.toml"))
		})
	})
}

// assertOnlyEntry asserts that dir contains exactly one entry named name,
// i.e. that Write did not leave any temporary files behind.
func assertOnlyEntry(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	h.AssertNil(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	h.AssertEq(t, names, []string{name})
}
