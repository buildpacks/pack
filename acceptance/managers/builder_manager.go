// +build acceptance

package managers

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sync"
	"testing"

	"github.com/docker/docker/client"

	h "github.com/buildpacks/pack/testhelpers"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/components"
)

type builderCombo struct {
	pack, lifecycle ComboValue
}

var (
	creationMux sync.Mutex
	created     = map[builderCombo]components.TestBuilder{}
	cleanupMux  sync.Mutex
	cleanedUp   = map[builderCombo]interface{}{}
)

type BuilderManager struct {
	testObject       *testing.T
	assert           assertions.AssertionManager
	registry         *h.TestRegistryConfig
	dockerCli        *client.Client
	pack             *components.PackExecutor
	lifecycle        *components.TestLifecycle
	packageManager   PackageManager
	buildpackManager BuildpackManager
	stacks           StackManager
	builderCombo     builderCombo
}

func NewBuilderManager(
	t *testing.T,
	assert assertions.AssertionManager,
	registry *h.TestRegistryConfig,
	dockerCli *client.Client,
	pack *components.PackExecutor,
	lifecycle *components.TestLifecycle,
	packageManager PackageManager,
	stacks StackManager,
	combo *RunCombo,
) BuilderManager {
	builderCombo := builderCombo{
		pack:      combo.PackCreateBuilder,
		lifecycle: combo.Lifecycle,
	}

	return BuilderManager{
		testObject:       t,
		assert:           assert,
		registry:         registry,
		dockerCli:        dockerCli,
		pack:             pack,
		lifecycle:        lifecycle,
		packageManager:   packageManager,
		stacks:           stacks,
		buildpackManager: NewBuildpackManager(t, assert),
		builderCombo:     builderCombo,
	}
}

func (m BuilderManager) EnsureComboBuilderExists() components.TestBuilder {
	creationMux.Lock()

	if _, ok := created[m.builderCombo]; !ok {
		created[m.builderCombo] = m.createDefaultBuilder()
	}

	creationMux.Unlock()

	return created[m.builderCombo]
}

func (m BuilderManager) EnsureComboBuilderCleanedUp() {
	creationMux.Lock()
	builder, builderCreated := created[m.builderCombo]
	creationMux.Unlock()

	if !builderCreated {
		return
	}

	cleanupMux.Lock()
	if _, ok := cleanedUp[m.builderCombo]; !ok {
		builder.Cleanup()
		cleanedUp[m.builderCombo] = true
	}
	cleanupMux.Unlock()
}

func (m BuilderManager) BuilderComboDescription() string {
	return fmt.Sprintf("pack_%s-lifecycle_%s", m.builderCombo.pack, m.builderCombo.lifecycle)
}

func (m BuilderManager) CreateOneOffBuilder() components.TestBuilder {
	return m.createDefaultBuilder()
}

func (m BuilderManager) createDefaultBuilder() components.TestBuilder {
	packageImageConfig := components.NewPackageImageConfig(m.registry, m.dockerCli)
	packageBuildpack := components.NewPackageImageBuildpack(
		m.packageManager,
		packageImageConfig,
		"package.toml",
		components.SimpleLayersBuildpack,
	)

	buildpacks := []components.TestBuildpack{
		components.NoOpBuildpack,
		components.NoOpBuildpack2,
		components.OtherStackBuildpack,
		components.ReadEnvBuildpack,
		packageBuildpack,
	}

	configProvider := m.newPackTemplateConfig(
		map[string]interface{}{
			"run_image_mirror":   m.stacks.RunImageMirror(),
			"lifecycle_uri":      m.lifecycle.EscapedPath(),
			"package_image_name": packageImageConfig.Name(""),
			"package_id":         "simple/layers",
		},
	)

	return m.createBuilder(buildpacks, configProvider)
}

func (m BuilderManager) CreateWindowsBuilder() components.TestBuilder {
	buildpacks := []components.TestBuildpack{
		components.NoOpBuildpack,
		components.NoOpBuildpack2,
		components.OtherStackBuildpack,
		components.ReadEnvBuildpack,
	}

	configProvider := m.newPackTemplateConfig(
		map[string]interface{}{
			"run_image_mirror": m.stacks.RunImageMirror(),
			"lifecycle_uri":    m.lifecycle.EscapedPath(),
		},
	)

	return m.createBuilder(buildpacks, configProvider)
}

type builderConfigProvider interface {
	ConfigFile(parentDir string) string
}

type packTemplateConfig struct {
	assert        assertions.AssertionManager
	pack          *components.PackExecutor
	template      string
	configuration map[string]interface{}
}

func (m BuilderManager) newPackTemplateConfig(configuration map[string]interface{}) packTemplateConfig {
	return packTemplateConfig{
		assert:        m.assert,
		pack:          m.pack,
		template:      "builder.toml",
		configuration: configuration,
	}
}

func (p packTemplateConfig) ConfigFile(parentDir string) string {
	configFile, err := ioutil.TempFile(parentDir, "builder.toml")
	p.assert.Nil(err)

	p.pack.TemplateFixtureToFile(p.template, configFile, p.configuration)
	err = configFile.Close()
	p.assert.Nil(err)

	return configFile.Name()
}

type DynamicBuilderConfig struct {
	assert     assertions.AssertionManager
	buildpacks []components.TestBuildpack
	stacks     StackManager
	lifecycle  *components.TestLifecycle
}

func (m BuilderManager) NewDynamicBuilderConfig(buildpacks ...components.TestBuildpack) DynamicBuilderConfig {
	return DynamicBuilderConfig{
		assert:     m.assert,
		buildpacks: buildpacks,
		stacks:     m.stacks,
		lifecycle:  m.lifecycle,
	}
}

func (d DynamicBuilderConfig) ConfigFile(parentDir string) string {
	configFile, err := ioutil.TempFile(parentDir, "builder.toml")
	d.assert.Nil(err)

	for _, bp := range d.buildpacks {
		_, err = io.WriteString(configFile, bp.BuilderConfigBlock())
		d.assert.Nil(err)
	}

	_, err = io.WriteString(configFile, d.stacks.BuilderConfigBlock())
	d.assert.Nil(err)

	_, err = io.WriteString(configFile, d.lifecycle.BuilderConfigBlock())
	d.assert.Nil(err)

	err = configFile.Close()
	d.assert.Nil(err)

	return configFile.Name()
}

//nolint:whitespace // A leading line of whitespace is left after a method declaration with multi-line arguments
func (m BuilderManager) createBuilder(
	buildpacks []components.TestBuildpack,
	configProvider builderConfigProvider,
) components.TestBuilder {

	m.testObject.Log("creating builder image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir("", "create-test-builder")
	m.assert.Nil(err)
	defer os.RemoveAll(tmpDir)

	m.buildpackManager.PlaceBuildpacksInDir(tmpDir, buildpacks...)

	configFile := configProvider.ConfigFile(tmpDir)

	name := m.registry.RepoName(fmt.Sprintf("test/builder-%s", h.RandString(8)))
	m.pack.SuccessfulRun("create-builder", "--no-color", name, "-b", configFile)

	err = h.PushImage(m.dockerCli, name, m.registry)
	m.assert.Nil(err)

	return components.NewTestBuilder(m.dockerCli, name)
}
