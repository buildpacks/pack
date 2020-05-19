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
