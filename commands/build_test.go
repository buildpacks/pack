package commands_test

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/commands"
	cmdmocks "github.com/buildpack/pack/commands/mocks"
	"github.com/buildpack/pack/config"
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
		mockClient     *cmdmocks.MockPackClient
		cfg            config.Config
	)

	it.Before(func() {
		logger = fakes.NewFakeLogger(&outBuf)
		cfg = config.Config{}
		mockController = gomock.NewController(t)
		mockClient = cmdmocks.NewMockPackClient(mockController)

		command = commands.Build(logger, cfg, mockClient)
	})

	when("#BuildCommand", func() {
		when("no env file is provided", func() {
			it("builds an image with a builder", func() {
				mockClient.EXPECT().Build(gomock.Any(), pack.BuildOptions{
					Builder:           "my-builder",
					Image:             "image",
					AdditionalMirrors: map[string][]string{},
					Env:               map[string]string{},
				}).Return(nil)

				command.SetArgs([]string{"--builder", "my-builder", "image"})
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

			it("builds an image with a builder", func() {
				mockClient.EXPECT().Build(gomock.Any(), pack.BuildOptions{
					Builder:           "my-builder",
					Image:             "image",
					AdditionalMirrors: map[string][]string{},
					Env: map[string]string{
						"KEY": "VALUE",
					},
				}).Return(nil)

				command.SetArgs([]string{"--builder", "my-builder", "image", "--env-file", envPath})
				h.AssertNil(t, command.Execute())
			})
		})

		when("two env files are provided", func() {
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

			it("builds an image with a builder", func() {
				mockClient.EXPECT().Build(gomock.Any(), pack.BuildOptions{
					Builder:           "my-builder",
					Image:             "image",
					AdditionalMirrors: map[string][]string{},
					Env: map[string]string{
						"KEY1": "VALUE1",
						"KEY2": "VALUE2",
					},
				}).Return(nil)

				command.SetArgs([]string{"--builder", "my-builder", "image", "--env-file", envPath1, "--env-file", envPath2})
				h.AssertNil(t, command.Execute())
			})
		})
	})
}
