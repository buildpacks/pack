package commands_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/project"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuildCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testBuildCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testBuildCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         *ilogging.LogWithWriters
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
		cfg            config.Config
	)

	it.Before(func() {
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		cfg = config.Config{}
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)

		command = commands.Build(logger, cfg, mockClient)
	})

	when("#BuildCommand", func() {
		when("no builder is specified", func() {
			it("returns a soft error", func() {
				mockClient.EXPECT().
					InspectBuilder(gomock.Any(), false).
					Return(&pack.BuilderInfo{Description: ""}, nil).
					AnyTimes()

				command.SetArgs([]string{"image"})
				err := command.Execute()
				h.AssertError(t, err, pack.NewSoftError().Error())
			})
		})

		when("a builder and image are set", func() {
			it("builds an image with a builder", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithImage("my-builder", "image")).
					Return(nil)

				command.SetArgs([]string{"--builder", "my-builder", "image"})
				h.AssertNil(t, command.Execute())
			})

			it("builds an image with a builder short command arg", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithImage("my-builder", "image")).
					Return(nil)

				logger.WantVerbose(true)
				command.SetArgs([]string{"-B", "my-builder", "image"})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Builder 'my-builder' is untrusted")
			})

			when("the builder is trusted", func() {
				it("sets the trust builder option", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithTrustedBuilder(true)).
						Return(nil)

					cfg := config.Config{TrustedBuilders: []config.TrustedBuilder{{Name: "my-builder"}}}
					command := commands.Build(logger, cfg, mockClient)

					logger.WantVerbose(true)
					command.SetArgs([]string{"image", "--builder", "my-builder"})
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, outBuf.String(), "Builder 'my-builder' is trusted")
				})
			})

			when("the builder is suggested", func() {
				it("sets the trust builder option", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithTrustedBuilder(true)).
						Return(nil)

					logger.WantVerbose(true)
					command.SetArgs([]string{"image", "--builder", "heroku/buildpacks:18"})
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, outBuf.String(), "Builder 'heroku/buildpacks:18' is trusted")
				})
			})
		})

		when("--buildpack-registry flag is specified but experimental isn't set in the config", func() {
			it("errors with a descriptive message", func() {
				command.SetArgs([]string{"image", "--builder", "my-builder", "--buildpack-registry", "some-registry"})
				err := command.Execute()
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "Support for buildpack registries is currently experimental.")
			})
		})

		when("a network is given", func() {
			it("forwards the network onto the client", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithNetwork("my-network")).
					Return(nil)

				command.SetArgs([]string{"image", "--builder", "my-builder", "--network", "my-network"})
				h.AssertNil(t, command.Execute())
			})
		})

		when("--asset-package is used", func() {
			it.Before(func() {
				// TODO: remove when asset packages are no longer experimental
				cfg.Experimental = true
				command = commands.Build(logger, cfg, mockClient)
			})
			it("passes asset packages to client", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithAssetPackage("first-asset", "second-asset")).
					Return(nil)

				command.SetArgs([]string{"image", "--builder", "my-builder", "--asset-package", "first-asset", "--asset-package", "second-asset"})
				h.AssertNil(t, command.Execute())
			})
		})

		when("--pull-policy", func() {
			it("sets pull-policy=never", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithPullPolicy(pubcfg.PullNever)).
					Return(nil)

				command.SetArgs([]string{"image", "--builder", "my-builder", "--pull-policy", "never"})
				h.AssertNil(t, command.Execute())
			})

			it("returns error for unknown policy", func() {
				command.SetArgs([]string{"image", "--builder", "my-builder", "--pull-policy", "unknown-policy"})
				h.AssertError(t, command.Execute(), "parsing pull policy")
			})
			it("takes precedence over a configured pull policy", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithPullPolicy(pubcfg.PullNever)).
					Return(nil)

				cfg := config.Config{PullPolicy: "if-not-present"}
				command := commands.Build(logger, cfg, mockClient)

				logger.WantVerbose(true)
				command.SetArgs([]string{"image", "--builder", "my-builder", "--pull-policy", "never"})
				h.AssertNil(t, command.Execute())
			})
		})

		when("--pull-policy is not specified", func() {
			when("no pull policy set in config", func() {
				it("uses the default policy", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithPullPolicy(pubcfg.PullAlways)).
						Return(nil)

					command.SetArgs([]string{"image", "--builder", "my-builder"})
					h.AssertNil(t, command.Execute())
				})
			})
			when("pull policy is set in config", func() {
				it("uses the set policy", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithPullPolicy(pubcfg.PullNever)).
						Return(nil)

					cfg := config.Config{PullPolicy: "never"}
					command := commands.Build(logger, cfg, mockClient)

					logger.WantVerbose(true)
					command.SetArgs([]string{"image", "--builder", "my-builder"})
					h.AssertNil(t, command.Execute())
				})
			})
		})

		when("volume mounts are specified", func() {
			it("mounts the volumes", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithVolumes([]string{"a:b", "c:d"})).
					Return(nil)

				command.SetArgs([]string{"image", "--builder", "my-builder", "--volume", "a:b", "--volume", "c:d"})
				h.AssertNil(t, command.Execute())
			})

			it("warns when running with an untrusted builder", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithVolumes([]string{"a:b", "c:d"})).
					Return(nil)

				command.SetArgs([]string{"image", "--builder", "my-builder", "--volume", "a:b", "--volume", "c:d"})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Warning: Using untrusted builder with volume mounts")
			})
		})

		when("a default process is specified", func() {
			it("sets that process", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsDefaultProcess("my-proc")).
					Return(nil)

				command.SetArgs([]string{"image", "--builder", "my-builder", "--default-process", "my-proc"})
				h.AssertNil(t, command.Execute())
			})
		})

		when("env file", func() {
			when("an env file is provided", func() {
				var envPath string

				it.Before(func() {
					envfile, err := ioutil.TempFile("", "envfile")
					h.AssertNil(t, err)
					defer envfile.Close()

					envfile.WriteString(`KEY=VALUE`)
					envPath = envfile.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(envPath))
				})

				it("builds an image env variables read from the env file", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithEnv(map[string]string{
							"KEY": "VALUE",
						})).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "image", "--env-file", envPath})
					h.AssertNil(t, command.Execute())
				})
			})

			when("a env file is provided but doesn't exist", func() {
				it("fails to run", func() {
					command.SetArgs([]string{"--builder", "my-builder", "image", "--env-file", ""})
					err := command.Execute()
					h.AssertError(t, err, "parse env file")
				})
			})

			when("an empty env file is provided", func() {
				var envPath string

				it.Before(func() {
					envfile, err := ioutil.TempFile("", "envfile")
					h.AssertNil(t, err)
					defer envfile.Close()

					envfile.WriteString(``)
					envPath = envfile.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(envPath))
				})

				it("successfully builds", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithEnv(map[string]string{})).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "image", "--env-file", envPath})
					h.AssertNil(t, command.Execute())
				})
			})

			when("two env files are provided with conflicted keys", func() {
				var envPath1 string
				var envPath2 string

				it.Before(func() {
					envfile1, err := ioutil.TempFile("", "envfile")
					h.AssertNil(t, err)
					defer envfile1.Close()

					envfile1.WriteString("KEY1=VALUE1\nKEY2=IGNORED")
					envPath1 = envfile1.Name()

					envfile2, err := ioutil.TempFile("", "envfile")
					h.AssertNil(t, err)
					defer envfile2.Close()

					envfile2.WriteString("KEY2=VALUE2")
					envPath2 = envfile2.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(envPath1))
					h.AssertNil(t, os.RemoveAll(envPath2))
				})

				it("builds an image with the last value of each env variable", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithEnv(map[string]string{
							"KEY1": "VALUE1",
							"KEY2": "VALUE2",
						})).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "image", "--env-file", envPath1, "--env-file", envPath2})
					h.AssertNil(t, command.Execute())
				})
			})
		})

		when("a cache-image passed", func() {
			when("--publish is not used", func() {
				it("errors", func() {
					command.SetArgs([]string{"--builder", "my-builder", "image", "--cache-image", "some-cache-image"})
					err := command.Execute()
					h.AssertError(t, err, "cache-image flag requires the publish flag")
				})
			})
			when("--publish is used", func() {
				it("succeeds", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithCacheImage("some-cache-image")).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "image", "--cache-image", "some-cache-image", "--publish"})
					h.AssertNil(t, command.Execute())
				})
			})
		})

		when("a valid lifecycle-image is provided", func() {
			when("only the image repo is provided", func() {
				it("uses the provided lifecycle-image and parses it correctly", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithLifecycleImage("index.docker.io/library/some-lifecycle-image:latest")).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "image", "--lifecycle-image", "some-lifecycle-image"})
					h.AssertNil(t, command.Execute())
				})
			})
			when("a custom image repo is provided", func() {
				it("uses the provided lifecycle-image and parses it correctly", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithLifecycleImage("test.com/some-lifecycle-image:latest")).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "image", "--lifecycle-image", "test.com/some-lifecycle-image"})
					h.AssertNil(t, command.Execute())
				})
			})
			when("a custom image repo is provided with a tag", func() {
				it("uses the provided lifecycle-image and parses it correctly", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithLifecycleImage("test.com/some-lifecycle-image:v1")).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "image", "--lifecycle-image", "test.com/some-lifecycle-image:v1"})
					h.AssertNil(t, command.Execute())
				})
			})
			when("a custom image repo is provided with a digest", func() {
				it("uses the provided lifecycle-image and parses it correctly", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithLifecycleImage("test.com/some-lifecycle-image@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "image", "--lifecycle-image", "test.com/some-lifecycle-image@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"})
					h.AssertNil(t, command.Execute())
				})
			})
		})
		when("an invalid lifecycle-image is provided", func() {
			when("the repo name is invalid", func() {
				it("returns a parse error", func() {
					command.SetArgs([]string{"--builder", "my-builder", "image", "--lifecycle-image", "some-!nv@l!d-image"})
					err := command.Execute()
					h.AssertError(t, err, "could not parse reference: some-!nv@l!d-image")
				})
			})
		})

		when("a lifecycle-image is not provided", func() {
			when("a lifecycle-image is set in the config", func() {
				it("uses the lifecycle-image from the config after parsing it", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithLifecycleImage("index.docker.io/library/some-lifecycle-image:latest")).
						Return(nil)

					cfg := config.Config{LifecycleImage: "some-lifecycle-image"}
					command := commands.Build(logger, cfg, mockClient)

					logger.WantVerbose(true)
					command.SetArgs([]string{"image", "--builder", "my-builder"})
					h.AssertNil(t, command.Execute())
				})
			})
			when("a lifecycle-image is not set in the config", func() {
				it("passes an empty lifecycle image and does not throw an error", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithLifecycleImage("")).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "image"})
					h.AssertNil(t, command.Execute())
				})
			})
		})

		when("env vars are passed as flags", func() {
			var (
				tmpVar   = "tmpVar"
				tmpValue = "tmpKey"
			)

			it.Before(func() {
				h.AssertNil(t, os.Setenv(tmpVar, tmpValue))
			})

			it.After(func() {
				h.AssertNil(t, os.Unsetenv(tmpVar))
			})

			it("sets flag variables", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithEnv(map[string]string{
						"KEY":  "VALUE",
						tmpVar: tmpValue,
					})).
					Return(nil)

				command.SetArgs([]string{"image", "--builder", "my-builder", "--env", "KEY=VALUE", "--env", tmpVar})
				h.AssertNil(t, command.Execute())
			})
		})

		when("build fails", func() {
			it("should show an error", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return(errors.New(""))

				command.SetArgs([]string{"--builder", "my-builder", "image"})
				err := command.Execute()
				h.AssertError(t, err, "failed to build")
			})
		})

		when("user specifies an invalid project descriptor file", func() {
			it("should show an error", func() {
				projectTomlPath := "/incorrect/path/to/project.toml"

				command.SetArgs([]string{"--builder", "my-builder", "--descriptor", projectTomlPath, "image"})
				h.AssertNotNil(t, command.Execute())
			})
		})

		when("parsing project descriptor", func() {
			when("file is valid", func() {
				var projectTomlPath string

				it.Before(func() {
					projectToml, err := ioutil.TempFile("", "project.toml")
					h.AssertNil(t, err)
					defer projectToml.Close()

					projectToml.WriteString(`
[project]
name = "Sample"

[[build.buildpacks]]
id = "example/lua"
version = "1.0"
`)
					projectTomlPath = projectToml.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(projectTomlPath))
				})

				it("should build an image with configuration in descriptor", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithProjectDescriptor(project.Descriptor{
							Project: project.Project{
								Name: "Sample",
							},
							Build: project.Build{
								Buildpacks: []project.Buildpack{{
									ID:      "example/lua",
									Version: "1.0",
								}},
							},
						})).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "--descriptor", projectTomlPath, "image"})
					h.AssertNil(t, command.Execute())
				})
			})
			when("file has a builder specified", func() {
				var projectTomlPath string

				it.Before(func() {
					projectToml, err := ioutil.TempFile("", "project.toml")
					h.AssertNil(t, err)
					defer projectToml.Close()

					projectToml.WriteString(`
[project]
name = "Sample"

[build]
builder = "my-builder"
`)
					projectTomlPath = projectToml.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(projectTomlPath))
				})
				when("a builder is not explicitly passed by the user", func() {
					it("should build an image with configuration in descriptor", func() {
						mockClient.EXPECT().
							Build(gomock.Any(), EqBuildOptionsWithBuilder("my-builder")).
							Return(nil)

						command.SetArgs([]string{"--descriptor", projectTomlPath, "image"})
						h.AssertNil(t, command.Execute())
					})
				})
				when("a builder is explicitly passed by the user", func() {
					it("should build an image with the passed builder flag", func() {
						mockClient.EXPECT().
							Build(gomock.Any(), EqBuildOptionsWithBuilder("flag-builder")).
							Return(nil)

						command.SetArgs([]string{"--builder", "flag-builder", "--descriptor", projectTomlPath, "image"})
						h.AssertNil(t, command.Execute())
					})
				})
			})
			when("file is invalid", func() {
				var projectTomlPath string

				it.Before(func() {
					projectToml, err := ioutil.TempFile("", "project.toml")
					h.AssertNil(t, err)
					defer projectToml.Close()

					projectToml.WriteString("project]")
					projectTomlPath = projectToml.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(projectTomlPath))
				})

				it("should fail to build", func() {
					command.SetArgs([]string{"--builder", "my-builder", "--descriptor", projectTomlPath, "image"})
					h.AssertNotNil(t, command.Execute())
				})
			})

			when("descriptor path is NOT specified", func() {
				when("project.toml exists in source repo", func() {
					it.Before(func() {
						h.AssertNil(t, os.Chdir("testdata"))
					})

					it.After(func() {
						h.AssertNil(t, os.Chdir(".."))
					})

					it("should use project.toml in source repo", func() {
						mockClient.EXPECT().
							Build(gomock.Any(), EqBuildOptionsWithProjectDescriptor(project.Descriptor{
								Project: project.Project{
									Name: "Sample",
								},
								Build: project.Build{
									Buildpacks: []project.Buildpack{{
										ID:      "example/lua",
										Version: "1.0",
									}},
									Env: []project.EnvVar{{
										Name:  "KEY1",
										Value: "VALUE1",
									}},
								},
							})).
							Return(nil)

						command.SetArgs([]string{"--builder", "my-builder", "image"})
						h.AssertNil(t, command.Execute())
					})
				})

				when("project.toml does NOT exist in source repo", func() {
					it("should use empty descriptor", func() {
						mockClient.EXPECT().
							Build(gomock.Any(), EqBuildOptionsWithEnv(map[string]string{})).
							Return(nil)

						command.SetArgs([]string{"--builder", "my-builder", "image"})
						h.AssertNil(t, command.Execute())
					})
				})
			})

			when("descriptor path is specified", func() {
				when("descriptor file exists", func() {
					var projectTomlPath string
					it.Before(func() {
						projectTomlPath = filepath.Join("testdata", "project.toml")
					})

					it("should use specified descriptor", func() {
						mockClient.EXPECT().
							Build(gomock.Any(), EqBuildOptionsWithProjectDescriptor(project.Descriptor{
								Project: project.Project{
									Name: "Sample",
								},
								Build: project.Build{
									Buildpacks: []project.Buildpack{{
										ID:      "example/lua",
										Version: "1.0",
									}},
									Env: []project.EnvVar{{
										Name:  "KEY1",
										Value: "VALUE1",
									}},
								},
							})).
							Return(nil)

						command.SetArgs([]string{"--builder", "my-builder", "--descriptor", projectTomlPath, "image"})
						h.AssertNil(t, command.Execute())
					})
				})

				when("descriptor file does NOT exist in source repo", func() {
					it("should fail with an error message", func() {
						command.SetArgs([]string{"--builder", "my-builder", "--descriptor", "non-existent-path", "image"})
						h.AssertError(t, command.Execute(), "stat project descriptor")
					})
				})
			})
		})

		when("additional tags are specified", func() {
			it("forwards additional tags to lifecycle", func() {
				expectedTags := []string{"additional-tag-1", "additional-tag-2"}
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithAdditionalTags(expectedTags)).
					Return(nil)

				command.SetArgs([]string{"image", "--builder", "my-builder", "--tag", expectedTags[0], "--tag", expectedTags[1]})
				h.AssertNil(t, command.Execute())
			})
		})
	})
}

