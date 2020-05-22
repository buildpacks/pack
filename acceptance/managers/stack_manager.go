// +build acceptance

package managers

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"

	"github.com/docker/docker/api/types"

	"github.com/buildpacks/pack/internal/archive"

	h "github.com/buildpacks/pack/testhelpers"
	"github.com/docker/docker/client"
)

const (
	RunImage   = "pack-test/run"
	buildImage = "pack-test/build"
)

var (
	baseStackPath = filepath.Join("testdata", "mock_stack")
	stackExists   = false
)

type StackManager struct {
	testObject       *testing.T
	dockerCli        *client.Client
	registry         *h.TestRegistryConfig
	assert           assertions.AssertionManager
	containerManager TestContainerManager
	runImageMirror   string
}

func NewStackManager(
	t *testing.T,
	dockerCli *client.Client,
	registry *h.TestRegistryConfig,
	assert assertions.AssertionManager,
	containerManager TestContainerManager,
) StackManager {

	return StackManager{
		testObject:       t,
		dockerCli:        dockerCli,
		registry:         registry,
		assert:           assert,
		containerManager: containerManager,
		runImageMirror:   registry.RepoName(RunImage),
	}
}

func (s StackManager) EnsureDefaultStackCreated() {
	s.testObject.Helper()

	if stackExists {
		return
	}

	s.createStack()
}

func (s StackManager) createStack() {
	s.testObject.Helper()
	s.testObject.Log("creating stack images...")

	s.createStackImage(RunImage, s.runImagePath())
	s.createStackImage(buildImage, s.buildImagePath())

	err := s.dockerCli.ImageTag(context.Background(), RunImage, s.runImageMirror)
	s.assert.Nil(err)

	err = h.PushImage(s.dockerCli, s.runImageMirror, s.registry)
	s.assert.Nil(err)

	stackExists = true
}

func (s StackManager) createStackImage(repoName, stackDir string) {
	s.testObject.Helper()

	// TODO: by reusing our implementation of archive.ReadDirAsTar are we representing a realistic or appropriately
	// segregated approach to how stacks are built?
	stackTarball := archive.ReadDirAsTar(stackDir, "/", 0, 0, -1, true, nil)

	s.buildImage(repoName, stackTarball)
}

func (s StackManager) runImagePath() string {
	s.testObject.Helper()

	return filepath.Join(baseStackPath, s.containerManager.HostOS(), "run")
}

func (s StackManager) buildImagePath() string {
	s.testObject.Helper()

	return filepath.Join(baseStackPath, s.containerManager.HostOS(), "build")
}

func (s StackManager) buildImage(name string, buildContext io.Reader) {
	buildResult, err := s.dockerCli.ImageBuild(
		context.Background(),
		buildContext,
		types.ImageBuildOptions{
			Tags:        []string{name},
			Remove:      true,
			ForceRemove: true,
		},
	)
	s.assert.Nil(err)

	// Discard of body required to complete synchronous image build
	// TODO: would polling daemon for built image be more reliable than CLI says done?
	_, err = io.Copy(ioutil.Discard, buildResult.Body)
	s.assert.Nil(err)

	err = buildResult.Body.Close()
	s.assert.Nil(err)
}

func (s StackManager) RunImageMirror() string {
	return s.runImageMirror
}

func (s StackManager) Cleanup() {
	s.testObject.Helper()

	if stackExists {
		err := h.DockerRmi(s.dockerCli, RunImage, buildImage, s.runImageMirror)
		s.assert.Nil(err)
	}
}

const customRebaseRunImageFormat = `
FROM %s
USER root
RUN echo %s > /contents1.txt
RUN echo %s > /contents2.txt
USER pack
`

func (s StackManager) CreateCustomRunImage(name, contents1, contents2 string) {
	s.testObject.Helper()

	h.CreateImage(s.testObject, s.dockerCli, name, fmt.Sprintf(customRebaseRunImageFormat, RunImage, contents1, contents2))
}

func (s StackManager) CreateCustomRunImageOnRemote(name, contents1, contents2 string) {
	s.testObject.Helper()

	h.CreateImageOnRemote(
		s.testObject,
		s.dockerCli,
		s.registry,
		name,
		fmt.Sprintf(customRebaseRunImageFormat, RunImage, contents1, contents2),
	)
}

func (s StackManager) BuilderConfigBlock() string {
	return fmt.Sprintf(`
[stack]
  id = "pack.test.stack"
  build-image = "%s"
  run-image = "%s"
  run-image-mirrors = ["%s"]
`, buildImage, RunImage, s.runImageMirror)
}
