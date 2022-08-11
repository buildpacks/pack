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
	pack *invoke.PackInvoker,
	lifecycle config.LifecycleAsset,
	buildpackManager buildpacks.BuildpackManager,
	runImageMirror string,
) (string, error) {
	t.Log("creating builder image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir("", "create-test-builder")
	assert.Nil(err)
	defer os.RemoveAll(tmpDir)

	templateMapping := map[string]interface{}{
		"run_image_mirror": runImageMirror,
	}

	// ARCHIVE BUILDPACKS
	builderBuildpacks := []buildpacks.TestBuildpack{
		buildpacks.Noop,
		buildpacks.Noop2,
		buildpacks.OtherStack,
		buildpacks.ReadEnv,
	}

	packageTomlPath := generatePackageTomlWithOS(t, assert, pack, tmpDir, "package.toml", imageManager.HostOS())
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

	templateMapping["package_image_name"] = packageImageName
	templateMapping["package_id"] = "simple/layers"

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
	configFileName := "builder.toml"

	builderConfigFile, err := ioutil.TempFile(tmpDir, "builder.toml")
	assert.Nil(err)

	pack.FixtureManager().TemplateFixtureToFile(
		configFileName,
		builderConfigFile,
		templateMapping,
	)

	err = builderConfigFile.Close()
	assert.Nil(err)

	// NAME BUILDER
	bldr := registryConfig.RepoName("test/builder-" + h.RandString(10))

	// CREATE BUILDER
	output := pack.RunSuccessfully(
		"builder", "create", bldr,
		"-c", builderConfigFile.Name(),
		"--no-color",
	)

	assert.Contains(output, fmt.Sprintf("Successfully created builder image '%s'", bldr))
	assert.Succeeds(h.PushImage(dockerCli, bldr, registryConfig))

	return bldr, nil
}

func generatePackageTomlWithOS(
	t *testing.T,
	assert h.AssertionManager,
	pack *invoke.PackInvoker,
	tmpDir string,
	fixtureName string,
	platform_os string,
) string {
	t.Helper()

	packageTomlFile, err := ioutil.TempFile(tmpDir, "package-*.toml")
	assert.Nil(err)

	pack.FixtureManager().TemplateFixtureToFile(
		fixtureName,
		packageTomlFile,
		map[string]interface{}{
			"OS": platform_os,
		},
	)

	assert.Nil(packageTomlFile.Close())

	return packageTomlFile.Name()
}

func createStack(t *testing.T, dockerCli client.CommonAPIClient, registryConfig *h.TestRegistryConfig, imageManager managers.ImageManager, runImageName, buildImageName, runImageMirror string) error {
	t.Helper()
	t.Log("creating stack images...")

	stackBaseDir := filepath.Join("..", "testdata", "mock_stack", imageManager.HostOS())

	_, err := os.Stat(stackBaseDir)
	if err != nil {
		return err
	}

	if err := createStackImage(dockerCli, runImageName, filepath.Join(stackBaseDir, "run")); err != nil {
		return err
	}
	if err := createStackImage(dockerCli, buildImageName, filepath.Join(stackBaseDir, "build")); err != nil {
		return err
	}

	imageManager.TagImage(runImageName, runImageMirror)
	if err := h.PushImage(dockerCli, runImageMirror, registryConfig); err != nil {
		return err
	}

	return nil
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
