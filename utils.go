package pack

import (
	"github.com/buildpack/packs"
	"github.com/buildpack/packs/img"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func readImage(repoName string, useDaemon bool) (v1.Image, error) {
	newRepoStore := img.NewRegistry
	if useDaemon {
		newRepoStore = img.NewDaemon
	}
	repoStore, err := newRepoStore(repoName)
	if err != nil {
		return nil, packs.FailErr(err, "access", repoName)
	}

	origImage, err := repoStore.Image()
	if err != nil {
		// Assume error is due to non-existent image
		return nil, nil
	}
	if _, err := origImage.RawManifest(); err != nil {
		// Assume error is due to non-existent image
		// This is necessary for registries
		return nil, nil
	}

	return origImage, nil
}
