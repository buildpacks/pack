// +build acceptance

package assertions

import (
	"context"
	"fmt"
	"testing"
	"time"

	h "github.com/buildpacks/pack/testhelpers"
)

type Container interface {
	Kill()
	Remove()
	WaitForResponse(time.Duration) string
}

type containerManager interface {
	RunDockerImageExposePort(string, string) Container
	RunDockerImageCombinedOutput(string, string) string
	RootFSLayers(string) []string
	ImageLabel(string, string) string
}

type ImageAssertionManager struct {
	testObject       *testing.T
	assert           AssertionManager
	containerManager containerManager
	imageName        string
}

func (a AssertionManager) NewImageAssertionManager(
	imageName string,
	containerManager containerManager,
) ImageAssertionManager {

	return ImageAssertionManager{
		testObject:       a.testObject,
		assert:           a,
		containerManager: containerManager,
		imageName:        imageName,
	}
}

func (a AssertionManager) ImageExistsLocally(name string) {
	a.testObject.Helper()

	_, _, err := a.dockerCli.ImageInspectWithRaw(context.Background(), name)
	a.Nil(err)
}

func (a AssertionManager) noImageExistsLocally(name, expected string) {
	a.testObject.Helper()

	_, _, err := a.dockerCli.ImageInspectWithRaw(context.Background(), name)
	a.errorMatches(err, expected)
}

func (a AssertionManager) ImageIsWindows(name string) {
	a.testObject.Helper()

	inspect, _, err := a.dockerCli.ImageInspectWithRaw(context.Background(), name)
	a.Nil(err)

	a.Equal(inspect.Os, "windows")
}

// TODO: Is it possible to perform this test using client.ImageSearch? Initial experiment was unsuccessful
func (a AssertionManager) ImageExistsInRegistry(name string, registry *h.TestRegistryConfig) {
	a.testObject.Helper()

	err := h.PullImageWithAuth(a.dockerCli, name, registry.RegistryAuth())
	a.Nil(err)

	a.ImageExistsLocally(name)
}

func (a AssertionManager) AppExistsInCatalog(name string, registry *h.TestRegistryConfig) {
	a.testObject.Helper()
	a.testObject.Log("checking that registry has contents")

	contents, err := registry.RegistryCatalog()
	a.Nil(err)
	a.ContainsWithMessage(contents, name, fmt.Sprintf("Expected to see image %s in %%s", name))
}

func (a AssertionManager) ImageExistsOnlyInRegistry(name string, registry *h.TestRegistryConfig) {
	a.testObject.Helper()

	a.noImageExistsLocally(name, "No such image")

	a.ImageExistsInRegistry(name, registry)
}

func (a AssertionManager) NoImageExistsInRegistry(name string, registry *h.TestRegistryConfig) {
	a.testObject.Helper()
	a.testObject.Log("registry is empty")

	contents, err := registry.RegistryCatalog()
	a.Nil(err)
	a.NotContainWithMessage(
		contents,
		name,
		"Should not have published image without the '--publish' flag: got %s",
	)
}

func (i ImageAssertionManager) RunsMockAppWithOutput(expectedOutputs ...string) {
	i.testObject.Helper()
	i.testObject.Log("app is runnable")

	containerName := "test-" + h.RandString(10)
	appContainer := i.containerManager.RunDockerImageExposePort(containerName, i.imageName)
	defer appContainer.Kill()
	defer appContainer.Remove()
	output := appContainer.WaitForResponse(10 * time.Second)
	i.assert.ContainsAll(output, expectedOutputs...)
}

func (i ImageAssertionManager) RunsMockAppWithLogs(expectedOutputs ...string) {
	i.testObject.Helper()

	containerName := "test-" + h.RandString(10)
	output := i.containerManager.RunDockerImageCombinedOutput(containerName, i.imageName)

	for _, expectedOutput := range expectedOutputs {
		i.assert.Contains(output, expectedOutput)
	}
}

func (i ImageAssertionManager) HasBase(base string) {
	i.testObject.Helper()
	i.testObject.Logf("use the expected base image %s", base)

	imageLayers := i.containerManager.RootFSLayers(i.imageName)
	baseLayers := i.containerManager.RootFSLayers(base)

	for j, layer := range baseLayers {
		i.assert.Equal(imageLayers[j], layer)
	}
}

func (i ImageAssertionManager) HasRunImageMetadata(runImage, runImageMirror string) {
	i.testObject.Helper()
	i.testObject.Log("sets the run image metadata")

	appMetadata := i.containerManager.ImageLabel(i.imageName, "io.buildpacks.lifecycle.metadata")
	i.assert.Contains(
		appMetadata,
		fmt.Sprintf(`"stack":{"runImage":{"image":"%s","mirrors":["%s"]}}}`, runImage, runImageMirror),
	)
}
