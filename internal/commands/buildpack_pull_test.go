package commands_test

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	"github.com/buildpacks/pack/pkg/client"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestPullBuildpackCommand(t *testing.T) {
	spec.Run(t, "PullBuildpackCommand", testPullBuildpackCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPullBuildpackCommand(t *testing.T, when spec.G, it spec.S) {
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
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)
		cfg = config.Config{}

		command = commands.BuildpackPull(logger, cfg, mockClient)
	})

	when("#BuildpackPullCommand", func() {
		when("no buildpack is provided", func() {
			it("fails to run", func() {
				err := command.Execute()
				h.AssertError(t, err, "accepts 1 arg")
			})
		})

		when("buildpack uri is provided", func() {
			it("should work for required args", func() {
				buildpackImage := "buildpack/image"
				opts := client.PullBuildpackOptions{
					URI:          buildpackImage,
					RegistryName: "official",
				}

				mockClient.EXPECT().
					PullBuildpack(gomock.Any(), opts).
					Return(nil)

				command.SetArgs([]string{buildpackImage})
				h.AssertNil(t, command.Execute())
			})
		})
	})
}
