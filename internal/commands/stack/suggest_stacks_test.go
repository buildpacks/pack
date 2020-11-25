package stack_test

import (
	"bytes"
	"testing"

	"github.com/heroku/color"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands/stack"
	"github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestSuggestStacksCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Commands", testSuggestStacksCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testSuggestStacksCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command *cobra.Command
		outBuf  bytes.Buffer
	)

	it.Before(func() {
		command = stack.SuggestStacks(logging.NewLogWithWriters(&outBuf, &outBuf))
	})

	when("#SuggestStacks", func() {
		it("displays stack information", func() {
			command.SetArgs([]string{})
			h.AssertNil(t, command.Execute())
			h.AssertEq(t, outBuf.String(), `Warning: Command 'pack suggest-stacks' has been deprecated, please use 'pack stack suggest' instead
Stacks maintained by the community:

    Stack ID: heroku-18
    Description: The official Heroku stack based on Ubuntu 18.04
    Maintainer: Heroku
    Build Image: heroku/pack:18-build
    Run Image: heroku/pack:18

    Stack ID: io.buildpacks.stacks.bionic
    Description: A minimal Paketo stack based on Ubuntu 18.04
    Maintainer: Paketo Project
    Build Image: paketobuildpacks/build:base-cnb
    Run Image: paketobuildpacks/run:base-cnb

    Stack ID: io.buildpacks.stacks.bionic
    Description: A large Paketo stack based on Ubuntu 18.04
    Maintainer: Paketo Project
    Build Image: paketobuildpacks/build:full-cnb
    Run Image: paketobuildpacks/run:full-cnb

    Stack ID: io.paketo.stacks.tiny
    Description: A tiny Paketo stack based on Ubuntu 18.04, similar to distroless
    Maintainer: Paketo Project
    Build Image: paketobuildpacks/build:tiny-cnb
    Run Image: paketobuildpacks/run:tiny-cnb
`)
		})
	})
}
