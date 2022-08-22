//go:build acceptance
// +build acceptance

package harness

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/docker/docker/client"

	"github.com/buildpacks/pack/acceptance/buildpacks"
	"github.com/buildpacks/pack/acceptance/config"
	"github.com/buildpacks/pack/acceptance/invoke"
	"github.com/buildpacks/pack/acceptance/managers"
	h "github.com/buildpacks/pack/testhelpers"
)

type BuilderCombo struct {
	builderName string
	pack        invoke.PackInvoker
	lifecycle   config.LifecycleAsset
}

func (b *BuilderCombo) BuilderName() string {
	return b.builderName
}

func (b *BuilderCombo) Pack() invoke.PackInvoker {
	return b.pack
}

func (b *BuilderCombo) Lifecycle() config.LifecycleAsset {
	return b.lifecycle
}

func (b *BuilderCombo) String() string {
	return fmt.Sprintf("builder=%s, lifecycle=%v, pack=%v", b.builderName, b.lifecycle, b.pack)
}

type BuilderTestHarness struct {
	t              *testing.T
	registryConfig h.TestRegistryConfig
	imageManager   managers.ImageManager
	stack          config.Stack
	combos         []BuilderCombo
	cleanups       []func()
}

func ContainingBuilder(t *testing.T, projectBaseDir string) BuilderTestHarness {
	t.Helper()
	h.RequireDocker(t)
	rand.Seed(time.Now().UTC().UnixNano())

	var err error
	cleanups := []func(){}
	assert := h.NewAssertionManager(t)

	projectBaseDir, err = filepath.Abs(projectBaseDir)
	assert.Nil(err)

	testDataDir := filepath.Join(projectBaseDir, "acceptance", "testdata")

	// docker cli
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	assert.Nil(err)

	imageManager := managers.NewImageManager(t, dockerCli)

	// run temp registry
	registry := h.RunRegistry(t)
	cleanups = append(cleanups, func() {
		t.Log("stoping and deleting registry...")
		registry.RmRegistry(t)
	})

	// gather config
	inputConfigManager, err := config.NewInputConfigurationManager()
	assert.Nil(err)

	assetsConfig := config.NewAssetManager(t, assert, inputConfigManager, projectBaseDir)

	// create stack
	stack, err := createStack(
		t,
		dockerCli,
		registry,
		imageManager,
		"pack-test/run:"+h.RandString(6),
		"pack-test/build:"+h.RandString(6),
		testDataDir,
	)
	assert.Nil(err)

	stackBaseImages := map[string][]string{
		"linux":   {"ubuntu:bionic"},
		"windows": {"mcr.microsoft.com/windows/nanoserver:1809", "golang:1.17-nanoserver-1809"},
	}
	baseStackNames := stackBaseImages[imageManager.HostOS()]
	cleanups = append(cleanups, func() {
		t.Log("cleaning up stack images...")
		imageManager.CleanupImages(baseStackNames...)
		imageManager.CleanupImages(stack.RunImage.Name, stack.BuildImageName, stack.RunImage.MirrorName)
	})

	combos := []BuilderCombo{}
	for _, combo := range inputConfigManager.Combinations() {
		lifecycle := assetsConfig.NewLifecycleAsset(combo.Lifecycle)
		pack := invoke.NewPackInvoker(
			t,
			assert,
			assetsConfig.NewPackAsset(combo.Pack),
			registry.DockerConfigDir,
		)

		pack.JustRunSuccessfully("config", "lifecycle-image", lifecycle.Image())
		pack.EnableExperimental()

		buildpackManager := buildpacks.NewBuildpackManager(
			t,
			assert,
			buildpacks.WithBuildpackAPIVersion(lifecycle.EarliestBuildpackAPIVersion()),
			buildpacks.WithBaseDir(filepath.Join(testDataDir, "mock_buildpacks")),
		)

		createBuilderPack := invoke.NewPackInvoker(
			t,
			assert,
			assetsConfig.NewPackAsset(combo.PackCreateBuilder),
			registry.DockerConfigDir,
		)
		createBuilderPack.EnableExperimental()

		// create builder
		builderName, err := createBuilder(
			t,
			assert,
			registry,
			imageManager,
			dockerCli,
			*createBuilderPack,
			lifecycle,
			buildpackManager,
			stack,
		)
		assert.Nil(err)
		cleanups = append(cleanups, func() {
			t.Log("cleaning up builder image...")
			imageManager.CleanupImages(builderName)
		})

		pack.JustRunSuccessfully("config", "default-builder", builderName)

		combos = append(combos, BuilderCombo{
			builderName: builderName,
			lifecycle:   lifecycle,
			pack:        *pack,
		})
	}

	return BuilderTestHarness{
		t:              t,
		registryConfig: *registry,
		imageManager:   imageManager,
		stack:          stack,
		combos:         combos,
		cleanups:       cleanups,
	}
}

func (b *BuilderTestHarness) Combinations() []BuilderCombo {
	return b.combos
}

func (b *BuilderTestHarness) Run(name string, test func(t *testing.T, th *BuilderTestHarness, combo BuilderCombo)) {
	for _, combo := range b.combos {
		combo := combo
		b.t.Run(name, func(t *testing.T) {
			test(t, b, combo)
		})
	}
}

func (b *BuilderTestHarness) RunC(test func(combo BuilderCombo)) {
	func_name := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
	b.Run(func_name, func(t *testing.T, th *BuilderTestHarness, combo BuilderCombo) {
		test(combo)
	})
}

func (b *BuilderTestHarness) RunTC(test func(t *testing.T, combo BuilderCombo)) {
	func_name := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
	b.Run(func_name, func(t *testing.T, th *BuilderTestHarness, combo BuilderCombo) {
		test(t, combo)
	})
}

func (b *BuilderTestHarness) RunA(test func(t *testing.T, th *BuilderTestHarness, combo BuilderCombo)) {
	func_name := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
	b.Run(func_name, test)
}

func (b *BuilderTestHarness) Registry() h.TestRegistryConfig {
	return b.registryConfig
}

func (b *BuilderTestHarness) ImageManager() managers.ImageManager {
	return b.imageManager
}

func (b *BuilderTestHarness) Stack() config.Stack {
	return b.stack
}

func (b *BuilderTestHarness) CleanUp() {
	for _, fn := range b.cleanups {
		fn()
	}
}
