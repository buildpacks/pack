//go:build acceptance
// +build acceptance

package harness

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/buildpacks/pack/acceptance/buildpacks"
	"github.com/buildpacks/pack/acceptance/config"
	"github.com/buildpacks/pack/acceptance/invoke"
	"github.com/buildpacks/pack/acceptance/managers"
	"github.com/buildpacks/pack/pkg/archive"
	h "github.com/buildpacks/pack/testhelpers"
)

func createBuilder(
	t *testing.T,
	assert h.AssertionManager,
	registryConfig *h.TestRegistryConfig,
	imageManager managers.ImageManager,
	dockerCli client.CommonAPIClient,
	pack invoke.PackInvoker,
	lifecycle config.LifecycleAsset,
	buildpackManager buildpacks.BuildpackManager,
	stack config.Stack,
) (string, error) {
	t.Log("creating builder image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir("", "create-test-builder")
	assert.Nil(err)
	defer os.RemoveAll(tmpDir)

	// ARCHIVE BUILDPACKS
	builderBuildpacks := []buildpacks.TestBuildpack{
		buildpacks.Noop,
		buildpacks.Noop2,
		buildpacks.OtherStack,
		buildpacks.ReadEnv,
	}

	packageTomlPath := pack.FixtureManager().TemplateFixtureToFile(
		tmpDir,
		"package.toml",
		map[string]interface{}{
			"OS": imageManager.HostOS(),
		},
	)
	defer os.RemoveAll(packageTomlPath)

	packageImageName := registryConfig.RepoName("simple-layers-package-image-buildpack-" + h.RandString(8))

	packageImageBuildpack := buildpacks.NewPackageImage(
		t,
		pack,
		packageImageName,
		packageTomlPath,
		buildpacks.WithRequiredBuildpacks(buildpacks.SimpleLayers),
	)

	defer imageManager.CleanupImages(packageImageName)

	builderBuildpacks = append(builderBuildpacks, packageImageBuildpack)

	templateMapping := map[string]interface{}{
		"build_image":        stack.BuildImageName,
		"run_image":          stack.RunImage.Name,
		"run_image_mirror":   stack.RunImage.MirrorName,
		"package_image_name": packageImageName,
		"package_id":         "simple/layers",
	}

	buildpackManager.PrepareBuildpacks(tmpDir, builderBuildpacks...)

	// ADD lifecycle
	var lifecycleURI string
	var lifecycleVersion string
	if lifecycle.HasLocation() {
		lifecycleURI = lifecycle.EscapedPath()
		t.Logf("adding lifecycle path '%s' to builder config", lifecycleURI)
		templateMapping["lifecycle_uri"] = lifecycleURI
	} else {
		lifecycleVersion = lifecycle.Version()
		t.Logf("adding lifecycle version '%s' to builder config", lifecycleVersion)
		templateMapping["lifecycle_version"] = lifecycleVersion
	}

	// RENDER builder.toml
	builderConfig := pack.FixtureManager().TemplateVersionedFixtureToFile(
		tmpDir,
		filepath.Join("%s", "builder.toml"),
		pack.SanitizedVersion(),
		"builder.toml",
		templateMapping,
	)

	// NAME BUILDER
	bldr := registryConfig.RepoName("test/builder-" + h.RandString(10))

	// CREATE BUILDER
	output := pack.RunSuccessfully(
		"builder", "create", bldr,
		"-c", builderConfig,
		"--no-color",
	)

	assert.Contains(output, fmt.Sprintf("Successfully created builder image '%s'", bldr))
	assert.Succeeds(h.PushImage(dockerCli, bldr, registryConfig))

	return bldr, nil
}

func createStack(t *testing.T, dockerCli client.CommonAPIClient, registry *h.TestRegistryConfig, imageManager managers.ImageManager, runImageName, buildImageName, testDataDir string) (config.Stack, error) {
	t.Helper()
	t.Log("creating stack images...")

	stackBaseDir := filepath.Join(testDataDir, "mock_stack", imageManager.HostOS())

	_, err := os.Stat(stackBaseDir)
	if err != nil {
		return config.Stack{}, err
	}

	if err := createStackImage(dockerCli, runImageName, filepath.Join(stackBaseDir, "run")); err != nil {
		return config.Stack{}, err
	}
	if err := createStackImage(dockerCli, buildImageName, filepath.Join(stackBaseDir, "build")); err != nil {
		return config.Stack{}, err
	}

	runImageMirror := registry.RepoName(runImageName)
	imageManager.TagImage(runImageName, runImageMirror)
	if err := h.PushImage(dockerCli, runImageMirror, registry); err != nil {
		return config.Stack{}, err
	}

	return config.Stack{
		RunImage: config.RunImage{
			Name:       runImageName,
			MirrorName: runImageMirror,
		},
		BuildImageName: buildImageName,
	}, nil
}

func createStackImage(dockerCli client.CommonAPIClient, repoName string, dir string) error {
	defaultFilterFunc := func(file string) bool { return true }

	ctx := context.Background()
	buildContext := archive.ReadDirAsTar(dir, "/", 0, 0, -1, true, false, defaultFilterFunc)

	return h.CheckImageBuildResult(dockerCli.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:        []string{repoName},
		Remove:      true,
		ForceRemove: true,
	}))
}
