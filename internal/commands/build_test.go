package commands_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/internal/commands"
	"github.com/buildpack/pack/internal/commands/testmocks"
	"github.com/buildpack/pack/internal/config"
	"github.com/buildpack/pack/internal/fakes"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
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
		logger = fakes.NewFakeLogger(&outBuf)
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

func EqBuildOptionsWithNetwork(network string) gomock.Matcher {
	return buildOptionsMatcher{
		description: fmt.Sprintf("Network=%s", network),
		equals: func(o pack.BuildOptions) bool {
			return o.ContainerConfig.Network == network
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
