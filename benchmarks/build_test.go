// +build benchmarks

package benchmarks

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	dockerCli "github.com/docker/docker/client"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	cfg "github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

var (
	baseImg     = "some-org/" + h.RandString(10)
	trustedImg  = baseImg + "-trusted-"
	builder     = "cnbs/sample-builder:bionic"
	mockAppPath = filepath.Join("..", "acceptance", "testdata", "mock_app")
)

func BenchmarkBuild(b *testing.B) {
	dockerClient, err := dockerCli.NewClientWithOpts(dockerCli.FromEnv, dockerCli.WithVersion("1.38"))
	if err != nil {
		b.Error(errors.Wrap(err, "creating docker client"))
	}

	if err = h.PullImageWithAuth(dockerClient, builder, ""); err != nil {
		b.Error(errors.Wrapf(err, "pulling builder %s", builder))
	}

	cmd := createCmd(b, dockerClient)

	b.Run("with Untrusted Builder", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// perform the operation we're analyzing
			cmd.SetArgs([]string{fmt.Sprintf("%s%d", baseImg, i), "-p", mockAppPath, "-B", builder})
			if err = cmd.Execute(); err != nil {
				b.Error(errors.Wrapf(err, "running build #%d", i))
			}
		}
	})

	b.Run("with Trusted Builder", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// perform the operation we're analyzing
			cmd.SetArgs([]string{fmt.Sprintf("%s%d", trustedImg, i), "-p", mockAppPath, "-B", builder, "--trust-builder"})
			if err = cmd.Execute(); err != nil {
				b.Error(errors.Wrapf(err, "running build #%d", i))
			}
		}
	})

	// Cleanup
	for i := 0; i < b.N; i++ {
		if err = h.DockerRmi(dockerClient, fmt.Sprintf("%s%d", baseImg, i)); err != nil {
			b.Error(errors.Wrapf(err, "deleting image #%d", i))
		}

		if err = h.DockerRmi(dockerClient, fmt.Sprintf("%s%d", trustedImg, i)); err != nil {
			b.Error(errors.Wrapf(err, "deleting image #%d", i))
		}
	}

	if err = h.DockerRmi(dockerClient, builder); err != nil {
		b.Error(errors.Wrapf(err, "deleting builder %s", builder))
	}
}

func createCmd(b *testing.B, docker *dockerCli.Client) *cobra.Command {
	outBuf := bytes.Buffer{}
	logger := logging.NewLogWithWriters(&outBuf, &outBuf)
	packClient, err := client.NewClient(client.WithLogger(logger), client.WithDockerClient(docker), client.WithExperimental(true))
	if err != nil {
		b.Error(errors.Wrap(err, "creating packClient"))
	}
	return commands.Build(logger, cfg.Config{}, packClient)
}
