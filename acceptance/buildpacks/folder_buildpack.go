//go:build acceptance
// +build acceptance

package buildpacks

import (
	"fmt"
	"os"
	"path/filepath"

	h "github.com/buildpacks/pack/testhelpers"
)

type folderBuildpack struct {
	name string
}

func (f folderBuildpack) Prepare(sourceDir, destination string) error {
	sourceBuildpack := filepath.Join(sourceDir, f.name)
	info, err := os.Stat(sourceBuildpack)
	if err != nil {
		return fmt.Errorf("retrieving buildpack folder info for folder: %s: %w", sourceBuildpack, err)
	}

	destinationBuildpack := filepath.Join(destination, f.name)
	err = os.Mkdir(filepath.Join(destinationBuildpack), info.Mode())
	if err != nil {
		return fmt.Errorf("creating temp buildpack folder in: %s: %w", destinationBuildpack, err)
	}

	err = h.RecursiveCopyE(filepath.Join(sourceDir, f.name), destinationBuildpack)
	if err != nil {
		return fmt.Errorf("copying folder buildpack %s: %w", f.name, err)
	}

	return nil
}

func (f folderBuildpack) FullPathIn(parentFolder string) string {
	return filepath.Join(parentFolder, f.name)
}

var (
	FolderNotInBuilder       = folderBuildpack{name: "not-in-builder-buildpack"}
	FolderSimpleLayersParent = folderBuildpack{name: "simple-layers-parent-buildpack"}
	FolderSimpleLayers       = folderBuildpack{name: "simple-layers-buildpack"}
)
