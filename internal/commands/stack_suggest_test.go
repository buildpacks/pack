package commands

import (
	"bytes"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestStacksSuggestCommand(t *testing.T) {
	spec.Run(t, "StacksSuggestCommand", testStacksSuggestCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testStacksSuggestCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command *cobra.Command
		outBuf  bytes.Buffer
	)

	it.Before(func() {
		command = stackSuggest(logging.NewLogWithWriters(&outBuf, &outBuf))
	})

	when("#SuggestStacks", func() {
		it("displays stack information", func() {
			command.SetArgs([]string{})
			h.AssertNil(t, command.Execute())
			h.AssertEq(t, outBuf.String(), `Stacks maintained by the community:

    Stack ID: Deprecation Notice
    Description: Stacks are deprecated in favor of using BuildImages and RunImages directly, but will continue to be supported throughout all of 2023 and 2024 if not longer. Please see our docs for more details- https://buildpacks.io/docs/concepts/components/stack
    Maintainer: CNB
    Build Image: 
    Run Image: 

    Stack ID: heroku-20
    Description: The official Heroku stack based on Ubuntu 20.04
    Maintainer: Heroku
    Build Image: heroku/heroku:20-cnb-build
    Run Image: heroku/heroku:20-cnb

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
