package commands_test

import (
	"bytes"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/commands"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestSuggestBuilders(t *testing.T) {
	spec.Run(t, "Commands", testSuggestBuildersCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testSuggestBuildersCommand(t *testing.T, when spec.G, it spec.S) {

	var (
		command *cobra.Command
		logger  *logging.Logger
		outBuf  bytes.Buffer
	)

	it.Before(func() {
		logger = logging.NewLogger(&outBuf, &outBuf, false, false)
		command = commands.SuggestBuilders(logger)
	})

	when("#SuggestBuilders", func() {
		it("display suggested builders", func() {
			command.SetArgs([]string{})
			h.AssertNil(t, command.Execute())
			h.AssertContains(t, outBuf.String(), "Suggested builders:")
			h.AssertContains(t, outBuf.String(), "cloudfoundry/cnb:bionic")
		})
	})
}