func EqBuildOptionsWithImage(builder, image string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Builder=%s and Image=%s", builder, image),
		equals: func(o pack.BuildOptions) bool {
			return o.Builder == builder && o.Image == image
		},
	}
}

func EqBuildOptionsDefaultProcess(defaultProc string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Default Process Type=%s", defaultProc),
		equals: func(o pack.BuildOptions) bool {
			return o.DefaultProcessType == defaultProc
		},
	}
}

func EqBuildOptionsWithPullPolicy(policy pubcfg.PullPolicy) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("PullPolicy=%s", policy),
		equals: func(o pack.BuildOptions) bool {
			return o.PullPolicy == policy
		},
	}
}

func EqBuildOptionsWithCacheImage(cacheImage string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("CacheImage=%s", cacheImage),
		equals: func(o pack.BuildOptions) bool {
			return o.CacheImage == cacheImage
		},
	}
}

func EqBuildOptionsWithAssetPackage(assetPackages ...string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("AssetPackages=[%s]", strings.Join(assetPackages, ", ")),
		equals: func(o pack.BuildOptions) bool {
			if len(o.AssetPackages) != len(assetPackages) {
				return false
			}
			for idx := 0; idx < len(assetPackages); idx++ {
				if o.AssetPackages[idx] != assetPackages[idx] {
					return false
				}
			}
			return true
		},
	}
}

