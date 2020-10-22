package commands_test

import (
	"bytes"
	"testing"

	"github.com/heroku/color"

	pubcfg "github.com/buildpacks/pack/config"

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

func TestRebaseCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testRebaseCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRebaseCommand(t *testing.T, when spec.G, it spec.S) {
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

		command = commands.Rebase(logger, cfg, mockClient)
	})

	when("#RebaseCommand", func() {
		when("no image is provided", func() {
			it("fails to run", func() {
				err := command.Execute()
				h.AssertError(t, err, "accepts 1 arg")
			})
		})

		when("image name is provided", func() {
			var (
				repoName string
				opts     pack.RebaseOptions
			)
			it.Before(func() {
				runImage := "test/image"
				testMirror1 := "example.com/some/run1"
				testMirror2 := "example.com/some/run2"

				cfg.RunImages = []config.RunImage{{
					Image:   runImage,
					Mirrors: []string{testMirror1, testMirror2},
				}}
				command = commands.Rebase(logger, cfg, mockClient)

				repoName = "test/repo-image"
				opts = pack.RebaseOptions{
					RepoName:   repoName,
					Publish:    false,
					PullPolicy: pubcfg.PullAlways,
					RunImage:   "",
					AdditionalMirrors: map[string][]string{
						runImage: {testMirror1, testMirror2},
					},
				}
			})

			it("works", func() {
				mockClient.EXPECT().
					Rebase(gomock.Any(), opts).
					Return(nil)

				command.SetArgs([]string{repoName})
				h.AssertNil(t, command.Execute())
			})

			when("--pull-policy never", func() {
				it("works", func() {
					opts.PullPolicy = pubcfg.PullNever
					mockClient.EXPECT().
						Rebase(gomock.Any(), opts).
						Return(nil)

					command.SetArgs([]string{repoName, "--pull-policy", "never"})
					h.AssertNil(t, command.Execute())
				})
			})

			when("--pull-policy unknown-policy", func() {
				it("fails to run", func() {
					command.SetArgs([]string{repoName, "--pull-policy", "unknown-policy"})
					h.AssertError(t, command.Execute(), "parsing pull policy")
				})
			})
		})
	})
}
