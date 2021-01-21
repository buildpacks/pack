package commands_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ilogging "github.com/buildpacks/pack/internal/logging"
)

func TestBuildpackCreateCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "BuildpackCreateCommand", testBuildpackCreateCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildpackCreateCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         *ilogging.LogWithWriters
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
		tmpDir         string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "build-test")
		h.AssertNil(t, err)

		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)

		command = commands.BuildpackCreate(logger, mockClient)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("BuildpackCreate#Execute", func() {
		it("uses the args to generate artifacts", func() {
			mockClient.EXPECT().CreateBuildpack(gomock.Any(), pack.CreateBuildpackOptions{
				ID:   "example/some-cnb",
				Path: tmpDir,
				Stacks: []dist.Stack{{
					ID:     "io.buildpacks.stacks.bionic",
					Mixins: []string{},
				}},
			}).Return(nil).MaxTimes(1)

			command.SetArgs([]string{"--path", tmpDir, "example/some-cnb"})

			err := command.Execute()
			h.AssertNil(t, err)
		})
	})
}
