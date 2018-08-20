package pack

import (
	"os"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/packs"
)

func analyzer(group lifecycle.BuildpackGroup, launchDir, repoName string, useDaemon bool) error {
	origImage, err := readImage(repoName, useDaemon)
	if err != nil {
		return err
	}

	if origImage == nil {
		// no previous image to analyze
		return nil
	}

	analyzer := &lifecycle.Analyzer{
		Buildpacks: group.Buildpacks,
		Out:        os.Stdout,
		Err:        os.Stderr,
	}
	err = analyzer.Analyze(
		launchDir,
		origImage,
	)
	if err != nil {
		return packs.FailErrCode(err, packs.CodeFailedBuild)
	}

	return nil
}
