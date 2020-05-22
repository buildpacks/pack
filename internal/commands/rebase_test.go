package commands_test

import (
	"bytes"
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

func TestRebase(t *testing.T) {
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
		when("image name is provided", func() {
			var (
				runImage    string
				testMirror1 string
				testMirror2 string
			)
			it.Before(func() {
				runImage = "test/image"
				testMirror1 = "example.com/some/run1"
				testMirror2 = "example.com/some/run2"

				cfg.RunImages = []config.RunImage{{
					Image:   runImage,
					Mirrors: []string{testMirror1, testMirror2},
				}}
				command = commands.Rebase(logger, cfg, mockClient)
			})

			it("works", func() {
				repoName := "test/repo-image"
				opts := pack.RebaseOptions{
					RepoName: repoName,
					Publish:  false,
					SkipPull: false,
					RunImage: "",
					AdditionalMirrors: map[string][]string{
						runImage: {testMirror1, testMirror2},
					},
				}

				mockClient.EXPECT().
					Build(gomock.Any(), opts).
					Return(nil)

				command.SetArgs([]string{repoName})
				h.AssertNil(t, command.Execute())
			})
		})
	})
}
