package commands_test

import (
	"bytes"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/commands"
	"github.com/buildpack/pack/internal/mocks"
	h "github.com/buildpack/pack/testhelpers"
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
		command = commands.SuggestStacks(mocks.NewMockLogger(&outBuf))
	})

	when("#SuggestStacks", func() {
		it("displays stack information", func() {
			command.SetArgs([]string{})
			h.AssertNil(t, command.Execute())
			h.AssertEq(t, outBuf.String(), `
Stacks maintained by the Cloud Native Buildpacks project:

    Stack ID: io.buildpacks.stacks.bionic
    Description: Minimal Ubuntu 18.04 stack
    Maintainer: Cloud Native Buildpacks
    Build Image: cnbs/build:bionic
    Run Image: cnbs/run:bionic

Stacks maintained by the community:

    Stack ID: heroku-18
    Description: The official Heroku stack based on Ubuntu 18.04
    Maintainer: Heroku
    Build Image: heroku/pack:18-build
    Run Image: heroku/pack:18

    Stack ID: org.cloudfoundry.stacks.cflinuxfs3
    Description: The official Cloud Foundry stack based on Ubuntu 18.04
    Maintainer: Cloud Foundry
    Build Image: cfbuildpacks/cflinuxfs3-cnb-experimental:build
    Run Image: cfbuildpacks/cflinuxfs3-cnb-experimental:run
`)
		})
	})
}
