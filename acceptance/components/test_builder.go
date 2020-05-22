// +build acceptance

package components

import (
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/docker/docker/client"
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
