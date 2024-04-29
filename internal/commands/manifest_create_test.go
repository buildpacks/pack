package commands_test

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/v1/types"
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

func TestManifestCreateCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "Commands", testManifestCreateCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testManifestCreateCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         *logging.LogWithWriters
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
	)

	it.Before(func() {
		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)

		command = commands.ManifestCreate(logger, mockClient)
	})

	when("valid arguments", func() {
		it.Before(func() {
			mockClient.
				EXPECT().
				CreateManifest(gomock.Any(),
					client.CreateManifestOptions{
						IndexRepoName: "some-index",
						RepoNames:     []string{"some-manifest"},
						Format:        types.DockerManifestList,
						Insecure:      true,
						Publish:       true,
					},
				).
				AnyTimes().
				Return(nil)
		})

		it("should annotate images with given flags", func() {
			command.SetArgs([]string{
				"some-index", "some-manifest",
				"--format", "docker",
				"--insecure", "--publish",
			})
			h.AssertNil(t, command.Execute())
			h.AssertEq(t, outBuf.String(), "")
		})
	})

	when("invalid arguments", func() {
		when("invalid media type", func() {
			var format string
			it.Before(func() {
				format = "invalid"
				mockClient.
					EXPECT().
					CreateManifest(gomock.Any(), gomock.Any()).
					AnyTimes().
					Return(nil)
			})

			it("error a message", func() {
				command.SetArgs([]string{
					"some-index", "some-manifest",
					"--format", format,
				})
				h.AssertNotNil(t, command.Execute())
			})
		})
	})

	when("help is invoke", func() {
		it.Before(func() {
			mockClient.
				EXPECT().
				CreateManifest(gomock.Any(), gomock.Any()).
				AnyTimes().
				Return(nil)
		})

		it("should have help flag", func() {
			command.SetArgs([]string{"--help"})
			h.AssertNilE(t, command.Execute())
			h.AssertEq(t, outBuf.String(), "")
		})
	})
}
