package pack

import (
	"context"
	"encoding/json"
	"os"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/packs"
	dockercli "github.com/docker/docker/client"
)

func analyzer(group lifecycle.BuildpackGroup, workspaceDir, repoName string, useDaemon bool) error {
	analyzer := &lifecycle.Analyzer{
		Buildpacks: group.Buildpacks,
		Out:        os.Stdout,
		Err:        os.Stderr,
	}

	if useDaemon {
		cli, err := dockercli.NewEnvClient()
		if err != nil {
			return packs.FailErrCode(err, packs.CodeFailedBuild)
		}
		i, _, err := cli.ImageInspectWithRaw(context.Background(), repoName)
		if err != nil {
			if dockercli.IsErrNotFound(err) {
				// images does not already exist, skip analyze
				return nil
			}
			return packs.FailErrCode(err, packs.CodeFailedBuild)
		}
		var config lifecycle.AppImageMetadata
		if err := json.Unmarshal([]byte(i.Config.Labels["sh.packs.build"]), &config); err != nil {
			return packs.FailErrCode(err, packs.CodeFailedBuild)
		}
		if err := analyzer.AnalyzeConfig(workspaceDir, config); err != nil {
			return packs.FailErrCode(err, packs.CodeFailedBuild)
		}
	} else {
		origImage, err := readImage(repoName, useDaemon)
		if err != nil {
			return err
		}

		if origImage == nil {
			// no previous image to analyze
			return nil
		}

		if err := analyzer.Analyze(workspaceDir, origImage); err != nil {
			return packs.FailErrCode(err, packs.CodeFailedBuild)
		}
	}

	return nil
}
