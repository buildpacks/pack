package commands_test

import (
	"bytes"
	"testing"

	"github.com/buildpacks/pack/cmd"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"
)

func TestVersion(t *testing.T) {
	spec.Run(t, "Commands", testVersionCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testVersionCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command *cobra.Command
		outBuf  bytes.Buffer
	)

	it.Before(func() {
		command = commands.Version(logging.NewLogWithWriters(&outBuf, &outBuf), cmd.Version)
	})

	when("#Version", func() {
		it("returns version", func() {
			command.SetArgs([]string{})
			h.AssertNil(t, command.Execute())
			h.AssertEq(t, outBuf.String(), cmd.Version)
		})
	})
}
