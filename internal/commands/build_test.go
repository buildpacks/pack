package commands_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuildCommand(t *testing.T) {
	spec.Run(t, "Commands", testBuildCommand, spec.Random(), spec.Report(report.Terminal{}))
}

func testBuildCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         logging.Logger
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
				mockClient.EXPECT().InspectBuilder(gomock.Any(), false).Return(&pack.BuilderInfo{
					Description: "",
				}, nil).AnyTimes()

				command.SetArgs([]string{"image"})
				err := command.Execute()
				h.AssertError(t, err, commands.NewSoftError().Error())
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

				command.SetArgs([]string{"-B", "my-builder", "image"})
				h.AssertNil(t, command.Execute())
			})
			when("the builder is trusted", func() {
				it("sets the trust builder option", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithTrustedBuilder(true)).
						Return(nil)

					cfg := config.Config{TrustedBuilders: []config.TrustedBuilder{{Name: "my-builder"}}}
					command := commands.Build(logger, cfg, mockClient)

					command.SetArgs([]string{"image", "--builder", "my-builder"})
					h.AssertNil(t, command.Execute())
				})
			})

			when("the builder is suggested", func() {
				it("sets the trust builder option", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithTrustedBuilder(true)).
						Return(nil)

					command.SetArgs([]string{"image", "--builder", "heroku/buildpacks:18"})
					h.AssertNil(t, command.Execute())
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

		when("volume mounts are specified", func() {
			it("mounts the volumes", func() {
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithVolumes([]string{"a:b", "c:d"})).
					Return(nil)

				command.SetArgs([]string{"image", "--builder", "my-builder", "--volume", "a:b", "--volume", "c:d"})
				h.AssertNil(t, command.Execute())
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

				it("succesfully builds", func() {
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
				mockClient.EXPECT().
					Build(gomock.Any(), EqBuildOptionsWithImage("my-builder", "image")).
					Return(nil)

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
						Build(gomock.Any(), EqBuildOptionsWithBuildpacks([]string{
							"example/lua@1.0",
						})).
						Return(nil)

					command.SetArgs([]string{"--builder", "my-builder", "--descriptor", projectTomlPath, "image"})
					h.AssertNil(t, command.Execute())
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
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithImage("my-builder", "image")).
						Return(nil)

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
							Build(gomock.Any(), EqBuildOptionsWithEnv(map[string]string{
								"KEY1": "VALUE1",
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
							Build(gomock.Any(), EqBuildOptionsWithEnv(map[string]string{
								"KEY1": "VALUE1",
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

			when("descriptor buildpack has uri", func() {
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
uri = "https://www.test.tgz"
`)
					projectTomlPath = projectToml.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(projectTomlPath))
				})

				it("should build an image with configuration in descriptor", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithBuildpacks([]string{
							"https://www.test.tgz",
						})).
						Return(nil)

					command.SetArgs([]string{"image", "--builder", "my-builder", "--descriptor", projectTomlPath})
					h.AssertNil(t, command.Execute())
				})
			})

			when("descriptor buildpack has malformed uri", func() {
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
uri = "://bad-uri"
`)
					projectTomlPath = projectToml.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(projectTomlPath))
				})

				it("should build an image with configuration in descriptor", func() {
					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithBuildpacks([]string{
							"https://www.test.tgz",
						})).
						Return(nil)

					command.SetArgs([]string{"image", "--builder", "my-builder", "--descriptor", projectTomlPath})
					err := command.Execute()
					h.AssertError(t, err, "parse")
				})
			})

			when("descriptor has exclude", func() {
				var projectTomlPath string

				it.Before(func() {
					projectToml, err := ioutil.TempFile("", "project.toml")
					h.AssertNil(t, err)
					defer projectToml.Close()

					projectToml.WriteString(`
[project]
name = "Sample"

[build]
exclude = [ "*.jar" ]
`)
					projectTomlPath = projectToml.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(projectTomlPath))
				})

				it("should return appropriate fileFilter function", func() {
					mockFilter := func(string) bool {
						return false
					}

					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithFileFilter(mockFilter, "test.jar")).
						Return(nil)

					command.SetArgs([]string{"image", "--builder", "my-builder", "--descriptor", projectTomlPath})
					h.AssertNil(t, command.Execute())
				})
			})

			when("descriptor has include", func() {
				var projectTomlPath string
				it.Before(func() {
					projectToml, err := ioutil.TempFile("", "project.toml")
					h.AssertNil(t, err)
					defer projectToml.Close()

					projectToml.WriteString(`
[project]
name = "Sample"

[build]
include = [ "*.jar" ]
`)
					projectTomlPath = projectToml.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(projectTomlPath))
				})

				it("should return appropriate fileFilter function", func() {
					mockFilter := func(string) bool {
						return true
					}

					mockClient.EXPECT().
						Build(gomock.Any(), EqBuildOptionsWithFileFilter(mockFilter, "test.jar")).
						Return(nil)

					command.SetArgs([]string{"image", "--builder", "my-builder", "--descriptor", projectTomlPath})
					h.AssertNil(t, command.Execute())
				})
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

func EqBuildOptionsWithNetwork(network string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Network=%s", network),
		equals: func(o pack.BuildOptions) bool {
			return o.ContainerConfig.Network == network
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

func EqBuildOptionsWithFileFilter(fileFilter func(string) bool, fileName string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("File Filter=%p", fileFilter),
		equals: func(o pack.BuildOptions) bool {
			return o.FileFilter(fileName) == fileFilter(fileName)
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

func EqBuildOptionsWithBuildpacks(buildpacks []string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Buildpacks=%+v", buildpacks),
		equals: func(o pack.BuildOptions) bool {
			for _, bp := range o.Buildpacks {
				if !contains(buildpacks, bp) {
					return false
				}
			}
			for _, bp := range buildpacks {
				if !contains(o.Buildpacks, bp) {
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

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}
