package commands_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuildpackCreateCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "BuildpackCreateCommand", testBuildpackCreateCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildpackCreateCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         *logging.LogWithWriters
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
		tmpDir         string
	)

	it.Before(func() {
		tmpDir = t.TempDir()
		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)

		command = commands.BuildpackCreate(logger, mockClient)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("BuildpackCreate#Execute", func() {
		it("uses the args to generate artifacts", func() {
			mockClient.EXPECT().CreateBuildpack(gomock.Any(), client.CreateBuildpackOptions{
				Template: "testdata",
				SubPath:  "create",
				Arguments: map[string]string{
					"Test": "Quack",
				},
			}).Return(nil).MaxTimes(1)

			command.SetArgs([]string{
				"--template", "testdata",
				"--sub-path", "create",
				"--arg", "Test=Quack",
			})

			err := command.Execute()
			h.AssertNil(t, err)
		})
	})
}
