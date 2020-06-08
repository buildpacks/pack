// +build acceptance

package components

import (
	"github.com/docker/docker/client"

	h "github.com/buildpacks/pack/testhelpers"
)

type TestBuilder struct {
	name      string
	dockerCli *client.Client
}

func NewTestBuilder(dockerCli *client.Client, name string) TestBuilder {
	return TestBuilder{
		name:      name,
		dockerCli: dockerCli,
	}
}

func (b TestBuilder) Name() string {
	return b.name
}

func (b TestBuilder) Cleanup() {
	h.DockerRmi(b.dockerCli, b.name)
}
