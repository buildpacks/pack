package builder_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/pkg/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestConfig(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "testConfig", testConfig, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testConfig(t *testing.T, when spec.G, it spec.S) {
	when("#ReadConfig", func() {
		var (
			tmpDir            string
			builderConfigPath string
			err               error
		)

		it.Before(func() {
			tmpDir, err = os.MkdirTemp("", "config-test")
			h.AssertNil(t, err)
			builderConfigPath = filepath.Join(tmpDir, "builder.toml")
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		when("file is written properly", func() {
			it.Before(func() {
				h.AssertNil(t, os.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
  id = "buildpack/1"
  version = "0.0.1"
  uri = "https://example.com/buildpack-1.tgz"

[[buildpacks]]
  image = "example.com/buildpack:2"

[[buildpacks]]
  uri = "https://example.com/buildpack-3.tgz"

[[order]]
[[order.group]]
  id = "buildpack/1"
`), 0666))
			})

			it("returns a builder config", func() {
				builderConfig, warns, err := builder.ReadConfig(builderConfigPath)
				h.AssertNil(t, err)
				h.AssertEq(t, len(warns), 0)

				h.AssertEq(t, builderConfig.Buildpacks[0].ID, "buildpack/1")
				h.AssertEq(t, builderConfig.Buildpacks[0].Version, "0.0.1")
				h.AssertEq(t, builderConfig.Buildpacks[0].URI, "https://example.com/buildpack-1.tgz")
				h.AssertEq(t, builderConfig.Buildpacks[0].ImageName, "")

				h.AssertEq(t, builderConfig.Buildpacks[1].ID, "")
				h.AssertEq(t, builderConfig.Buildpacks[1].URI, "")
				h.AssertEq(t, builderConfig.Buildpacks[1].ImageName, "example.com/buildpack:2")

				h.AssertEq(t, builderConfig.Buildpacks[2].ID, "")
				h.AssertEq(t, builderConfig.Buildpacks[2].URI, "https://example.com/buildpack-3.tgz")
				h.AssertEq(t, builderConfig.Buildpacks[2].ImageName, "")

				h.AssertEq(t, builderConfig.Order[0].Group[0].ID, "buildpack/1")
			})
		})

		when("an error occurs while reading", func() {
			it("bubbles up the error", func() {
				_, _, err := builder.ReadConfig(builderConfigPath)
				h.AssertError(t, err, "opening config file")
			})
		})

		when("detecting warnings", func() {
			when("'groups' field is used", func() {
				it.Before(func() {
					h.AssertNil(t, os.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
  id = "some.buildpack"
  version = "some.buildpack.version"

[[groups]]
[[groups.buildpacks]]
  id = "some.buildpack"
  version = "some.buildpack.version"

[[order]]
[[order.group]]
  id = "some.buildpack"
`), 0666))
				})

				it("returns error when obsolete 'groups' field is used", func() {
					_, warns, err := builder.ReadConfig(builderConfigPath)
					h.AssertError(t, err, "parse contents of")
					h.AssertEq(t, len(warns), 0)
				})
			})

			when("'order' is missing or empty", func() {
				it.Before(func() {
					h.AssertNil(t, os.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
  id = "some.buildpack"
  version = "some.buildpack.version"
`), 0666))
				})

				it("returns warnings", func() {
					_, warns, err := builder.ReadConfig(builderConfigPath)
					h.AssertNil(t, err)

					h.AssertSliceContainsOnly(t, warns, "empty 'order' definition")
				})
			})

			when("unknown buildpack key is present", func() {
				it.Before(func() {
					h.AssertNil(t, os.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
url = "noop-buildpack.tgz"
`), 0666))
				})

				it("returns an error", func() {
					_, _, err := builder.ReadConfig(builderConfigPath)
					h.AssertError(t, err, "unknown configuration element 'buildpacks.url'")
				})
			})

			when("unknown array table is present", func() {
				it.Before(func() {
					h.AssertNil(t, os.WriteFile(builderConfigPath, []byte(`
[[buidlpack]]
uri = "noop-buildpack.tgz"
`), 0666))
				})

				it("returns an error", func() {
					_, _, err := builder.ReadConfig(builderConfigPath)
					h.AssertError(t, err, "unknown configuration element 'buidlpack'")
				})
			})
		})
	})

	when("#ValidateConfig()", func() {
		var (
			testID         = "testID"
			testRunImage   = "test-run-image"
			testBuildImage = "test-build-image"
		)

		it("returns error if no stack id and no run images", func() {
			config := builder.Config{
				Stack: builder.StackConfig{
					BuildImage: testBuildImage,
					RunImage:   testRunImage,
				}}
			h.AssertError(t, builder.ValidateConfig(config), "run.images are required")
		})

		it("returns error if no build image", func() {
			config := builder.Config{
				Stack: builder.StackConfig{
					ID:       testID,
					RunImage: testRunImage,
				}}
			h.AssertError(t, builder.ValidateConfig(config), "build.image is required")
		})

		it("returns error if no run image", func() {
			config := builder.Config{
				Stack: builder.StackConfig{
					ID:         testID,
					BuildImage: testBuildImage,
				}}
			h.AssertError(t, builder.ValidateConfig(config), "run.images are required")
		})

		it("returns error if no run images image", func() {
			config := builder.Config{
				Build: builder.BuildConfig{
					Image: testBuildImage,
				},
				Run: builder.RunConfig{
					Images: []builder.RunImageConfig{{
						Image: "",
					}},
				}}
			h.AssertError(t, builder.ValidateConfig(config), "run.images.image is required")
		})

		it("returns error if no stack or run image", func() {
			config := builder.Config{
				Build: builder.BuildConfig{
					Image: testBuildImage,
				}}
			h.AssertError(t, builder.ValidateConfig(config), "run.images are required")
		})

		it("returns error if no stack and no build image", func() {
			config := builder.Config{
				Run: builder.RunConfig{
					Images: []builder.RunImageConfig{{
						Image: testBuildImage,
					}},
				}}
			h.AssertError(t, builder.ValidateConfig(config), "build.image is required")
		})

		it("returns error if no stack, run, or build image", func() {
			config := builder.Config{}
			h.AssertError(t, builder.ValidateConfig(config), "build.image is required")
		})
	})
	when("#ParseBuildConfigEnv()", func() {
		it("should return an error when name is not defined", func() {
			_, _, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{
				{
					Name:  "",
					Value: "vaiue",
				},
			}, "")
			h.AssertNotNil(t, err)
		})
		it("should warn when the value is nil or empty string", func() {
			env, warn, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{
				{
					Name:   "key",
					Value:  "",
					Suffix: "override",
				},
			}, "")

			h.AssertNotNil(t, warn)
			h.AssertNil(t, err)
			h.AssertMapContains[string, string](t, env, h.NewKeyValue[string, string]("key.override", ""))
		})
		it("should return an error when unknown suffix is specified", func() {
			_, _, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{
				{
					Name:   "key",
					Value:  "",
					Suffix: "invalid",
				},
			}, "")

			h.AssertNotNil(t, err)
		})
		it("should override and show a warning when suffix or delim is defined multiple times", func() {
			env, warn, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{
				{
					Name:   "key1",
					Value:  "value1",
					Suffix: "append",
					Delim:  "%",
				},
				{
					Name:   "key1",
					Value:  "value2",
					Suffix: "append",
					Delim:  ",",
				},
				{
					Name:   "key1",
					Value:  "value3",
					Suffix: "default",
					Delim:  ";",
				},
				{
					Name:   "key1",
					Value:  "value4",
					Suffix: "prepend",
					Delim:  ":",
				},
			}, "")

			h.AssertNotNil(t, warn)
			h.AssertNil(t, err)
			h.AssertMapContains[string, string](
				t,
				env,
				h.NewKeyValue[string, string]("key1.append", "value2"),
				h.NewKeyValue[string, string]("key1.default", "value3"),
				h.NewKeyValue[string, string]("key1.prepend", "value4"),
				h.NewKeyValue[string, string]("key1.delim", ":"),
			)
			h.AssertMapNotContains[string, string](
				t,
				env,
				h.NewKeyValue[string, string]("key1.append", "value1"),
				h.NewKeyValue[string, string]("key1.delim", "%"),
				h.NewKeyValue[string, string]("key1.delim", ","),
				h.NewKeyValue[string, string]("key1.delim", ";"),
			)
		})
		it("should return an error when `suffix` is defined as `append` or `prepend` without a `delim`", func() {
			_, warn, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{
				{
					Name:   "key",
					Value:  "value",
					Suffix: "append",
				},
			}, "")

			h.AssertNotNil(t, warn)
			h.AssertNotNil(t, err)
		})
		it("when suffix is NONE or omitted should default to `override`", func() {
			env, warn, err := builder.ParseBuildConfigEnv([]builder.BuildConfigEnv{
				{
					Name:   "key",
					Value:  "value",
					Suffix: "",
				},
			}, "")

			h.AssertNotNil(t, warn)
			h.AssertNil(t, err)
			h.AssertMapContains[string, string](t, env, h.NewKeyValue[string, string]("key", "value"))
		})
	})
	when("#ReadMultiArchConfig", func() {
		var (
			tmpDir            string
			builderConfigPath string
			err               error
			builderConfig     builder.MultiArchConfig
		)
		it.Before(func() {
			tmpDir, err = os.MkdirTemp("", "config-test")
			h.AssertNil(t, err)
			builderConfigPath = filepath.Join(tmpDir, "builder.toml")
			builderConfig = builder.MultiArchConfig{
				Config: builder.Config{
					Buildpacks: builder.ModuleCollection{
						{
							ImageOrURI: dist.ImageOrURI{
								BuildpackURI: dist.BuildpackURI{
									URI: "busybox:1.36-musl",
								},
							},
						},
					},
					WithTargets: []dist.Target{
						{
							OS:   "linux",
							Arch: "amd64",
						},
						{
							OS:          "linux",
							Arch:        "arm",
							ArchVariant: "v6",
						},
					},
					Order: dist.Order{
						dist.OrderEntry{
							Group: []dist.ModuleRef{
								{
									ModuleInfo: dist.ModuleInfo{
										Name:    "busybox",
										Version: "1.36-musl",
									},
								},
							},
						},
					},
				},
			}
		})
		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})
		it("should return multi-arch config", func() {
			file, err := os.OpenFile(builderConfigPath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, os.ModePerm)
			h.AssertNil(t, err)
			h.AssertNil(t, toml.NewEncoder(file).Encode(&builderConfig))

			config, warnings, err := builder.ReadMultiArchConfig(builderConfigPath, nil)
			h.AssertNil(t, err)
			h.AssertEq(t, len(warnings), 0)
			h.AssertEq(t, config.Config, builderConfig.Config)
		})
		it("should return multi-arch config with flag targets", func() {
			file, err := os.OpenFile(builderConfigPath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, os.ModePerm)
			h.AssertNil(t, err)
			h.AssertNil(t, toml.NewEncoder(file).Encode(&builderConfig))

			config, warnings, err := builder.ReadMultiArchConfig(builderConfigPath, []dist.Target{
				{
					OS:   "some-os",
					Arch: "some-arch",
				},
			})
			h.AssertNil(t, err)
			h.AssertEq(t, len(warnings), 0)
			h.AssertEq(t, config.Config, builderConfig.Config)
			h.AssertNotEq(t, config.Targets(), builderConfig.Config.WithTargets)
		})
		it("should return an error", func() {
			_, _, err := builder.ReadMultiArchConfig(builderConfigPath, nil)
			h.AssertNotNil(t, err)
		})
		it("should return a warning when order is not specified", func() {
			file, err := os.OpenFile(builderConfigPath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, os.ModePerm)
			h.AssertNil(t, err)

			builderConfig := builderConfig
			builderConfig.Order = nil

			h.AssertNil(t, toml.NewEncoder(file).Encode(&builderConfig))

			config, warnings, err := builder.ReadMultiArchConfig(builderConfigPath, nil)
			h.AssertNil(t, err)
			h.AssertEq(t, len(warnings), 1)
			h.AssertEq(t, config.Config, builderConfig.Config)
		})
		when("#BuilderConfigs", func() {
			var (
				config   builder.MultiArchConfig
				warnings []string
			)
			it.Before(func() {
				builderConfig.Extensions = builder.ModuleCollection{
					{
						ImageOrURI: dist.ImageOrURI{BuildpackURI: dist.BuildpackURI{URI: "./some-uri"}},
					},
				}
				builderConfig.Build.Image = "some/build:image"
				builderConfig.Run.Images = append(builderConfig.Run.Images, builder.RunImageConfig{
					Image:   "$ome/image+",
					Mirrors: []string{"some/run-image:mirror"},
				})
				builderConfig.Stack = builder.StackConfig{
					BuildImage:      "some/stack:build-image",
					RunImage:        "$ome/run-image",
					RunImageMirrors: []string{"some/stack:run-image1"},
				}
				file, err := os.OpenFile(builderConfigPath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, os.ModePerm)
				h.AssertNil(t, err)
				h.AssertNil(t, toml.NewEncoder(file).Encode(&builderConfig))
				config, warnings, err = builder.ReadMultiArchConfig(builderConfigPath, nil)
				h.AssertEq(t, len(warnings), 0)
				h.AssertEq(t, err, nil)
			})
			it.After(func() {
				h.AssertNil(t, os.RemoveAll(tmpDir))
			})
			it("should return multiple configs", func() {
				configs, err := config.BuilderConfigs(h.FakeIndexManifestBuilderFn(config.Targets()))
				h.AssertNil(t, err)
				h.AssertEq(t, len(configs), len(config.Targets()))
			})
		})
		when("#MultiArch", func() {
			it("should return true when multi-target config provided", func() {
				h.AssertTrue(t, builderConfig.MultiArch())
			})
			it("should return true when multi-distro config provided", func() {
				builderConfig := builderConfig
				builderConfig.WithTargets = []dist.Target{
					{
						Distributions: []dist.Distribution{
							{
								Name: "distro1",
							}, {
								Name: "distro2",
							},
						},
					},
				}
				h.AssertTrue(t, builderConfig.MultiArch())
			})
			it("should return true when single distro multi-version config provided", func() {
				builderConfig := builderConfig
				builderConfig.WithTargets = []dist.Target{
					{
						Distributions: []dist.Distribution{
							{
								Name:     "distro1",
								Versions: []string{"version1", "version2"},
							},
						},
					},
				}
				h.AssertTrue(t, builderConfig.MultiArch())
			})
		})
	})
}
