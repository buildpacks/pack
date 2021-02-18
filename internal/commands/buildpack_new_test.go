package commands_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/cobra"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"
	ilogging "github.com/buildpacks/pack/internal/logging"
)

func TestBuildpackNewCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "BuildpackNewCommand", testBuildpackNewCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildpackNewCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.BuildpackNew(logger, mockClient)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("BuildpackNew#Execute", func() {
		it("uses the args to generate artifacts", func() {
			mockClient.EXPECT().NewBuildpack(gomock.Any(), pack.NewBuildpackOptions{
				ID:   "example/some-cnb",
				Path: filepath.Join(tmpDir, "some-cnb"),
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
