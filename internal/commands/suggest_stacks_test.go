package commands_test

import (
	"bytes"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestSuggestStacks(t *testing.T) {
	spec.Run(t, "Commands", testSuggestStacksCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testSuggestStacksCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command *cobra.Command
		outBuf  bytes.Buffer
	)

	it.Before(func() {
		command = commands.SuggestStacks(logging.NewLogWithWriters(&outBuf, &outBuf))
	})

	when("#SuggestStacks", func() {
		it("displays stack information", func() {
			command.SetArgs([]string{})
			h.AssertNil(t, command.Execute())
			h.AssertEq(t, outBuf.String(), `Stacks maintained by the community:

    Stack ID: heroku-18
    Description: The official Heroku stack based on Ubuntu 18.04
    Maintainer: Heroku
    Build Image: heroku/pack:18-build
    Run Image: heroku/pack:18

    Stack ID: io.buildpacks.stacks.bionic
    Description: A minimal Cloud Foundry stack based on Ubuntu 18.04
    Maintainer: Cloud Foundry
    Build Image: cloudfoundry/build:base-cnb
    Run Image: cloudfoundry/run:base-cnb

    Stack ID: org.cloudfoundry.stacks.cflinuxfs3
    Description: A large Cloud Foundry stack based on Ubuntu 18.04
    Maintainer: Cloud Foundry
    Build Image: cloudfoundry/build:full-cnb
    Run Image: cloudfoundry/run:full-cnb

    Stack ID: org.cloudfoundry.stacks.tiny
    Description: A tiny Cloud Foundry stack based on Ubuntu 18.04, similar to distroless
    Maintainer: Cloud Foundry
    Build Image: cloudfoundry/build:tiny-cnb
    Run Image: cloudfoundry/run:tiny-cnb
`)
		})
	})
}