func EqBuildOptionsWithLifecycleImage(lifecycleImage string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("LifecycleImage=%s", lifecycleImage),
		equals: func(o pack.BuildOptions) bool {
			return o.LifecycleImage == lifecycleImage
		},
	}
}

func EqBuildOptionsWithNetwork(network string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Network=%s", network),
		equals: func(o pack.BuildOptions) bool {
			return o.ContainerConfig.Network == network
		},
	}
}

func EqBuildOptionsWithBuilder(builder string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Builder=%s", builder),
		equals: func(o pack.BuildOptions) bool {
			return o.Builder == builder
		},
	}
}

func EqBuildOptionsWithTrustedBuilder(trustBuilder bool) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Trust Builder=%t", trustBuilder),
		equals: func(o pack.BuildOptions) bool {
			return o.TrustBuilder == trustBuilder
		},
	}
}

func EqBuildOptionsWithVolumes(volumes []string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Volumes=%s", volumes),
		equals: func(o pack.BuildOptions) bool {
			return reflect.DeepEqual(o.ContainerConfig.Volumes, volumes)
		},
	}
}

func EqBuildOptionsWithAdditionalTags(additionalTags []string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("AdditionalTags=%s", additionalTags),
		equals: func(o pack.BuildOptions) bool {
			return reflect.DeepEqual(o.AdditionalTags, additionalTags)
		},
	}
}

func EqBuildOptionsWithProjectDescriptor(descriptor project.Descriptor) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Descriptor=%s", descriptor),
		equals: func(o pack.BuildOptions) bool {
			return reflect.DeepEqual(o.ProjectDescriptor, descriptor)
		},
	}
}

func EqBuildOptionsWithEnv(env map[string]string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Env=%+v", env),
		equals: func(o pack.BuildOptions) bool {
			for k, v := range o.Env {
				if env[k] != v {
					return false
				}
			}
			for k, v := range env {
				if o.Env[k] != v {
					return false
				}
			}
			return true
		},
	}
}

type buildOptionsMatcher struct {
	equals      func(pack.BuildOptions) bool
	description string
}

func (m buildOptionsMatcher) Matches(x interface{}) bool {
	if b, ok := x.(pack.BuildOptions); ok {
		return m.equals(b)
	}
	return false
}

func (m buildOptionsMatcher) String() string {
	return "is a BuildOptions with " + m.description
}
