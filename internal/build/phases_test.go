package build_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/build/fakes"
	ilogging "github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

// TestPhases are unit tests that test each possible phase to ensure they are executed with the proper parameters
func TestPhases(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "phases", testPhases, spec.Report(report.Terminal{}), spec.Sequential())
}

func testPhases(t *testing.T, when spec.G, it spec.S) {
	// Avoid contaminating tests with existing docker configuration.
	// GGCR resolves the default keychain by inspecting DOCKER_CONFIG - this is used by the Analyze step
	// when constructing the auth config (see `auth.BuildEnvVar` in phases.go).
	var dockerConfigDir string
	it.Before(func() {
		var err error
		dockerConfigDir, err = ioutil.TempDir("", "empty-docker-config-dir")
		h.AssertNil(t, err)

		h.AssertNil(t, os.Setenv("DOCKER_CONFIG", dockerConfigDir))
	})

	it.After(func() {
		h.AssertNil(t, os.Unsetenv("DOCKER_CONFIG"))
		h.AssertNil(t, os.RemoveAll(dockerConfigDir))
	})

	when("#Create", func() {
		it("creates a phase and then run it", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Create(
				context.Background(),
				false,
				false,
				"test",
				"test",
				"test",
				"test",
				"test",
				[]string{},
				fakePhaseFactory,
			)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := newTestLifecycle(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedRepoName := "some-repo-name"
			expectedRunImage := "some-run-image"

			err := verboseLifecycle.Create(
				context.Background(),
				false,
				false,
				expectedRunImage,
				"test",
				"test",
				expectedRepoName,
				"test",
				[]string{},
				fakePhaseFactory,
			)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.Name(), "creator")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
				[]string{"-run-image", expectedRunImage},
				[]string{expectedRepoName},
			)
		})

		it("configures the phase with the expected network mode", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedNetworkMode := "some-network-mode"

			err := lifecycle.Create(
				context.Background(),
				false,
				false,
				"test",
				"test",
				"test",
				"test",
				expectedNetworkMode,
				[]string{},
				fakePhaseFactory,
			)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
		})

		when("clear cache", func() {
			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycle(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := verboseLifecycle.Create(
					context.Background(),
					false,
					true,
					"test",
					"test",
					"test",
					"test",
					"test",
					[]string{},
					fakePhaseFactory,
				)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "creator")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-skip-restore"},
				)
			})
		})

		when("clear cache is false", func() {
			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycle(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := verboseLifecycle.Create(
					context.Background(),
					false,
					false,
					"test",
					"test",
					"test",
					"test",
					"test",
					[]string{},
					fakePhaseFactory,
				)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "creator")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-cache-dir", "/cache"},
				)
			})
		})

		when("publish", func() {
			it("configures the phase with binds", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				volumeMount := "custom-mount-source:/custom-mount-target"
				expectedBinds := []string{volumeMount, "some-cache:/cache"}

				err := lifecycle.Create(
					context.Background(),
					true,
					false,
					"test",
					"test",
					"some-cache",
					"test",
					"test",
					[]string{volumeMount},
					fakePhaseFactory,
				)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)
			})

			it("configures the phase with root", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Create(
					context.Background(),
					true,
					false,
					"test",
					"test",
					"test",
					"test",
					"test",
					[]string{},
					fakePhaseFactory,
				)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
			})

			it("configures the phase with registry access", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepos := "some-repo-name"

				err := lifecycle.Create(
					context.Background(),
					true,
					false,
					"test",
					"test",
					"test",
					expectedRepos,
					"test",
					[]string{},
					fakePhaseFactory,
				)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_REGISTRY_AUTH={}")
			})
		})

		when("publish is false", func() {
			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycle(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := verboseLifecycle.Create(
					context.Background(),
					false,
					false,
					"test",
					"test",
					"test",
					"test",
					"test",
					[]string{},
					fakePhaseFactory,
				)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "creator")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-daemon"},
					[]string{"-launch-cache", "/launch-cache"},
				)
			})

			it("configures the phase with daemon access", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Create(
					context.Background(),
					false,
					false,
					"test",
					"some-launch-cache",
					"some-cache",
					"test",
					"test",
					[]string{},
					fakePhaseFactory,
				)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
			})

			it("configures the phase with binds", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				volumeMount := "custom-mount-source:/custom-mount-target"
				expectedBinds := []string{volumeMount, "some-cache:/cache", "some-launch-cache:/launch-cache"}

				err := lifecycle.Create(
					context.Background(),
					false,
					false,
					"test",
					"some-launch-cache",
					"some-cache",
					"test",
					"test",
					[]string{volumeMount},
					fakePhaseFactory,
				)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)
			})
		})
	})

	when("#Detect", func() {
		it("creates a phase and then runs it", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Detect(context.Background(), "test", []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := newTestLifecycle(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := verboseLifecycle.Detect(context.Background(), "test", []string{"test"}, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.Name(), "detector")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
				[]string{"-app", "/workspace"},
				[]string{"-platform", "/platform"},
			)
		})

		it("configures the phase with the expected network mode", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedNetworkMode := "some-network-mode"

			err := lifecycle.Detect(context.Background(), expectedNetworkMode, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
		})

		it("configures the phase to copy app dir", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedBind := "some-mount-source:/some-mount-target"

			err := lifecycle.Detect(context.Background(), "test", []string{expectedBind}, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)

			h.AssertEq(t, len(configProvider.ContainerOps()), 1)
			h.AssertFunctionName(t, configProvider.ContainerOps()[0], "CopyDir")
		})
	})

	when("#Analyze", func() {
		it("creates a phase and then runs it", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Analyze(context.Background(), "test", "test", "test", false, false, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		when("clear cache", func() {
			it("configures the phase with the expected arguments", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := lifecycle.Analyze(context.Background(), expectedRepoName, "test", "test", false, true, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Cmd, "-skip-layers")
			})
		})

		when("clear cache is false", func() {
			it("configures the phase with the expected arguments", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := lifecycle.Analyze(context.Background(), expectedRepoName, "test", "test", false, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-cache-dir", "/cache"},
				)
			})
		})

		when("publish", func() {
			it("runs the phase with the lifecycle image", func() {
				lifecycle := newTestLifecycle(t, true, func(options *build.LifecycleOptions) {
					options.LifecycleImage = "some-lifecycle-image"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", "test", true, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().Image, "some-lifecycle-image")
			})

			it("sets the CNB_USER_ID and CNB_GROUP_ID in the environment", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithUID(2222), fakes.WithGID(3333))
				h.AssertNil(t, err)
				lifecycle := newTestLifecycle(t, false, fakes.WithBuilder(fakeBuilder))
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err = lifecycle.Analyze(context.Background(), "test", "test", "test", true, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_USER_ID=2222")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_GROUP_ID=3333")
			})

			it("configures the phase with registry access", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepos := "some-repo-name"
				expectedNetworkMode := "some-network-mode"

				err := lifecycle.Analyze(context.Background(), expectedRepos, "test", expectedNetworkMode, true, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_REGISTRY_AUTH={}")
				h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
			})

			it("configures the phase with root", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", "test", true, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
			})

			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycle(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := verboseLifecycle.Analyze(context.Background(), expectedRepoName, "test", "test", true, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-log-level", "debug"},
					[]string{"-layers", "/layers"},
					[]string{expectedRepoName},
				)
			})

			it("configures the phase with binds", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBind := "some-cache:/cache"

				err := lifecycle.Analyze(context.Background(), "test", "some-cache", "test", true, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
			})
		})

		when("publish is false", func() {
			it("runs the phase with the lifecycle image", func() {
				lifecycle := newTestLifecycle(t, true, func(options *build.LifecycleOptions) {
					options.LifecycleImage = "some-lifecycle-image"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", "test", false, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().Image, "some-lifecycle-image")
			})

			it("sets the CNB_USER_ID and CNB_GROUP_ID in the environment", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithUID(2222), fakes.WithGID(3333))
				h.AssertNil(t, err)
				lifecycle := newTestLifecycle(t, false, fakes.WithBuilder(fakeBuilder))
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err = lifecycle.Analyze(context.Background(), "test", "test", "test", false, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_USER_ID=2222")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_GROUP_ID=3333")
			})

			it("configures the phase with daemon access", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", "test", false, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
			})

			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycle(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := verboseLifecycle.Analyze(context.Background(), expectedRepoName, "test", "test", false, true, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-log-level", "debug"},
					[]string{"-daemon"},
					[]string{"-layers", "/layers"},
					[]string{expectedRepoName},
				)
			})

			it("configures the phase with the expected network mode", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedNetworkMode := "some-network-mode"

				err := lifecycle.Analyze(context.Background(), "test", "test", expectedNetworkMode, false, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
			})

			it("configures the phase with binds", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBind := "some-cache:/cache"

				err := lifecycle.Analyze(context.Background(), "test", "some-cache", "test", false, true, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
			})
		})
	})

	when("#Restore", func() {
		it("runs the phase with the lifecycle image", func() {
			lifecycle := newTestLifecycle(t, true, func(options *build.LifecycleOptions) {
				options.LifecycleImage = "some-lifecycle-image"
			})
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := lifecycle.Restore(context.Background(), "test", "test", fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.ContainerConfig().Image, "some-lifecycle-image")
		})

		it("sets the CNB_USER_ID and CNB_GROUP_ID in the environment", func() {
			fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithUID(2222), fakes.WithGID(3333))
			h.AssertNil(t, err)
			lifecycle := newTestLifecycle(t, false, fakes.WithBuilder(fakeBuilder))
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err = lifecycle.Restore(context.Background(), "test", "test", fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_USER_ID=2222")
			h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_GROUP_ID=3333")
		})

		it("creates a phase and then runs it", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Restore(context.Background(), "test", "test", fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with root access", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := lifecycle.Restore(context.Background(), "test", "test", fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.ContainerConfig().User, "root")
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := newTestLifecycle(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := verboseLifecycle.Restore(context.Background(), "test", "test", fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.Name(), "restorer")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
				[]string{"-cache-dir", "/cache"},
				[]string{"-layers", "/layers"},
			)
		})

		it("configures the phase with the expected network mode", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedNetworkMode := "some-network-mode"

			err := lifecycle.Restore(context.Background(), "test", expectedNetworkMode, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
		})

		it("configures the phase with binds", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedBind := "some-cache:/cache"

			err := lifecycle.Restore(context.Background(), "some-cache", "test", fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
		})
	})

	when("#Build", func() {
		it("creates a phase and then runs it", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Build(context.Background(), "test", []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			platformAPIVersion, err := api.NewVersion("0.3")
			h.AssertNil(t, err)
			fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithPlatformVersion(platformAPIVersion))
			h.AssertNil(t, err)
			verboseLifecycle := newTestLifecycle(t, true, fakes.WithBuilder(fakeBuilder))
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err = verboseLifecycle.Build(context.Background(), "test", []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.Name(), "builder")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
				[]string{"-layers", "/layers"},
				[]string{"-app", "/workspace"},
				[]string{"-platform", "/platform"},
			)
		})

		it("configures the phase with the expected network mode", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedNetworkMode := "some-network-mode"

			err := lifecycle.Build(context.Background(), expectedNetworkMode, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
		})

		it("configures the phase with binds", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedBind := "some-mount-source:/some-mount-target"

			err := lifecycle.Build(context.Background(), "test", []string{expectedBind}, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
		})
	})

	when("#Export", func() {
		it("creates a phase and then runs it", func() {
			lifecycle := newTestLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Export(context.Background(), "test", "test", false, "test", "test", "test", fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := newTestLifecycle(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedRepoName := "some-repo-name"
			expectedRunImage := "some-run-image"

			err := verboseLifecycle.Export(context.Background(), expectedRepoName, expectedRunImage, false, "test", "test", "test", fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.Name(), "exporter")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
				[]string{"-cache-dir", "/cache"},
				[]string{"-layers", "/layers"},
				[]string{"-app", "/workspace"},
				[]string{"-run-image", expectedRunImage},
				[]string{expectedRepoName},
			)
		})

		when("publish", func() {
			it("runs the phase with the lifecycle image", func() {
				lifecycle := newTestLifecycle(t, true, func(options *build.LifecycleOptions) {
					options.LifecycleImage = "some-lifecycle-image"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", true, "test", "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().Image, "some-lifecycle-image")
			})

			it("sets the CNB_USER_ID and CNB_GROUP_ID in the environment", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithUID(2222), fakes.WithGID(3333))
				h.AssertNil(t, err)
				lifecycle := newTestLifecycle(t, false, fakes.WithBuilder(fakeBuilder))
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err = lifecycle.Export(context.Background(), "test", "test", true, "test", "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_USER_ID=2222")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_GROUP_ID=3333")
			})

			it("configures the phase with registry access", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepos := []string{"some-repo-name", "some-run-image"}

				err := lifecycle.Export(context.Background(), expectedRepos[0], expectedRepos[1], true, "test", "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_REGISTRY_AUTH={}")
			})

			it("configures the phase with root", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", true, "test", "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
			})

			it("configures the phase with the expected network mode", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedNetworkMode := "some-network-mode"

				err := lifecycle.Export(context.Background(), "test", "test", true, "test", "test", expectedNetworkMode, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
			})

			it("configures the phase with binds", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBind := "some-cache:/cache"

				err := lifecycle.Export(context.Background(), "test", "test", true, "test", "some-cache", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
			})

			it("configures the phase to write stack toml", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBinds := []string{"some-cache:/cache", "some-launch-cache:/launch-cache"}

				err := lifecycle.Export(context.Background(), "test", "test", false, "some-launch-cache", "some-cache", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)

				h.AssertEq(t, len(configProvider.ContainerOps()), 1)
				h.AssertFunctionName(t, configProvider.ContainerOps()[0], "WriteStackToml")
			})

			it("configures the phase with default process type", func() {
				lifecycle := newTestLifecycle(t, true, func(options *build.LifecycleOptions) {
					options.DefaultProcessType = "test-process"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedDefaultProc := []string{"-process-type", "test-process"}

				err := lifecycle.Export(context.Background(), "test", "test", true, "test", "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)
				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertIncludeAllExpectedPatterns(t, configProvider.ContainerConfig().Cmd, expectedDefaultProc)
			})
		})

		when("publish is false", func() {
			it("runs the phase with the lifecycle image", func() {
				lifecycle := newTestLifecycle(t, true, func(options *build.LifecycleOptions) {
					options.LifecycleImage = "some-lifecycle-image"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", false, "test", "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().Image, "some-lifecycle-image")
			})

			it("sets the CNB_USER_ID and CNB_GROUP_ID in the environment", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithUID(2222), fakes.WithGID(3333))
				h.AssertNil(t, err)
				lifecycle := newTestLifecycle(t, false, fakes.WithBuilder(fakeBuilder))
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err = lifecycle.Export(context.Background(), "test", "test", false, "test", "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_USER_ID=2222")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_GROUP_ID=3333")
			})

			it("configures the phase with daemon access", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", false, "test", "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
			})

			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycle(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := verboseLifecycle.Export(context.Background(), "test", "test", false, "test", "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "exporter")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-daemon"},
					[]string{"-launch-cache", "/launch-cache"},
				)
			})

			it("configures the phase with the expected network mode", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedNetworkMode := "some-network-mode"

				err := lifecycle.Export(context.Background(), "test", "test", false, "test", "test", expectedNetworkMode, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
			})

			it("configures the phase with binds", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBinds := []string{"some-cache:/cache", "some-launch-cache:/launch-cache"}

				err := lifecycle.Export(context.Background(), "test", "test", false, "some-launch-cache", "some-cache", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)
			})

			it("configures the phase to write stack toml", func() {
				lifecycle := newTestLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBinds := []string{"some-cache:/cache", "some-launch-cache:/launch-cache"}

				err := lifecycle.Export(context.Background(), "test", "test", false, "some-launch-cache", "some-cache", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)

				h.AssertEq(t, len(configProvider.ContainerOps()), 1)
				h.AssertFunctionName(t, configProvider.ContainerOps()[0], "WriteStackToml")
			})

			it("configures the phase with default process type", func() {
				lifecycle := newTestLifecycle(t, true, func(options *build.LifecycleOptions) {
					options.DefaultProcessType = "test-process"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedDefaultProc := []string{"-process-type", "test-process"}

				err := lifecycle.Export(context.Background(), "test", "test", false, "test", "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)
				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertIncludeAllExpectedPatterns(t, configProvider.ContainerConfig().Cmd, expectedDefaultProc)
			})
		})
	})
}

func newTestLifecycle(t *testing.T, logVerbose bool, ops ...func(*build.LifecycleOptions)) *build.Lifecycle {
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)

	var outBuf bytes.Buffer
	logger := ilogging.NewLogWithWriters(&outBuf, &outBuf)
	if logVerbose {
		logger.Level = log.DebugLevel
	}

	lifecycle := build.NewLifecycle(docker, logger)

	defaultBuilder, err := fakes.NewFakeBuilder()
	h.AssertNil(t, err)

	opts := build.LifecycleOptions{
		AppPath:    "some-app-path",
		Builder:    defaultBuilder,
		HTTPProxy:  "some-http-proxy",
		HTTPSProxy: "some-https-proxy",
		NoProxy:    "some-no-proxy",
	}

	for _, op := range ops {
		op(&opts)
	}

	lifecycle.Setup(opts)

	return lifecycle
}
