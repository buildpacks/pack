package builder_test

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands/builder"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuilderCommand(t *testing.T) {
	spec.Run(t, "BuilderCommand", testBuilderCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilderCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		cmd    *cobra.Command
		logger logging.Logger
		outBuf bytes.Buffer
	)

	it.Before(func() {
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		mockController := gomock.NewController(t)
		mockClient := testmocks.NewMockPackClient(mockController)
		cmd = builder.NewBuilderCommand(logger, config.Config{}, mockClient)
		cmd.SetOut(logging.GetWriterForLevel(logger, logging.InfoLevel))
	})

	when("builder", func() {
		it("prints help text", func() {
			cmd.SetArgs([]string{})
			h.AssertNil(t, cmd.Execute())
			output := outBuf.String()
			h.AssertContains(t, output, "Interact with builders")
			h.AssertContains(t, output, "Usage:")
			for _, command := range []string{"create", "suggest"} {
				h.AssertContains(t, output, command)
				h.AssertNotContains(t, output, command+"-builder")
			}
		})
	})
}
