package buildpackage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestMultiArchBuildpackageConfig(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Multi Arch Buildpackage Config Reader", testMultiArchBuildpackageConfigReader, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testMultiArchBuildpackageConfigReader(t *testing.T, when spec.G, it spec.S) {
	var (
		bpPath  = "./someDir"
		targets = []dist.Target{
			{
				OS:          "linux",
				Arch:        "arm",
				ArchVariant: "v6",
				Distributions: []dist.Distribution{
					{
						Name:     "ubuntu",
						Versions: []string{"22.04", "20.04"},
					},
					{
						Name:     "debian",
						Versions: []string{"8.0"},
					},
				},
				Specs: dist.TargetSpecs{
					Features:       []string{"feature1", "feature2"},
					OSFeatures:     []string{"osFeature1", "osFeature2"},
					URLs:           []string{"url1", "url2"},
					Annotations:    map[string]string{"key1": "value1", "key2": "value2"},
					Flatten:        false,
					FlattenExclude: make([]string, 0),
					Labels:         map[string]string{"io.buildpacks.distro.name": "debian"},
					Path:           "some-path",
				},
			},
			{
				OS:   "linux",
				Arch: "amd64",
				Distributions: []dist.Distribution{
					{
						Name:     "ubuntu",
						Versions: []string{"version1", "version2"},
					},
				},
			},
		}
		target = dist.Target{
			OS:          "some-os",
			Arch:        "some-arch",
			ArchVariant: "some-arch",
			Distributions: []dist.Distribution{
				{
					Name:     "some-name",
					Versions: []string{"some-version", "some-other-version"},
				},
				{
					Name:     "some-name1",
					Versions: []string{"some-version1", "some-other-version"},
				},
			},
			Specs: dist.TargetSpecs{
				Features:       []string{"some-feature"},
				OSFeatures:     []string{"some-osFeature1", "someOSFeature2"},
				URLs:           []string{"some-url1", "some-url2"},
				Annotations:    map[string]string{"some-key1": "some-key2", "some-key2": "some-value2"},
				Flatten:        true,
				FlattenExclude: []string{},
				Labels:         make(map[string]string),
				Path:           ".",
			},
		}
		buildpackURICurrent = dist.BuildpackURI{
			URI: ".",
		}
		dependencies = []dist.ImageOrURI{
			{
				BuildpackURI: dist.BuildpackURI{
					URI: ".",
				},
			},
			{
				BuildpackURI: dist.BuildpackURI{
					URI: "urn:cnb:registry:paketo-buildpacks/node-engine@3.2.1",
				},
			},
			{
				BuildpackURI: dist.BuildpackURI{
					URI: "https://example.com/buildpack.tgz",
				},
			},
			{
				BuildpackURI: dist.BuildpackURI{
					URI: "docker://cnbs/some-bp",
				},
			},
			{
				BuildpackURI: dist.BuildpackURI{
					URI: "docker://cnbs/some-bp@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				},
			},
			{
				BuildpackURI: dist.BuildpackURI{
					URI: "docker://cnbs/some-bp:some-tag",
				},
			},
			{
				ImageRef: dist.ImageRef{
					// FIXME: not sure if this ImageName is valid
					ImageName: "cnbs/sample-package@hello-universe",
				},
			},
		}
		platform      = dist.Platform{OS: "linux"}
		packageConfig = buildpackage.Config{
			Buildpack:    buildpackURICurrent,
			Dependencies: dependencies,
			Platform:     platform,
		}
	)
	when("#NewMultiArchBuildpack", func() {
		var (
			platformAPIVersion = api.Platform.Latest()
			moduleInfo         = dist.ModuleInfo{
				ID:          "some/buildpack",
				Name:        "SomeBuildpack",
				Version:     "",
				Description: "some description",
			}
			BuildPackConfig = dist.BuildpackDescriptor{
				WithInfo:    moduleInfo,
				WithAPI:     platformAPIVersion,
				WithTargets: append(targets, target),
			}
		)
		it("should return new #multiArchBuildpack pointer", func() {
			cfg := buildpackage.NewMultiArchBuildpack(BuildPackConfig, "", false, false, targets)
			h.AssertNotNil(t, cfg)
		})
		when("#multiArchBuildpack", func() {
			it("should return config targets", func() {
				cfg := buildpackage.NewMultiArchBuildpack(BuildPackConfig, "", false, false, nil)
				h.AssertNotNil(t, cfg)
				h.AssertEq(t, cfg.Targets(), append(targets, target))
			})
			it("should return cli targets", func() {
				cfg := buildpackage.NewMultiArchBuildpack(BuildPackConfig, "", false, false, targets)
				h.AssertNotNil(t, cfg)
				h.AssertEq(t, cfg.Targets(), targets)
			})
			it("should return BuildpackConfigs", func() {
				expectedConfigsLen := 9
				cfg := buildpackage.NewMultiArchBuildpack(BuildPackConfig, "", false, false, nil)
				h.AssertNotNil(t, cfg)

				cfgs, err := cfg.MultiArchConfigs()
				h.AssertNil(t, err)
				h.AssertEq(t, len(cfgs), expectedConfigsLen)
			})
			it("shouldhave expected multiArch configs", func() {
				cfg := buildpackage.NewMultiArchBuildpack(BuildPackConfig, "", false, false, nil)
				h.AssertNotNil(t, cfg)

				cfgs, err := cfg.MultiArchConfigs()
				h.AssertNil(t, err)
				h.AssertEq(t, len(cfgs) > 1, true)

				splitedTargets := splitTargets(cfg.Targets())
				h.AssertEq(t, cfgs[0].BuildpackDescriptor.WithInfo, moduleInfo)
				h.AssertEq(t, cfgs[0].BuildpackDescriptor.WithAPI, platformAPIVersion)
				h.AssertEq(t, cfgs[0].Targets()[0], splitedTargets[0])

				h.AssertEq(t, cfgs[1].BuildpackDescriptor.WithInfo, moduleInfo)
				h.AssertEq(t, cfgs[1].BuildpackDescriptor.WithAPI, platformAPIVersion)
				h.AssertEq(t, cfgs[1].Targets()[0], splitedTargets[1])
			})
		})
	})
	when("#NewMultiArchExtension", func() {
		var (
			platformAPIVersion = api.Platform.Latest()
			moduleInfo         = dist.ModuleInfo{
				ID:          "some/buildpack",
				Name:        "SomeBuildpack",
				Version:     "",
				Description: "some description",
			}
			ExtensionConfig = dist.ExtensionDescriptor{
				WithInfo:    moduleInfo,
				WithAPI:     platformAPIVersion,
				WithTargets: append(targets, target),
			}
		)
		it("should return new #multiArchExtension pointer", func() {
			cfg := buildpackage.NewMultiArchExtension(ExtensionConfig, "", targets)
			h.AssertNotNil(t, cfg)
		})
		when("#multiArchExtension", func() {
			it("should return config targets", func() {
				cfg := buildpackage.NewMultiArchExtension(ExtensionConfig, "", nil)
				h.AssertNotNil(t, cfg)
				h.AssertEq(t, cfg.Targets(), append(targets, target))
			})
			it("should return cli targets", func() {
				cfg := buildpackage.NewMultiArchExtension(ExtensionConfig, "", targets)
				h.AssertNotNil(t, cfg)
				h.AssertEq(t, cfg.Targets(), targets)
			})
			it("should return ExtensionConfigs", func() {
				expectedConfigsLen := 9
				cfg := buildpackage.NewMultiArchExtension(ExtensionConfig, "", nil)
				h.AssertNotNil(t, cfg)

				cfgs, err := cfg.MultiArchConfigs()
				h.AssertNil(t, err)
				h.AssertEq(t, len(cfgs), expectedConfigsLen)
			})
			it("should have expected multiArch configs", func() {
				cfg := buildpackage.NewMultiArchExtension(ExtensionConfig, "", nil)
				h.AssertNotNil(t, cfg)

				cfgs, err := cfg.MultiArchConfigs()
				h.AssertNil(t, err)
				h.AssertEq(t, len(cfgs) > 1, true)

				splitedTargets := splitTargets(cfg.Targets())
				h.AssertEq(t, len(cfgs), len(splitedTargets))
				h.AssertEq(t, cfgs[0].ExtensionDescriptor.WithInfo, moduleInfo)
				h.AssertEq(t, cfgs[0].ExtensionDescriptor.WithAPI, platformAPIVersion)
				h.AssertEq(t, cfgs[0].Targets()[0], splitedTargets[0])

				h.AssertEq(t, cfgs[1].ExtensionDescriptor.WithInfo, moduleInfo)
				h.AssertEq(t, cfgs[1].ExtensionDescriptor.WithAPI, platformAPIVersion)
				h.AssertEq(t, cfgs[1].Targets()[0], splitedTargets[1])
			})
		})
	})
	when("#NewMultiArchPackage", func() {
		it("should return a new #MultiArchPackage", func() {
			cfg := buildpackage.NewMultiArchPackage(packageConfig, "./some-dir")
			h.AssertNotNil(t, cfg)
			h.AssertEq(t, cfg.Buildpack, packageConfig.Buildpack)
			h.AssertEq(t, cfg.Dependencies, packageConfig.Dependencies)
			h.AssertEq(t, cfg.Extension, packageConfig.Extension)
			h.AssertEq(t, cfg.Platform, packageConfig.Platform)
		})
	})
	when("#MultiArchBuildpackConfig", func() {
		var (
			bpPath     = "./someBPPath"
			BPAPI      = api.Buildpack.Latest()
			ModuleInfo = dist.ModuleInfo{
				ID: "some/bp",
			}
			buildpackDescriptor = dist.BuildpackDescriptor{
				WithAPI:     BPAPI,
				WithInfo:    ModuleInfo,
				WithTargets: append(targets, target),
			}
			order = dist.Order{
				dist.OrderEntry{
					Group: []dist.ModuleRef{
						{
							ModuleInfo: dist.ModuleInfo{
								ID: "some/bp1",
							},
						},
						{
							ModuleInfo: dist.ModuleInfo{
								ID:      "some/bp2",
								Version: "22.04",
							},
						},
					},
				},
			}
		)
		it("should return target.Spec.Path if specified", func() {
			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, append(targets, target))
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			h.AssertEq(t, configs[0].Path(), "some-path/buildpack.toml")
			h.AssertEq(t, configs[1].Path(), "some-path/buildpack.toml")
		})
		it("should return relativeDir when target.Spec.Path not specified", func() {
			targets := append(targets, target)
			for i, t := range targets {
				t.Specs.Path = ""
				targets[i] = t
			}

			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, targets)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			h.AssertEq(t, configs[0].Path(), configs[0].RelativeBaseDir())
			h.AssertEq(t, configs[1].Path(), configs[1].RelativeBaseDir())
		})
		it("should return BP Targets", func() {
			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, nil)
			h.AssertEq(t, multiArchBP.Targets(), append(targets, target))
		})
		it("should return Flag Targets", func() {
			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, targets)
			h.AssertEq(t, multiArchBP.Targets(), targets)
		})
		it("should return flatten when explitly to true", func() {
			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, true, true, nil)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			h.AssertEq(t, configs[0].Flatten, true)
		})
		it("should return false when explicitly flatten set to false", func() {
			for i := range buildpackDescriptor.WithTargets {
				buildpackDescriptor.WithTargets[i].Specs.Flatten = true
			}

			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, true, nil)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			h.AssertEq(t, configs[0].Flatten, false)
		})
		it("should return config value(false) when not explicitly flatten value defined", func() {
			for i := range buildpackDescriptor.WithTargets {
				buildpackDescriptor.WithTargets[i].Specs.Flatten = false
			}

			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, nil)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			h.AssertEq(t, configs[0].Flatten, false)
			h.AssertEq(t, configs[1].Flatten, false)
		})
		it("should return config value(true) when not explicitly flatten value defined", func() {
			for i := range buildpackDescriptor.WithTargets {
				buildpackDescriptor.WithTargets[i].Specs.Flatten = true
			}

			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, nil)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			h.AssertEq(t, configs[0].Flatten, true)
			h.AssertEq(t, configs[1].Flatten, true)
		})
		it("should return expected len of config's multi arch configs", func() {
			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, nil)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			expectedTargets := splitTargets(append(targets, target))
			h.AssertEq(t, len(configs), len(expectedTargets))

			h.AssertEq(t, configs[0].BuildpackDescriptor.WithTargets[0], expectedTargets[0])
			h.AssertEq(t, configs[1].WithTargets[0], expectedTargets[1])

			h.AssertEq(t, configs[0].BuildpackDescriptor.WithAPI, buildpackDescriptor.WithAPI)
			h.AssertEq(t, configs[0].BuildpackDescriptor.WithInfo, buildpackDescriptor.WithInfo)
			h.AssertEq(t, configs[0].BuildpackDescriptor.WithLinuxBuild, buildpackDescriptor.WithLinuxBuild)
			h.AssertEq(t, configs[0].BuildpackDescriptor.WithOrder, buildpackDescriptor.WithOrder)
			h.AssertEq(t, configs[0].BuildpackDescriptor.WithStacks, buildpackDescriptor.WithStacks)
			h.AssertEq(t, configs[0].BuildpackDescriptor.WithWindowsBuild, buildpackDescriptor.WithWindowsBuild)

			h.AssertEq(t, configs[0].Flatten, false)
		})
		it("should return expected len of flag defined targets", func() {
			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, targets)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			expectedConfigs := splitTargets(targets)
			h.AssertEq(t, len(configs), len(expectedConfigs))

			h.AssertEq(t, configs[0].WithTargets[0], expectedConfigs[0])
			h.AssertEq(t, configs[1].WithTargets[0], expectedConfigs[1])

			h.AssertEq(t, configs[0].BuildpackDescriptor.WithAPI, buildpackDescriptor.WithAPI)
			h.AssertEq(t, configs[0].BuildpackDescriptor.WithInfo, buildpackDescriptor.WithInfo)
			h.AssertEq(t, configs[0].BuildpackDescriptor.WithLinuxBuild, buildpackDescriptor.WithLinuxBuild)
			h.AssertEq(t, configs[0].BuildpackDescriptor.WithOrder, buildpackDescriptor.WithOrder)
			h.AssertEq(t, configs[0].BuildpackDescriptor.WithStacks, buildpackDescriptor.WithStacks)
			h.AssertEq(t, configs[0].BuildpackDescriptor.WithWindowsBuild, buildpackDescriptor.WithWindowsBuild)

			h.AssertEq(t, configs[0].Flatten, false)
		})
		when("#Type", func() {
			it("should return Composite", func() {
				buildpackDescriptor.WithOrder = order
				multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, targets)
				configs, err := multiArchBP.MultiArchConfigs()
				h.AssertNil(t, err)

				h.AssertEq(t, configs[0].BuildpackType(), buildpackage.Composite)
				h.AssertEq(t, configs[1].BuildpackType(), buildpackage.Composite)
			})
			it("should return Buildpack", func() {
				multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, targets)
				configs, err := multiArchBP.MultiArchConfigs()
				h.AssertNil(t, err)

				h.AssertEq(t, configs[0].BuildpackType(), buildpackage.Buildpack)
				h.AssertEq(t, configs[1].BuildpackType(), buildpackage.Buildpack)
			})
		})
		when("#CopyBuildpackToml", func() {
			it("should copy buildpack.toml to expected path", func() {
				multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, append(targets, target))
				configs, err := multiArchBP.MultiArchConfigs()
				h.AssertNil(t, err)

				h.AssertNil(t, configs[0].CopyBuildpackToml(h.FakeIndexManifestBuilderFn(append(targets, target))))

				bp1Target := configs[0].Targets()[0]
				// platformRootBP1Dir := buildpack.PlatformRootDirectory(bp1Target, bp1Target.Distributions[0].Name, bp1Target.Distributions[0].Versions[0])
				// BP1buildpackToml := filepath.Join(bpPath, platformRootBP1Dir, BuildpackTOMLStr)

				_, err = os.Stat(configs[0].Path())
				h.AssertNil(t, err)

				config1 := &dist.BuildpackDescriptor{}
				tomlMetaDataBP1, err := toml.DecodeFile(configs[0].Path(), config1)
				h.AssertEq(t, len(tomlMetaDataBP1.Undecoded()), 0)
				h.AssertEq(t, err, nil)

				expectedBP1Config := dist.BuildpackDescriptor{
					WithAPI:          BPAPI,
					WithTargets:      []dist.Target{bp1Target},
					WithStacks:       configs[0].WithStacks,
					WithOrder:        configs[0].WithOrder,
					WithWindowsBuild: configs[0].WithWindowsBuild,
					WithLinuxBuild:   configs[0].WithLinuxBuild,
					WithInfo:         configs[0].WithInfo,
				}
				h.AssertEq(t, configs[0].BuildpackDescriptor, expectedBP1Config)

				h.AssertNil(t, configs[1].CopyBuildpackToml(h.FakeIndexManifestBuilderFn(append(targets, target))))

				bp2Target := configs[1].Targets()[0]
				_, err = os.Stat(configs[1].Path())
				h.AssertNil(t, err)

				config2 := &dist.BuildpackDescriptor{}
				tomlMetaDataBP2, err := toml.DecodeFile(configs[1].Path(), config2)
				h.AssertEq(t, len(tomlMetaDataBP2.Undecoded()), 0)
				h.AssertEq(t, err, nil)

				expectedBP2Config := dist.BuildpackDescriptor{
					WithAPI:          BPAPI,
					WithTargets:      []dist.Target{bp2Target},
					WithStacks:       configs[1].WithStacks,
					WithOrder:        configs[0].WithOrder,
					WithWindowsBuild: configs[1].WithWindowsBuild,
					WithLinuxBuild:   configs[1].WithLinuxBuild,
					WithInfo:         configs[1].WithInfo,
				}
				h.AssertEq(t, configs[1].BuildpackDescriptor, expectedBP2Config)

				h.AssertNil(t, os.Remove(configs[0].Path()))
			})
		})
		it("should cleanBuildpackToml", func() {
			var targets []dist.Target
			for _, t := range append(targets, target) {
				t.Specs.Path = ""
				targets = append(targets, t)
			}

			multiArchBP := buildpackage.NewMultiArchBuildpack(buildpackDescriptor, bpPath, false, false, targets)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			config1, config2 := configs[0], configs[1]

			h.AssertNil(t, config1.CopyBuildpackToml(h.FakeIndexManifestBuilderFn(targets)))
			h.AssertNil(t, config2.CopyBuildpackToml(h.FakeIndexManifestBuilderFn(targets)))

			_, err = os.Stat(config1.Path())
			h.AssertNil(t, err)

			_, err = os.Stat(config2.Path())
			h.AssertNil(t, err)

			// should only remove config1
			h.AssertNil(t, config1.CleanBuildpackToml())

			_, err = os.Stat(config1.Path())
			h.AssertNotNil(t, err)

			_, err = os.Stat(config2.Path())
			h.AssertNil(t, err)

			h.AssertNil(t, config2.CleanBuildpackToml())

			_, err = os.Stat(config1.Path())
			h.AssertNotNil(t, err)

			_, err = os.Stat(config2.Path())
			h.AssertNotNil(t, err)
		})
	})
	when("#MultiArchExtensionConfig", func() {
		var (
			bpPath     = "./someBPPath"
			BPAPI      = api.Buildpack.Latest()
			ModuleInfo = dist.ModuleInfo{
				ID: "some/bp",
			}
			extensionDescriptor = dist.ExtensionDescriptor{
				WithAPI:     BPAPI,
				WithInfo:    ModuleInfo,
				WithTargets: append(targets, target),
			}
		)
		it("should return target.Spec.Path if specified", func() {
			multiArchBP := buildpackage.NewMultiArchExtension(extensionDescriptor, bpPath, append(targets, target))
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			h.AssertEq(t, configs[0].Path(), "some-path/extension.toml")
			h.AssertEq(t, configs[1].Path(), "some-path/extension.toml")
		})
		it("should return relativeDir when target.Spec.Path not specified", func() {
			targets := append(targets, target)
			for i, t := range targets {
				t.Specs.Path = ""
				targets[i] = t
			}

			multiArchBP := buildpackage.NewMultiArchExtension(extensionDescriptor, bpPath, targets)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			h.AssertEq(t, configs[0].Path(), configs[0].RelativeBaseDir())
			h.AssertEq(t, configs[1].Path(), configs[1].RelativeBaseDir())
		})
		it("should return BP Targets", func() {
			multiArchBP := buildpackage.NewMultiArchExtension(extensionDescriptor, bpPath, nil)
			h.AssertEq(t, multiArchBP.Targets(), append(targets, target))
		})
		it("should return Flag Targets", func() {
			multiArchBP := buildpackage.NewMultiArchExtension(extensionDescriptor, bpPath, targets)
			h.AssertEq(t, multiArchBP.Targets(), targets)
		})
		it("should return expected len of config's multi arch configs", func() {
			multiArchBP := buildpackage.NewMultiArchExtension(extensionDescriptor, bpPath, nil)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			expectedTargets := splitTargets(append(targets, target))
			h.AssertEq(t, len(configs), len(expectedTargets))

			h.AssertEq(t, configs[0].ExtensionDescriptor.WithTargets[0], expectedTargets[0])
			h.AssertEq(t, configs[1].WithTargets[0], expectedTargets[1])

			h.AssertEq(t, configs[0].ExtensionDescriptor.WithAPI, extensionDescriptor.WithAPI)
			h.AssertEq(t, configs[0].ExtensionDescriptor.WithInfo, extensionDescriptor.WithInfo)
		})
		it("should return expected len of flag defined targets", func() {
			multiArchBP := buildpackage.NewMultiArchExtension(extensionDescriptor, bpPath, targets)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			expectedConfigs := splitTargets(targets)
			h.AssertEq(t, len(configs), len(expectedConfigs))

			h.AssertEq(t, configs[0].WithTargets[0], expectedConfigs[0])
			h.AssertEq(t, configs[1].WithTargets[0], expectedConfigs[1])

			h.AssertEq(t, configs[0].ExtensionDescriptor.WithAPI, extensionDescriptor.WithAPI)
			h.AssertEq(t, configs[0].ExtensionDescriptor.WithInfo, extensionDescriptor.WithInfo)
		})
		when("#CopyExtensionToml", func() {
			it("should copy extension.toml to expected path", func() {
				multiArchBP := buildpackage.NewMultiArchExtension(extensionDescriptor, bpPath, append(targets, target))
				configs, err := multiArchBP.MultiArchConfigs()
				h.AssertNil(t, err)

				h.AssertNil(t, configs[0].CopyExtensionToml(h.FakeIndexManifestBuilderFn(append(targets, target))))

				bp1Target := configs[0].Targets()[0]
				// platformRootBP1Dir := buildpack.PlatformRootDirectory(bp1Target, bp1Target.Distributions[0].Name, bp1Target.Distributions[0].Versions[0])
				// BP1buildpackToml := filepath.Join(bpPath, platformRootBP1Dir, BuildpackTOMLStr)

				_, err = os.Stat(configs[0].Path())
				h.AssertNil(t, err)

				config1 := &dist.ExtensionDescriptor{}
				tomlMetaDataBP1, err := toml.DecodeFile(configs[0].Path(), config1)
				h.AssertEq(t, len(tomlMetaDataBP1.Undecoded()), 0)
				h.AssertEq(t, err, nil)

				expectedBP1Config := dist.ExtensionDescriptor{
					WithAPI:     BPAPI,
					WithTargets: []dist.Target{bp1Target},
					WithInfo:    configs[0].WithInfo,
				}
				h.AssertEq(t, configs[0].ExtensionDescriptor, expectedBP1Config)

				h.AssertNil(t, configs[1].CopyExtensionToml(h.FakeIndexManifestBuilderFn(append(targets, target))))

				bp2Target := configs[1].Targets()[0]
				_, err = os.Stat(configs[1].Path())
				h.AssertNil(t, err)

				config2 := &dist.ExtensionDescriptor{}
				tomlMetaDataBP2, err := toml.DecodeFile(configs[1].Path(), config2)
				h.AssertEq(t, len(tomlMetaDataBP2.Undecoded()), 0)
				h.AssertEq(t, err, nil)

				expectedBP2Config := dist.ExtensionDescriptor{
					WithAPI:     BPAPI,
					WithTargets: []dist.Target{bp2Target},
					WithInfo:    configs[1].WithInfo,
				}
				h.AssertEq(t, configs[1].ExtensionDescriptor, expectedBP2Config)
				h.AssertNil(t, os.Remove(configs[0].Path()))
			})
		})
		it("should cleanBuildpackToml", func() {
			var targets []dist.Target
			for _, t := range append(targets, target) {
				t.Specs.Path = ""
				targets = append(targets, t)
			}

			multiArchBP := buildpackage.NewMultiArchExtension(extensionDescriptor, bpPath, targets)
			configs, err := multiArchBP.MultiArchConfigs()
			h.AssertNil(t, err)

			config1, config2 := configs[0], configs[1]

			h.AssertNil(t, config1.CopyExtensionToml(h.FakeIndexManifestBuilderFn(targets)))
			h.AssertNil(t, config2.CopyExtensionToml(h.FakeIndexManifestBuilderFn(targets)))

			_, err = os.Stat(config1.Path())
			h.AssertNil(t, err)

			_, err = os.Stat(config2.Path())
			h.AssertNil(t, err)

			// should only remove config1
			h.AssertNil(t, config1.CleanExtensionToml())

			_, err = os.Stat(config1.Path())
			h.AssertNotNil(t, err)

			_, err = os.Stat(config2.Path())
			h.AssertNil(t, err)

			h.AssertNil(t, config2.CleanExtensionToml())

			_, err = os.Stat(config1.Path())
			h.AssertNotNil(t, err)

			_, err = os.Stat(config2.Path())
			h.AssertNotNil(t, err)
		})
	})
	when("#MultiArchPackage", func() {
		it("should copy package descriptor to expected location", func() {
			tmpDir, err := os.MkdirTemp("", "someCPPKGDir")
			h.AssertNil(t, err)

			defer os.RemoveAll(bpPath)
			defer os.RemoveAll(tmpDir)

			cfg := buildpackage.NewMultiArchPackage(packageConfig, tmpDir)
			h.AssertNotNil(t, cfg)

			distro := target.Distributions[0]
			h.AssertNil(t, cfg.CopyPackageToml(bpPath, target, distro.Name, distro.Versions[0], h.FakeIndexManifestBuilderFn([]dist.Target{target})))

			platformRootDir := buildpack.PlatformRootDirectory(target, distro.Name, distro.Versions[0])

			config := &buildpackage.Config{}
			tomlMetaData, err := toml.DecodeFile(filepath.Join(bpPath, platformRootDir, "package.toml"), config)
			h.AssertEq(t, len(tomlMetaData.Undecoded()), 0)
			h.AssertEq(t, err, nil)

			path, err := filepath.Abs(tmpDir)
			h.AssertNilE(t, err)
			expectedPackageConfig := buildpackage.Config{
				Buildpack: dist.BuildpackURI{
					URI: "file://" + filepath.Join(path, platformRootDir),
				},
				Platform: dist.Platform{OS: "linux"},
				Dependencies: []dist.ImageOrURI{
					{
						BuildpackURI: dist.BuildpackURI{
							URI: "file://" + filepath.Join(path, platformRootDir),
						},
					},
					{
						BuildpackURI: dist.BuildpackURI{
							URI: "https://example.com/buildpack.tgz",
						},
					},
				},
			}

			h.AssertEq(t, config.Buildpack.URI, expectedPackageConfig.Buildpack.URI)
			h.AssertEq(t, config.Extension, expectedPackageConfig.Extension)
			h.AssertEq(t, config.Platform, expectedPackageConfig.Platform)
			for _, expDep := range expectedPackageConfig.Dependencies {
				contains := false
				for _, orgDep := range config.Dependencies {
					if expDep == orgDep {
						contains = true
						break
					}
				}
				h.AssertEq(t, contains, true)
			}
		})
		it("should cleanPackageToml", func() {
			bpPath := "./SomePKGCleanPath"
			tmpDir, err := os.MkdirTemp("", "someCleanPKGOtherDir")
			h.AssertNil(t, err)

			cfg := buildpackage.NewMultiArchPackage(packageConfig, tmpDir)
			h.AssertNotNil(t, cfg)

			distro := target.Distributions[0]
			h.AssertNil(t, cfg.CopyPackageToml(bpPath, target, distro.Name, distro.Versions[0], h.FakeIndexManifestBuilderFn([]dist.Target{target})))

			platformRootDir := buildpack.PlatformRootDirectory(target, distro.Name, distro.Versions[0])
			packageToml := filepath.Join(bpPath, platformRootDir, "package.toml")

			_, err = os.Stat(packageToml)
			h.AssertNil(t, err)

			h.AssertNil(t, cfg.CleanPackageToml(bpPath, target, distro.Name, distro.Versions[0]))

			_, err = os.Stat(packageToml)
			h.AssertNotNil(t, err)
		})
	})
	when("#DigestFromIndex", func() {
		var (
			idxMfest *v1.IndexManifest
		)
		it.Before(func() {
			fakeIndexManifestFn := h.FakeIndexManifestBuilderFn(append(targets, target))
			fakeTag, err := name.NewTag("cnbs/samples", name.Insecure, name.WeakValidation)
			h.AssertNil(t, err)

			idxMfest, err = fakeIndexManifestFn(fakeTag)
			h.AssertNil(t, err)
		})
		it("should return an error when IndexManifest is Nil", func() {
			hashStr, err := buildpackage.DigestFromIndex(nil, dist.Target{OS: "linux", Arch: "amd64"})
			h.AssertNotNil(t, err)
			h.AssertEq(t, hashStr, "")
		})
		it("should return an error when target not found in index", func() {
			hashStr, err := buildpackage.DigestFromIndex(idxMfest, dist.Target{OS: "someNotFoundOS", Arch: "someNotFoundArch"})
			h.AssertNotNil(t, err)
			h.AssertEq(t, hashStr, "")
		})
		it("should return an error when target not found in index", func() {
			hashStr, err := buildpackage.DigestFromIndex(idxMfest, dist.Target{OS: "linux", Arch: "amd64"})
			h.AssertNil(t, err)
			h.AssertNotEq(t, hashStr, "")
		})
	})
}

func splitTargets(targets []dist.Target) (out []dist.Target) {
	for _, t := range targets {
		t.Range(func(target dist.Target, distroName, distroVersion string) error {
			target.Distributions = []dist.Distribution{
				{
					Name:     distroName,
					Versions: []string{distroVersion},
				},
			}
			out = append(out, target)
			return nil
		})
	}

	return out
}
