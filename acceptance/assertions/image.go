// +build acceptance

package assertions

import (
	"fmt"
	"testing"

	"github.com/buildpacks/pack/acceptance/managers"
	h "github.com/buildpacks/pack/testhelpers"
)

type ImageAssertionManager struct {
	testObject   *testing.T
	assert       h.AssertionManager
	imageManager managers.ImageManager
	registry     *h.TestRegistryConfig
}

func NewImageAssertionManager(t *testing.T, imageManager managers.ImageManager, registry *h.TestRegistryConfig) ImageAssertionManager {
	return ImageAssertionManager{
		testObject:   t,
		assert:       h.NewAssertionManager(t),
		imageManager: imageManager,
		registry:     registry,
	}
}

func (a ImageAssertionManager) ExistsLocally(name string) {
	a.testObject.Helper()

	_, err := a.imageManager.Inspect(name)
	a.assert.Nil(err)
}

func (a ImageAssertionManager) NotExistsLocally(name string) {
	a.testObject.Helper()
	_, err := a.imageManager.Inspect(name)
	a.assert.ErrorContains(err, "No such image")
}

func (a ImageAssertionManager) HasBaseImage(image, base string) {
	a.testObject.Helper()
	imageInspect, err := a.imageManager.Inspect(image)
	a.assert.Nil(err)
	baseInspect, err := a.imageManager.Inspect(base)
	a.assert.Nil(err)
	for i, layer := range baseInspect.RootFS.Layers {
		a.assert.Equal(imageInspect.RootFS.Layers[i], layer)
	}
}

func (a ImageAssertionManager) HasLabelWithData(image, label, data string) {
	a.testObject.Helper()
	inspect, err := a.imageManager.Inspect(image)
	a.assert.Nil(err)
	label, ok := inspect.Config.Labels[label]
	a.assert.TrueWithMessage(ok, fmt.Sprintf("expected label %s to exist", label))
	a.assert.Contains(label, data)
}

func (a ImageAssertionManager) RunsWithOutput(image string, expectedOutputs ...string) {
	a.testObject.Helper()
	containerName := "test-" + h.RandString(10)
	container := a.imageManager.ExposePortOnImage(image, containerName)
	defer container.Cleanup()

	output := container.WaitForResponse(managers.DefaultDuration)
	a.assert.ContainsAll(output, expectedOutputs...)
}

func (a ImageAssertionManager) RunsWithLogs(image string, expectedOutputs ...string) {
	a.testObject.Helper()
	container := a.imageManager.CreateContainer(image)
	defer container.Cleanup()

	output := container.RunWithOutput()
	a.assert.ContainsAll(output, expectedOutputs...)
}

func (a ImageAssertionManager) CanBePulledFromRegistry(name string) {
	a.testObject.Helper()
	a.imageManager.PullImage(name, a.registry.RegistryAuth())
	a.ExistsLocally(name)
}

func (a ImageAssertionManager) ExistsInRegistryCatalog(name string) {
	a.testObject.Helper()
	contents, err := a.registry.RegistryCatalog()
	a.assert.Nil(err)
	a.assert.ContainsWithMessage(contents, name, fmt.Sprintf("Expected to see image %s in %%s", name))
}

func (a ImageAssertionManager) NotExistsInRegistry(name string) {
	a.testObject.Helper()
	contents, err := a.registry.RegistryCatalog()
	a.assert.Nil(err)
	a.assert.NotContainWithMessage(
		contents,
		name,
		"Didn't expect to see image %s in the registry",
	)
}
