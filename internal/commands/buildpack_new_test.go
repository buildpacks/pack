package commands_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuildpackNewCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "BuildpackNewCommand", testBuildpackNewCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildpackNewCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         *logging.LogWithWriters
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
		tmpDir         string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "build-test")
		h.AssertNil(t, err)

		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)

		command = commands.BuildpackNew(logger, mockClient)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("BuildpackNew#Execute", func() {
		it("uses the args to generate artifacts", func() {
			mockClient.EXPECT().NewBuildpack(gomock.Any(), client.NewBuildpackOptions{
				API:     "0.6",
				ID:      "example/some-cnb",
				Path:    filepath.Join(tmpDir, "some-cnb"),
				Version: "1.0.0",
				Stacks: []dist.Stack{{
					ID:     "io.buildpacks.stacks.bionic",
					Mixins: []string{},
				}},
			}).Return(nil).MaxTimes(1)

			path := filepath.Join(tmpDir, "some-cnb")
			command.SetArgs([]string{"--path", path, "example/some-cnb"})

			err := command.Execute()
			h.AssertNil(t, err)
		})

		it("stops if the directory already exists", func() {
			err := os.MkdirAll(tmpDir, 0600)
			h.AssertNil(t, err)

			command.SetArgs([]string{"--path", tmpDir, "example/some-cnb"})
			err = command.Execute()
			h.AssertNotNil(t, err)
			h.AssertContains(t, outBuf.String(), "ERROR: directory")
		})
	})
}
