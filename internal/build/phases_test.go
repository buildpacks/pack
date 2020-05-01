package build_test

import (
	"context"
	ioutil "io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/api"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/build/fakes"
	h "github.com/buildpacks/pack/testhelpers"
)

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

	when("#Detect", func() {
		it("creates a phase and then runs it", func() {
			lifecycle := fakeLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Detect(context.Background(), "test", []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := fakeLifecycle(t, true)
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
			lifecycle := fakeLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedNetworkMode := "some-network-mode"

			err := lifecycle.Detect(context.Background(), expectedNetworkMode, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
		})

		it("configures the phase with binds", func() {
			lifecycle := fakeLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedBind := "some-mount-source:/some-mount-target"

			err := lifecycle.Detect(context.Background(), "test", []string{expectedBind}, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
		})
	})

	when("#Analyze", func() {
		it("creates a phase and then runs it", func() {
			lifecycle := fakeLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Analyze(context.Background(), "test", "test", false, false, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		when("clear cache", func() {
			it("configures the phase with the expected arguments", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := lifecycle.Analyze(context.Background(), expectedRepoName, "test", false, true, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Cmd, "-skip-layers")
			})
		})

		when("clear cache is false", func() {
			it("configures the phase with the expected arguments", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := lifecycle.Analyze(context.Background(), expectedRepoName, "test", false, false, fakePhaseFactory)
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
			it("configures the phase with registry access", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepos := "some-repo-name"

				err := lifecycle.Analyze(context.Background(), expectedRepos, "test", true, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_REGISTRY_AUTH={}")
				h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode("host"))
			})

			it("configures the phase with root", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", true, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
			})

			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := fakeLifecycle(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := verboseLifecycle.Analyze(context.Background(), expectedRepoName, "test", true, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					//[]string{"-log-level", "debug"}, // TODO: fix [https://github.com/buildpacks/pack/issues/419].
					[]string{"-layers", "/layers"},
					[]string{expectedRepoName},
				)
			})

			it("configures the phase with binds", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBind := "some-cache:/cache"

				err := lifecycle.Analyze(context.Background(), "test", "some-cache", true, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
			})
		})

		when("publish is false", func() {
			it("configures the phase with daemon access", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", false, false, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
			})

			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := fakeLifecycle(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := verboseLifecycle.Analyze(context.Background(), expectedRepoName, "test", false, true, fakePhaseFactory)
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

			it("configures the phase with binds", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBind := "some-cache:/cache"

				err := lifecycle.Analyze(context.Background(), "test", "some-cache", false, true, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
			})
		})
	})

	when("#Restore", func() {
		it("creates a phase and then runs it", func() {
			lifecycle := fakeLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Restore(context.Background(), "test", fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with root access", func() {
			lifecycle := fakeLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := lifecycle.Restore(context.Background(), "test", fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.ContainerConfig().User, "root")
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := fakeLifecycle(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := verboseLifecycle.Restore(context.Background(), "test", fakePhaseFactory)
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

		it("configures the phase with binds", func() {
			lifecycle := fakeLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedBind := "some-cache:/cache"

			err := lifecycle.Restore(context.Background(), "some-cache", fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
		})
	})

	when("#Build", func() {
		it("creates a phase and then runs it", func() {
			lifecycle := fakeLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Build(context.Background(), "test", []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := fakeLifecycle(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := verboseLifecycle.Build(context.Background(), "test", []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.Name(), "builder")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				//[]string{"-log-level", "debug"}, // TODO: fix [https://github.com/buildpacks/pack/issues/419].
				[]string{"-layers", "/layers"},
				[]string{"-app", "/workspace"},
				[]string{"-platform", "/platform"},
			)
		})

		it("configures the phase with the expected network mode", func() {
			lifecycle := fakeLifecycle(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedNetworkMode := "some-network-mode"

			err := lifecycle.Build(context.Background(), expectedNetworkMode, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
		})

		it("configures the phase with binds", func() {
			lifecycle := fakeLifecycle(t, false)
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
			lifecycle := fakeLifecycle(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Export(context.Background(), "test", "test", false, "test", "test", fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := fakeLifecycle(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedRepoName := "some-repo-name"

			err := verboseLifecycle.Export(context.Background(), expectedRepoName, "test", false, "test", "test", fakePhaseFactory)
			h.AssertNil(t, err)

			configProvider := fakePhaseFactory.NewCalledWithProvider
			h.AssertEq(t, configProvider.Name(), "exporter")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
				[]string{"-cache-dir", "/cache"},
				[]string{"-layers", "/layers"},
				[]string{"-app", "/workspace"},
				[]string{expectedRepoName},
			)
		})

		when("publish", func() {
			it("configures the phase with registry access", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepos := []string{"some-repo-name", "some-run-image"}

				err := lifecycle.Export(context.Background(), expectedRepos[0], expectedRepos[1], true, "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_REGISTRY_AUTH={}")
				h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode("host"))
			})

			it("configures the phase with root", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", true, "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
			})

			it("configures the phase with binds", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBind := "some-cache:/cache"

				err := lifecycle.Export(context.Background(), "test", "test", true, "test", "some-cache", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
			})
		})

		when("publish is false", func() {
			it("configures the phase with daemon access", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", false, "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
			})

			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := fakeLifecycle(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"
				expectedRunImage := "some-run-image"
				expectedLaunchCacheName := "some-launch-cache"
				expectedCacheName := "some-cache"

				err := verboseLifecycle.Export(context.Background(), expectedRepoName, expectedRunImage, false, expectedLaunchCacheName, expectedCacheName, fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "exporter")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-daemon"},
					[]string{"-launch-cache", "/launch-cache"},
				)
			})

			it("configures the phase with binds", func() {
				lifecycle := fakeLifecycle(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBinds := []string{"some-cache:/cache", "some-launch-cache:/launch-cache"}

				err := lifecycle.Export(context.Background(), "test", "test", false, "some-launch-cache", "some-cache", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)
			})
		})

		when("platform api 0.2", func() {
			it("uses -image", func() {
				platformAPIVersion, err := api.NewVersion("0.2")
				h.AssertNil(t, err)
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithPlatformVersion(platformAPIVersion))
				h.AssertNil(t, err)
				lifecycle := fakeLifecycle(t, false, fakes.WithBuilder(fakeBuilder))
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRunImage := "some-run-image"

				err = lifecycle.Export(context.Background(), "test", expectedRunImage, false, "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "exporter")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-image", expectedRunImage},
				)
			})
		})

		when("platform api 0.3+", func() {
			var (
				fakeBuilder *fakes.FakeBuilder
				err         error
			)

			it.Before(func() {
				platformAPIVersion, err := api.NewVersion("0.3")
				h.AssertNil(t, err)
				fakeBuilder, err = fakes.NewFakeBuilder(fakes.WithPlatformVersion(platformAPIVersion))
				h.AssertNil(t, err)
			})

			it("uses -run-image instead of deprecated -image", func() {
				lifecycle := fakeLifecycle(t, false, fakes.WithBuilder(fakeBuilder))
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRunImage := "some-run-image"

				err = lifecycle.Export(context.Background(), "test", expectedRunImage, false, "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)

				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertEq(t, configProvider.Name(), "exporter")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-run-image", expectedRunImage},
				)
			})

			it("configures the phase with default arguments", func() {
				lifecycle := fakeLifecycle(t, true, fakes.WithBuilder(fakeBuilder), func(options *build.LifecycleOptions) {
					options.DefaultProcessType = "test-process"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedDefaultProc := []string{"-process-type", "test-process"}

				err := lifecycle.Export(context.Background(), "test", "test", false, "test", "test", fakePhaseFactory)
				h.AssertNil(t, err)
				configProvider := fakePhaseFactory.NewCalledWithProvider
				h.AssertIncludeAllExpectedPatterns(t, configProvider.ContainerConfig().Cmd, expectedDefaultProc)
			})

		})
	})
}

func fakeLifecycle(t *testing.T, logVerbose bool, ops ...func(*build.LifecycleOptions)) *build.Lifecycle {
	lifecycle, err := fakes.NewFakeLifecycle(logVerbose, ops...)
	h.AssertNil(t, err)
	return lifecycle
}
