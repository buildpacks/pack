package commands_test

import (
	"bytes"
	"testing"

	"github.com/buildpacks/pack"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestRegisterBuildpackCommand(t *testing.T) {
	spec.Run(t, "Commands", testRegisterBuildpackCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRegisterBuildpackCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.RegisterBuildpack(logger, cfg, mockClient)
	})

	it.After(func() {
	})

	when("#RegisterBuildpackCommand", func() {
		when("no image is provided", func() {
			it("fails to run", func() {
				err := command.Execute()
				h.AssertError(t, err, "accepts 1 arg")
			})
		})

		when("image name is provided", func() {
			var (
				buildpackImage           string
				buildpackDefaultRegistry string
			)

			it.Before(func() {
				buildpackImage = "test/image"
				buildpackDefaultRegistry = "https://default-regisry.com/test"
				cfg.DefaultRegistry = buildpackDefaultRegistry

				command = commands.RegisterBuildpack(logger, cfg, mockClient)
			})

			it("should work for required args", func() {
				opts := pack.RegisterBuildpackOptions{
					BuildpackageURL:   buildpackImage,
					BuildpackRegistry: buildpackDefaultRegistry,
				}

				mockClient.EXPECT().
					RegisterBuildpack(gomock.Any(), opts).
					Return(nil)

				command.SetArgs([]string{buildpackImage})
				h.AssertNil(t, command.Execute())
			})

			it("should support buildpack-registry flag", func() {
				buildpackRegistry := "https://registry.com/test"
				opts := pack.RegisterBuildpackOptions{
					BuildpackageURL:   buildpackImage,
					BuildpackRegistry: buildpackRegistry,
				}

				mockClient.EXPECT().
					RegisterBuildpack(gomock.Any(), opts).
					Return(nil)

				command.SetArgs([]string{buildpackImage, "--buildpack-registry", buildpackRegistry})
				h.AssertNil(t, command.Execute())
			})
		})
	})
}
