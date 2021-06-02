package build_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/buildpacks/pack/internal/cache"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/apex/log"
	"github.com/buildpacks/lifecycle/api"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/build/fakes"
	ilogging "github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

// TestLifecycleExecution are unit tests that test each possible phase to ensure they are executed with the proper parameters
func TestLifecycleExecution(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "phases", testLifecycleExecution, spec.Report(report.Terminal{}), spec.Sequential())
}

func testLifecycleExecution(t *testing.T, when spec.G, it spec.S) {
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

	when("#NewLifecycleExecution", func() {
		when("lifecycle supports multiple platform APIs", func() {
			it("select the latest supported version", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithSupportedPlatformAPIs([]*api.Version{
					api.MustParse("0.2"),
					api.MustParse("0.3"),
					api.MustParse("0.4"),
					api.MustParse("0.5"),
				}))
				h.AssertNil(t, err)

				lifecycleExec := newTestLifecycleExec(t, false, fakes.WithBuilder(fakeBuilder))
				h.AssertEq(t, lifecycleExec.PlatformAPI().String(), "0.4")
			})
		})

		when("supported platform API is deprecated", func() {
			it("select the deprecated version", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(
					fakes.WithDeprecatedPlatformAPIs([]*api.Version{api.MustParse("0.4")}),
					fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("1.2")}),
				)
				h.AssertNil(t, err)

				lifecycleExec := newTestLifecycleExec(t, false, fakes.WithBuilder(fakeBuilder))
				h.AssertEq(t, lifecycleExec.PlatformAPI().String(), "0.4")
			})
		})

		when("pack doesn't support any lifecycle supported platform API", func() {
			it("errors", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(
					fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("1.2")}),
				)
				h.AssertNil(t, err)

				_, err = newTestLifecycleExecErr(t, false, fakes.WithBuilder(fakeBuilder))
				h.AssertError(t, err, "unable to find a supported Platform API version")
			})
		})
	})

	when("Run", func() {
		var (
			imageName        name.Tag
			fakeBuilder      *fakes.FakeBuilder
			outBuf           bytes.Buffer
			logger           *ilogging.LogWithWriters
			docker           *client.Client
			fakePhaseFactory *fakes.FakePhaseFactory
		)

		it.Before(func() {
			var err error
			imageName, err = name.NewTag("/some/image", name.WeakValidation)
			h.AssertNil(t, err)

			fakeBuilder, err = fakes.NewFakeBuilder(fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("0.3")}))
			h.AssertNil(t, err)
			logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
			docker, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
			h.AssertNil(t, err)
			fakePhaseFactory = fakes.NewFakePhaseFactory()
		})

		when("Run using creator", func() {
			it("succeeds", func() {
				opts := build.LifecycleOptions{
					Publish:      false,
					ClearCache:   false,
					RunImage:     "test",
					Image:        imageName,
					Builder:      fakeBuilder,
					TrustBuilder: false,
					UseCreator:   true,
				}

				lifecycle, err := build.NewLifecycleExecution(logger, docker, opts)
				h.AssertNil(t, err)
				h.AssertEq(t, filepath.Base(lifecycle.AppDir()), "workspace")

				err = lifecycle.Run(context.Background(), func(execution *build.LifecycleExecution) build.PhaseFactory {
					return fakePhaseFactory
				})
				h.AssertNil(t, err)

				h.AssertEq(t, len(fakePhaseFactory.NewCalledWithProvider), 1)

				for _, entry := range fakePhaseFactory.NewCalledWithProvider {
					if entry.Name() == "creator" {
						h.AssertSliceContains(t, entry.ContainerConfig().Cmd, "/some/image")
					}
				}
			})
			when("Run with workspace dir", func() {
				it("succeeds", func() {
					opts := build.LifecycleOptions{
						Publish:      false,
						ClearCache:   false,
						RunImage:     "test",
						Image:        imageName,
						Builder:      fakeBuilder,
						TrustBuilder: true,
						Workspace:    "app",
						UseCreator:   true,
					}

					lifecycle, err := build.NewLifecycleExecution(logger, docker, opts)
					h.AssertNil(t, err)

					err = lifecycle.Run(context.Background(), func(execution *build.LifecycleExecution) build.PhaseFactory {
						return fakePhaseFactory
					})
					h.AssertNil(t, err)

					h.AssertEq(t, len(fakePhaseFactory.NewCalledWithProvider), 1)

					for _, entry := range fakePhaseFactory.NewCalledWithProvider {
						if entry.Name() == "creator" {
							h.AssertSliceContainsInOrder(t, entry.ContainerConfig().Cmd, "-app", "/app")
							h.AssertSliceContains(t, entry.ContainerConfig().Cmd, "/some/image")
						}
					}
				})
			})
		})
		when("Run without using creator", func() {
			it("succeeds", func() {
				opts := build.LifecycleOptions{
					Publish:      false,
					ClearCache:   false,
					RunImage:     "test",
					Image:        imageName,
					Builder:      fakeBuilder,
					TrustBuilder: false,
					UseCreator:   false,
				}

				lifecycle, err := build.NewLifecycleExecution(logger, docker, opts)
				h.AssertNil(t, err)

				err = lifecycle.Run(context.Background(), func(execution *build.LifecycleExecution) build.PhaseFactory {
					return fakePhaseFactory
				})
				h.AssertNil(t, err)

				h.AssertEq(t, len(fakePhaseFactory.NewCalledWithProvider), 5)

				for _, entry := range fakePhaseFactory.NewCalledWithProvider {
					switch entry.Name() {
					case "exporter":
						h.AssertSliceContains(t, entry.ContainerConfig().Cmd, "/some/image")
					case "analyzer":
						h.AssertSliceContains(t, entry.ContainerConfig().Cmd, "/some/image")
					}
				}
			})
			when("Run with workspace dir", func() {
				it("succeeds", func() {
					opts := build.LifecycleOptions{
						Publish:      false,
						ClearCache:   false,
						RunImage:     "test",
						Image:        imageName,
						Builder:      fakeBuilder,
						TrustBuilder: false,
						Workspace:    "app",
						UseCreator:   false,
					}

					lifecycle, err := build.NewLifecycleExecution(logger, docker, opts)
					h.AssertNil(t, err)
					h.AssertEq(t, filepath.Base(lifecycle.AppDir()), "app")

					err = lifecycle.Run(context.Background(), func(execution *build.LifecycleExecution) build.PhaseFactory {
						return fakePhaseFactory
					})
					h.AssertNil(t, err)

					h.AssertEq(t, len(fakePhaseFactory.NewCalledWithProvider), 5)

					appCount := 0
					for _, entry := range fakePhaseFactory.NewCalledWithProvider {
						switch entry.Name() {
						case "detector", "builder", "exporter":
							h.AssertSliceContainsInOrder(t, entry.ContainerConfig().Cmd, "-app", "/app")
							appCount++
						}
					}
					h.AssertEq(t, appCount, 3)
				})
			})
		})

		when("Error cases", func() {
			when("passed invalid cache-image", func() {
				it("fails", func() {
					opts := build.LifecycleOptions{
						Publish:      false,
						ClearCache:   false,
						RunImage:     "test",
						Image:        imageName,
						Builder:      fakeBuilder,
						TrustBuilder: false,
						UseCreator:   false,
						CacheImage:   "%%%",
					}

					lifecycle, err := build.NewLifecycleExecution(logger, docker, opts)
					h.AssertNil(t, err)

					err = lifecycle.Run(context.Background(), func(execution *build.LifecycleExecution) build.PhaseFactory {
						return fakePhaseFactory
					})

					h.AssertError(t, err, fmt.Sprintf("invalid cache image name: %s", "could not parse reference: %%!(NOVERB)"))
				})
			})
		})
	})

	when("#Create", func() {
		var (
			fakeBuildCache  *fakes.FakeCache
			fakeLaunchCache *fakes.FakeCache
		)
		it.Before(func() {
			fakeBuildCache = fakes.NewFakeCache()
			fakeBuildCache.ReturnForType = cache.Volume
			fakeBuildCache.ReturnForName = "some-cache"

			fakeLaunchCache = fakes.NewFakeCache()
			fakeLaunchCache.ReturnForType = cache.Volume
			fakeLaunchCache.ReturnForName = "some-launch-cache"
		})

		it("creates a phase and then run it", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Create(context.Background(), false, "", false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := newTestLifecycleExec(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedRepoName := "some-repo-name"
			expectedRunImage := "some-run-image"

			err := verboseLifecycle.Create(context.Background(), false, "", false, expectedRunImage, expectedRepoName, "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.Name(), "creator")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
				[]string{"-run-image", expectedRunImage},
				[]string{expectedRepoName},
			)
		})

		it("configures the phase with the expected network mode", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedNetworkMode := "some-network-mode"

			err := lifecycle.Create(context.Background(), false, "", false, "test", "test", expectedNetworkMode, fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
		})

		when("clear cache", func() {
			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycleExec(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := verboseLifecycle.Create(context.Background(), false, "", true, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "creator")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-skip-restore"},
				)
			})
		})

		when("clear cache is false", func() {
			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycleExec(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := verboseLifecycle.Create(context.Background(), false, "", false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "creator")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-cache-dir", "/cache"},
				)
			})
		})

		when("using a cache image", func() {
			it.Before(func() {
				fakeBuildCache.ReturnForType = cache.Image
				fakeBuildCache.ReturnForName = "some-cache-image"
			})
			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycleExec(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := verboseLifecycle.Create(context.Background(), false, "", true, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "creator")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-skip-restore"},
					[]string{"-cache-image", "some-cache-image"},
				)

				h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, ":/cache")
			})
		})

		when("additional tags are specified", func() {
			it("configures phases with additional tags", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				additionalTags := []string{"additional-tag-1", "additional-tag-2"}

				err := lifecycle.Create(context.Background(), false, "", false, "test", "test", "test", fakes.NewFakeCache(), fakes.NewFakeCache(), additionalTags, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-tag", additionalTags[0], "-tag", additionalTags[1]},
				)
			})
		})

		when("publish", func() {
			var (
				fakeBuildCache  *fakes.FakeCache
				fakeLaunchCache *fakes.FakeCache
			)
			it.Before(func() {
				fakeBuildCache = fakes.NewFakeCache()
				fakeBuildCache.ReturnForName = "some-cache"
				fakeBuildCache.ReturnForType = cache.Volume

				fakeLaunchCache = fakes.NewFakeCache()
				fakeLaunchCache.ReturnForType = cache.Volume
				fakeLaunchCache.ReturnForName = "some-launch-cache"
			})

			it("configures the phase with binds", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				volumeMount := "custom-mount-source:/custom-mount-target"
				expectedBinds := []string{volumeMount, "some-cache:/cache"}

				err := lifecycle.Create(context.Background(), true, "", false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{volumeMount}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)
			})

			it("configures the phase with root", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Create(context.Background(), true, "", false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
			})

			it("configures the phase with registry access", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepos := "some-repo-name"

				err := lifecycle.Create(context.Background(), true, "", false, "test", expectedRepos, "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_REGISTRY_AUTH={}")
			})

			when("using a cache image", func() {
				it.Before(func() {
					fakeBuildCache.ReturnForType = cache.Image
					fakeBuildCache.ReturnForName = "some-cache-image"
				})

				it("configures the phase with the expected arguments", func() {
					verboseLifecycle := newTestLifecycleExec(t, true)
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err := verboseLifecycle.Create(context.Background(), true, "", true, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertEq(t, configProvider.Name(), "creator")
					h.AssertIncludeAllExpectedPatterns(t,
						configProvider.ContainerConfig().Cmd,
						[]string{"-skip-restore"},
						[]string{"-cache-image", "some-cache-image"},
					)

					h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, ":/cache")
				})
			})

			when("platform 0.3", func() {
				var (
					fakeBuildCache  *fakes.FakeCache
					fakeLaunchCache *fakes.FakeCache
				)
				it.Before(func() {
					fakeBuildCache = fakes.NewFakeCache()
					fakeBuildCache.ReturnForName = "some-cache"
					fakeBuildCache.ReturnForType = cache.Volume

					fakeLaunchCache = fakes.NewFakeCache()
					fakeLaunchCache.ReturnForType = cache.Volume
					fakeLaunchCache.ReturnForName = "some-launch-cache"
				})

				it("doesn't hint at default process type", func() {
					fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("0.3")}))
					h.AssertNil(t, err)
					lifecycle := newTestLifecycleExec(t, true, fakes.WithBuilder(fakeBuilder))
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err = lifecycle.Export(context.Background(), "test", "test", true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertSliceNotContains(t, configProvider.ContainerConfig().Cmd, "-process-type")
				})
			})

			when("platform 0.4", func() {
				it("hints at default process type", func() {
					fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("0.4")}))
					h.AssertNil(t, err)
					lifecycle := newTestLifecycleExec(t, true, fakes.WithBuilder(fakeBuilder))
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err = lifecycle.Export(context.Background(), "test", "test", true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertIncludeAllExpectedPatterns(t, configProvider.ContainerConfig().Cmd, []string{"-process-type", "web"})
				})
			})
		})

		when("publish is false", func() {
			var (
				fakeBuildCache  *fakes.FakeCache
				fakeLaunchCache *fakes.FakeCache
			)
			it.Before(func() {
				fakeBuildCache = fakes.NewFakeCache()
				fakeBuildCache.ReturnForName = "some-cache"
				fakeBuildCache.ReturnForType = cache.Volume

				fakeLaunchCache = fakes.NewFakeCache()
				fakeLaunchCache.ReturnForType = cache.Volume
				fakeLaunchCache.ReturnForName = "some-launch-cache"
			})
			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycleExec(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := verboseLifecycle.Create(context.Background(), false, "", false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "creator")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-daemon"},
					[]string{"-launch-cache", "/launch-cache"},
				)
			})

			it("configures the phase with daemon access", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Create(context.Background(), false, "", false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
			})

			it("configures the phase with daemon access with tcp docker-host", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Create(context.Background(), false, "tcp://localhost:1234", false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "DOCKER_HOST=tcp://localhost:1234")
			})

			it("configures the phase with daemon access with alternative unix socket docker-host", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Create(context.Background(), false, "unix:///home/user/docker.sock", false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, "/home/user/docker.sock:/var/run/docker.sock")
			})

			it("configures the phase with daemon access with alternative windows pipe docker-host", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Create(context.Background(), false, `npipe:\\\\.\pipe\docker_engine_alt`, false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, "/home/user/docker.sock:/var/run/docker.sock")
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, `\\.\pipe\docker_engine_alt:\\.\pipe\docker_engine`)
			})

			when("environment variable DOCKER_HOST is set", func() {
				var (
					oldDH       string
					oldDHExists bool
				)
				it.Before(func() {
					oldDH, oldDHExists = os.LookupEnv("DOCKER_HOST")
					os.Setenv("DOCKER_HOST", "tcp://example.com:1234")
				})
				it.After(func() {
					if oldDHExists {
						os.Setenv("DOCKER_HOST", oldDH)
					} else {
						os.Unsetenv("DOCKER_HOST")
					}
				})
				it("configures the phase with daemon access with inherited docker-host", func() {
					lifecycle := newTestLifecycleExec(t, false)
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err := lifecycle.Create(context.Background(), false, `inherit`, false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "DOCKER_HOST=tcp://example.com:1234")
				})
			})

			it("configures the phase with daemon access with docker-host with unknown protocol", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				err := lifecycle.Create(context.Background(), false, `withoutprotocol`, false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "DOCKER_HOST=withoutprotocol")
			})

			it("configures the phase with binds", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				volumeMount := "custom-mount-source:/custom-mount-target"
				expectedBinds := []string{volumeMount, "some-cache:/cache", "some-launch-cache:/launch-cache"}

				err := lifecycle.Create(context.Background(), false, "", false, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{volumeMount}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)
			})

			when("platform 0.3", func() {
				it("doesn't hint at default process type", func() {
					fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("0.3")}))
					h.AssertNil(t, err)
					lifecycle := newTestLifecycleExec(t, true, fakes.WithBuilder(fakeBuilder))
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err = lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertSliceNotContains(t, configProvider.ContainerConfig().Cmd, "-process-type")
				})
			})

			when("platform 0.4", func() {
				it("hints at default process type", func() {
					fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("0.4")}))
					h.AssertNil(t, err)
					lifecycle := newTestLifecycleExec(t, true, fakes.WithBuilder(fakeBuilder))
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err = lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertIncludeAllExpectedPatterns(t, configProvider.ContainerConfig().Cmd, []string{"-process-type", "web"})
				})
			})
		})

		when("override GID", func() {
			when("override GID is provided", func() {
				it("configures the phase with the expected arguments", func() {
					verboseLifecycle := newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
						options.GID = 2
					})
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err := verboseLifecycle.Create(context.Background(), false, "", true, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertEq(t, configProvider.Name(), "creator")
					h.AssertIncludeAllExpectedPatterns(t,
						configProvider.ContainerConfig().Cmd,
						[]string{"-gid", "2"},
					)
				})
			})
			when("override GID is not provided", func() {
				it("gid is not added to the expected arguments", func() {
					verboseLifecycle := newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
						options.GID = -1
					})
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err := verboseLifecycle.Create(context.Background(), false, "", true, "test", "test", "test", fakeBuildCache, fakeLaunchCache, []string{}, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertEq(t, configProvider.Name(), "creator")
					h.AssertSliceNotContains(t, configProvider.ContainerConfig().Cmd, "-gid")
				})
			})
		})
	})

	when("#Detect", func() {
		it("creates a phase and then runs it", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Detect(context.Background(), "test", []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := newTestLifecycleExec(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := verboseLifecycle.Detect(context.Background(), "test", []string{"test"}, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.Name(), "detector")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
			)
		})

		it("configures the phase with the expected network mode", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedNetworkMode := "some-network-mode"

			err := lifecycle.Detect(context.Background(), expectedNetworkMode, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
		})

		it("configures the phase to copy app dir", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedBind := "some-mount-source:/some-mount-target"

			err := lifecycle.Detect(context.Background(), "test", []string{expectedBind}, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)

			h.AssertEq(t, len(configProvider.ContainerOps()), 2)
			h.AssertFunctionName(t, configProvider.ContainerOps()[0], "EnsureVolumeAccess")
			h.AssertFunctionName(t, configProvider.ContainerOps()[1], "CopyDir")
		})
	})

	when("#Analyze", func() {
		var fakeCache *fakes.FakeCache
		it.Before(func() {
			fakeCache = fakes.NewFakeCache()
			fakeCache.ReturnForType = cache.Volume
		})
		it("creates a phase and then runs it", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Analyze(context.Background(), "test", "test", false, "", false, fakeCache, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		when("clear cache", func() {
			it("configures the phase with the expected arguments", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := lifecycle.Analyze(context.Background(), expectedRepoName, "test", false, "", true, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Cmd, "-skip-layers")
			})
		})

		when("clear cache is false", func() {
			it("configures the phase with the expected arguments", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := lifecycle.Analyze(context.Background(), expectedRepoName, "test", false, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-cache-dir", "/cache"},
				)
			})
		})

		when("using a cache image", func() {
			var (
				lifecycle        *build.LifecycleExecution
				fakePhaseFactory *fakes.FakePhaseFactory
				expectedRepoName = "some-repo-name"
			)
			it.Before(func() {
				fakeCache.ReturnForType = cache.Image
				fakeCache.ReturnForName = "some-cache-image"

				lifecycle = newTestLifecycleExec(t, false, func(options *build.LifecycleOptions) {
					options.GID = -1
				})
				fakePhaseFactory = fakes.NewFakePhaseFactory()
			})
			it("configures the phase with a build cache images", func() {
				err := lifecycle.Analyze(context.Background(), expectedRepoName, "", false, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, ":/cache")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-cache-image", "some-cache-image"},
				)
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-cache-dir", "/cache"},
				)
			})
			when("clear-cache", func() {
				it("cache is omitted from Analyze", func() {
					err := lifecycle.Analyze(context.Background(), expectedRepoName, "", false, "", true, fakeCache, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertEq(t, configProvider.Name(), "analyzer")
					h.AssertSliceNotContains(t, configProvider.ContainerConfig().Cmd, "-cache-image")
				})
			})
		})

		when("publish", func() {
			it("runs the phase with the lifecycle image", func() {
				lifecycle := newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
					options.LifecycleImage = "some-lifecycle-image"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", true, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.ContainerConfig().Image, "some-lifecycle-image")
			})

			it("sets the CNB_USER_ID and CNB_GROUP_ID in the environment", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithUID(2222), fakes.WithGID(3333))
				h.AssertNil(t, err)
				lifecycle := newTestLifecycleExec(t, false, fakes.WithBuilder(fakeBuilder))
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err = lifecycle.Analyze(context.Background(), "test", "test", true, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_USER_ID=2222")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_GROUP_ID=3333")
			})

			it("configures the phase with registry access", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepos := "some-repo-name"
				expectedNetworkMode := "some-network-mode"

				err := lifecycle.Analyze(context.Background(), expectedRepos, expectedNetworkMode, true, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_REGISTRY_AUTH={}")
				h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
			})

			it("configures the phase with root", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", true, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
			})

			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycleExec(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := verboseLifecycle.Analyze(context.Background(), expectedRepoName, "test", true, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-log-level", "debug"},
					[]string{expectedRepoName},
				)
			})

			it("configures the phase with binds", func() {
				fakeCache.ReturnForName = "some-cache"
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBind := "some-cache:/cache"

				err := lifecycle.Analyze(context.Background(), "test", "test", true, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
			})

			when("using a cache image", func() {
				it.Before(func() {
					fakeCache.ReturnForName = "some-cache-image"
					fakeCache.ReturnForType = cache.Image
				})

				it("configures the phase with a build cache images", func() {
					lifecycle := newTestLifecycleExec(t, false, func(options *build.LifecycleOptions) {
						options.GID = -1
					})
					fakePhaseFactory := fakes.NewFakePhaseFactory()
					expectedRepoName := "some-repo-name"

					err := lifecycle.Analyze(context.Background(), expectedRepoName, "test", true, "", false, fakeCache, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, ":/cache")
					h.AssertIncludeAllExpectedPatterns(t,
						configProvider.ContainerConfig().Cmd,
						[]string{"-cache-image", "some-cache-image"},
					)
					h.AssertIncludeAllExpectedPatterns(t,
						configProvider.ContainerConfig().Cmd,
						[]string{"-cache-dir", "/cache"},
					)
				})
			})
		})

		when("publish is false", func() {
			it("runs the phase with the lifecycle image", func() {
				lifecycle := newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
					options.LifecycleImage = "some-lifecycle-image"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", false, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.ContainerConfig().Image, "some-lifecycle-image")
			})

			it("sets the CNB_USER_ID and CNB_GROUP_ID in the environment", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithUID(2222), fakes.WithGID(3333))
				h.AssertNil(t, err)
				lifecycle := newTestLifecycleExec(t, false, fakes.WithBuilder(fakeBuilder))
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err = lifecycle.Analyze(context.Background(), "test", "test", false, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_USER_ID=2222")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_GROUP_ID=3333")
			})

			it("configures the phase with daemon access", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", false, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
			})

			it("configures the phase with daemon access with TCP docker-host", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Analyze(context.Background(), "test", "test", false, "tcp://localhost:1234", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "DOCKER_HOST=tcp://localhost:1234")
			})

			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycleExec(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"

				err := verboseLifecycle.Analyze(context.Background(), expectedRepoName, "test", false, "", true, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "analyzer")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-log-level", "debug"},
					[]string{"-daemon"},
					[]string{expectedRepoName},
				)
			})

			it("configures the phase with the expected network mode", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedNetworkMode := "some-network-mode"

				err := lifecycle.Analyze(context.Background(), "test", expectedNetworkMode, false, "", false, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
			})

			it("configures the phase with binds", func() {
				fakeCache.ReturnForName = "some-cache"
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBind := "some-cache:/cache"

				err := lifecycle.Analyze(context.Background(), "test", "test", false, "", true, fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
			})
		})

		when("override GID", func() {
			var (
				lifecycle        *build.LifecycleExecution
				fakePhaseFactory *fakes.FakePhaseFactory
			)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory = fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			when("override GID is provided", func() {
				it.Before(func() {
					lifecycle = newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
						options.GID = 2
					})
				})
				it("configures the phase with the expected arguments", func() {
					err := lifecycle.Analyze(context.Background(), "test", "test", false, "", false, fakeCache, fakePhaseFactory)
					h.AssertNil(t, err)
					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)
					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertIncludeAllExpectedPatterns(t,
						configProvider.ContainerConfig().Cmd,
						[]string{"-gid", "2"},
					)
				})
			})
			when("override GID is not provided", func() {
				it.Before(func() {
					lifecycle = newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
						options.GID = -1
					})
				})
				it("gid is not added to the expected arguments", func() {
					err := lifecycle.Analyze(context.Background(), "test", "test", false, "", false, fakeCache, fakePhaseFactory)
					h.AssertNil(t, err)
					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)
					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertSliceNotContains(t, configProvider.ContainerConfig().Cmd, "-gid")
				})
			})
		})
	})

	when("#Restore", func() {
		var fakeCache *fakes.FakeCache
		it.Before(func() {
			fakeCache = fakes.NewFakeCache()
			fakeCache.ReturnForName = "some-cache"
			fakeCache.ReturnForType = cache.Volume
		})
		it("runs the phase with the lifecycle image", func() {
			lifecycle := newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
				options.LifecycleImage = "some-lifecycle-image"
			})
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := lifecycle.Restore(context.Background(), "test", fakeCache, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.ContainerConfig().Image, "some-lifecycle-image")
		})

		it("sets the CNB_USER_ID and CNB_GROUP_ID in the environment", func() {
			fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithUID(2222), fakes.WithGID(3333))
			h.AssertNil(t, err)
			lifecycle := newTestLifecycleExec(t, false, fakes.WithBuilder(fakeBuilder))
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err = lifecycle.Restore(context.Background(), "test", fakeCache, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_USER_ID=2222")
			h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_GROUP_ID=3333")
		})

		it("creates a phase and then runs it", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Restore(context.Background(), "test", fakeCache, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with root access", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := lifecycle.Restore(context.Background(), "test", fakeCache, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.ContainerConfig().User, "root")
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := newTestLifecycleExec(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err := verboseLifecycle.Restore(context.Background(), "test", fakeCache, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.Name(), "restorer")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
				[]string{"-cache-dir", "/cache"},
			)
		})

		it("configures the phase with the expected network mode", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedNetworkMode := "some-network-mode"

			err := lifecycle.Restore(context.Background(), expectedNetworkMode, fakeCache, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
		})

		it("configures the phase with binds", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedBind := "some-cache:/cache"

			err := lifecycle.Restore(context.Background(), "test", fakeCache, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
		})

		when("using cache image", func() {
			var (
				lifecycle        *build.LifecycleExecution
				fakePhaseFactory *fakes.FakePhaseFactory
			)

			it.Before(func() {
				fakeCache.ReturnForType = cache.Image
				fakeCache.ReturnForName = "some-cache-image"

				lifecycle = newTestLifecycleExec(t, false, func(options *build.LifecycleOptions) {
					options.GID = -1
				})
				fakePhaseFactory = fakes.NewFakePhaseFactory()
			})
			it("configures the phase with a cache image", func() {
				err := lifecycle.Restore(context.Background(), "test", fakeCache, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, ":/cache")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-cache-image", "some-cache-image"},
				)
			})
		})

		when("override GID", func() {
			var (
				lifecycle        *build.LifecycleExecution
				fakePhaseFactory *fakes.FakePhaseFactory
			)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory = fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			when("override GID is provided", func() {
				it.Before(func() {
					lifecycle = newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
						options.GID = 2
					})
				})
				it("configures the phase with the expected arguments", func() {
					err := lifecycle.Restore(context.Background(), "test", fakeCache, fakePhaseFactory)
					h.AssertNil(t, err)
					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)
					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertIncludeAllExpectedPatterns(t,
						configProvider.ContainerConfig().Cmd,
						[]string{"-gid", "2"},
					)
				})
			})
			when("override GID is not provided", func() {
				it.Before(func() {
					lifecycle = newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
						options.GID = -1
					})
				})
				it("gid is not added to the expected arguments", func() {
					err := lifecycle.Restore(context.Background(), "test", fakeCache, fakePhaseFactory)
					h.AssertNil(t, err)
					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)
					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertSliceNotContains(t, configProvider.ContainerConfig().Cmd, "-gid")
				})
			})
		})
	})

	when("#Build", func() {
		it("creates a phase and then runs it", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Build(context.Background(), "test", []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			fakeBuilder, err := fakes.NewFakeBuilder()
			h.AssertNil(t, err)
			verboseLifecycle := newTestLifecycleExec(t, true, fakes.WithBuilder(fakeBuilder))
			fakePhaseFactory := fakes.NewFakePhaseFactory()

			err = verboseLifecycle.Build(context.Background(), "test", []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.Name(), "builder")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
			)
		})

		it("configures the phase with the expected network mode", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedNetworkMode := "some-network-mode"

			err := lifecycle.Build(context.Background(), expectedNetworkMode, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
		})

		it("configures the phase with binds", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedBind := "some-mount-source:/some-mount-target"

			err := lifecycle.Build(context.Background(), "test", []string{expectedBind}, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
		})
	})

	when("#Export", func() {
		var (
			fakeBuildCache  *fakes.FakeCache
			fakeLaunchCache *fakes.FakeCache
		)

		it.Before(func() {
			fakeBuildCache = fakes.NewFakeCache()
			fakeBuildCache.ReturnForType = cache.Volume
			fakeBuildCache.ReturnForName = "some-cache"

			fakeLaunchCache = fakes.NewFakeCache()
			fakeLaunchCache.ReturnForType = cache.Volume
			fakeLaunchCache.ReturnForName = "some-launch-cache"
		})

		it("creates a phase and then runs it", func() {
			lifecycle := newTestLifecycleExec(t, false)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory := fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			err := lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			h.AssertEq(t, fakePhase.CleanupCallCount, 1)
			h.AssertEq(t, fakePhase.RunCallCount, 1)
		})

		it("configures the phase with the expected arguments", func() {
			verboseLifecycle := newTestLifecycleExec(t, true)
			fakePhaseFactory := fakes.NewFakePhaseFactory()
			expectedRepoName := "some-repo-name"
			expectedRunImage := "some-run-image"

			err := verboseLifecycle.Export(context.Background(), expectedRepoName, expectedRunImage, false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
			h.AssertNil(t, err)

			lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
			h.AssertNotEq(t, lastCallIndex, -1)

			configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
			h.AssertEq(t, configProvider.Name(), "exporter")
			h.AssertIncludeAllExpectedPatterns(t,
				configProvider.ContainerConfig().Cmd,
				[]string{"-log-level", "debug"},
				[]string{"-cache-dir", "/cache"},
				[]string{"-run-image", expectedRunImage},
				[]string{expectedRepoName},
			)
		})

		when("additional tags are specified", func() {
			it("passes tag arguments to the exporter", func() {
				verboseLifecycle := newTestLifecycleExec(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"
				expectedRunImage := "some-run-image"
				additionalTags := []string{"additional-tag-1", "additional-tag-2"}

				err := verboseLifecycle.Export(context.Background(), expectedRepoName, expectedRunImage, false, "", "test", fakes.NewFakeCache(), fakes.NewFakeCache(), additionalTags, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "exporter")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-log-level", "debug"},
					[]string{"-cache-dir", "/cache"},
					[]string{"-run-image", expectedRunImage},
					[]string{expectedRepoName, additionalTags[0], additionalTags[1]},
				)
			})
		})

		when("using cache image", func() {
			it.Before(func() {
				fakeBuildCache.ReturnForType = cache.Image
				fakeBuildCache.ReturnForName = "some-cache-image"
			})

			it("configures phase with cache image", func() {
				verboseLifecycle := newTestLifecycleExec(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepoName := "some-repo-name"
				expectedRunImage := "some-run-image"

				err := verboseLifecycle.Export(context.Background(), expectedRepoName, expectedRunImage, false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "exporter")

				h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, ":/cache")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-cache-image", "some-cache-image"},
				)
			})
		})

		when("publish", func() {
			it("runs the phase with the lifecycle image", func() {
				lifecycle := newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
					options.LifecycleImage = "some-lifecycle-image"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.ContainerConfig().Image, "some-lifecycle-image")
			})

			it("sets the CNB_USER_ID and CNB_GROUP_ID in the environment", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithUID(2222), fakes.WithGID(3333))
				h.AssertNil(t, err)
				lifecycle := newTestLifecycleExec(t, false, fakes.WithBuilder(fakeBuilder))
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err = lifecycle.Export(context.Background(), "test", "test", true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_USER_ID=2222")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_GROUP_ID=3333")
			})

			it("configures the phase with registry access", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedRepos := []string{"some-repo-name", "some-run-image"}

				err := lifecycle.Export(context.Background(), expectedRepos[0], expectedRepos[1], true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_REGISTRY_AUTH={}")
			})

			it("configures the phase with root", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
			})

			it("configures the phase with the expected network mode", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedNetworkMode := "some-network-mode"

				err := lifecycle.Export(context.Background(), "test", "test", true, "", expectedNetworkMode, fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
			})

			it("configures the phase with binds", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBind := "some-cache:/cache"

				err := lifecycle.Export(context.Background(), "test", "test", true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBind)
			})

			it("configures the phase to write stack toml", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBinds := []string{"some-cache:/cache", "some-launch-cache:/launch-cache"}

				err := lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)

				h.AssertEq(t, len(configProvider.ContainerOps()), 1)
				h.AssertFunctionName(t, configProvider.ContainerOps()[0], "WriteStackToml")
			})

			it("configures the phase with default process type", func() {
				lifecycle := newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
					options.DefaultProcessType = "test-process"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedDefaultProc := []string{"-process-type", "test-process"}

				err := lifecycle.Export(context.Background(), "test", "test", true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertIncludeAllExpectedPatterns(t, configProvider.ContainerConfig().Cmd, expectedDefaultProc)
			})

			when("using cache image and publishing", func() {
				it.Before(func() {
					fakeBuildCache.ReturnForType = cache.Image
					fakeBuildCache.ReturnForName = "some-cache-image"
				})

				it("configures phase with cache image", func() {
					verboseLifecycle := newTestLifecycleExec(t, true)
					fakePhaseFactory := fakes.NewFakePhaseFactory()
					expectedRepoName := "some-repo-name"
					expectedRunImage := "some-run-image"

					err := verboseLifecycle.Export(context.Background(), expectedRepoName, expectedRunImage, true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertEq(t, configProvider.Name(), "exporter")

					h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, ":/cache")
					h.AssertIncludeAllExpectedPatterns(t,
						configProvider.ContainerConfig().Cmd,
						[]string{"-cache-image", "some-cache-image"},
					)
				})
			})

			when("platform 0.3", func() {
				it("doesn't hint at default process type", func() {
					fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("0.3")}))
					h.AssertNil(t, err)
					lifecycle := newTestLifecycleExec(t, true, fakes.WithBuilder(fakeBuilder))
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err = lifecycle.Export(context.Background(), "test", "test", true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertSliceNotContains(t, configProvider.ContainerConfig().Cmd, "-process-type")
				})
			})

			when("platform 0.4", func() {
				it("hints at default process type", func() {
					fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("0.4")}))
					h.AssertNil(t, err)
					lifecycle := newTestLifecycleExec(t, true, fakes.WithBuilder(fakeBuilder))
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err = lifecycle.Export(context.Background(), "test", "test", true, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertIncludeAllExpectedPatterns(t, configProvider.ContainerConfig().Cmd, []string{"-process-type", "web"})
				})
			})
		})

		when("publish is false", func() {
			it("runs the phase with the lifecycle image", func() {
				lifecycle := newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
					options.LifecycleImage = "some-lifecycle-image"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.ContainerConfig().Image, "some-lifecycle-image")
			})

			it("sets the CNB_USER_ID and CNB_GROUP_ID in the environment", func() {
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithUID(2222), fakes.WithGID(3333))
				h.AssertNil(t, err)
				lifecycle := newTestLifecycleExec(t, false, fakes.WithBuilder(fakeBuilder))
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err = lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_USER_ID=2222")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "CNB_GROUP_ID=3333")
			})

			it("configures the phase with daemon access", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.ContainerConfig().User, "root")
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
			})

			it("configures the phase with daemon access with tcp docker-host", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := lifecycle.Export(context.Background(), "test", "test", false, "tcp://localhost:1234", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceNotContains(t, configProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
				h.AssertSliceContains(t, configProvider.ContainerConfig().Env, "DOCKER_HOST=tcp://localhost:1234")
			})

			it("configures the phase with the expected arguments", func() {
				verboseLifecycle := newTestLifecycleExec(t, true)
				fakePhaseFactory := fakes.NewFakePhaseFactory()

				err := verboseLifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.Name(), "exporter")
				h.AssertIncludeAllExpectedPatterns(t,
					configProvider.ContainerConfig().Cmd,
					[]string{"-daemon"},
					[]string{"-launch-cache", "/launch-cache"},
				)
			})

			it("configures the phase with the expected network mode", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedNetworkMode := "some-network-mode"

				err := lifecycle.Export(context.Background(), "test", "test", false, "", expectedNetworkMode, fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertEq(t, configProvider.HostConfig().NetworkMode, container.NetworkMode(expectedNetworkMode))
			})

			it("configures the phase with binds", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBinds := []string{"some-cache:/cache", "some-launch-cache:/launch-cache"}

				err := lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)
			})

			it("configures the phase to write stack toml", func() {
				lifecycle := newTestLifecycleExec(t, false)
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedBinds := []string{"some-cache:/cache", "some-launch-cache:/launch-cache"}

				err := lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertSliceContains(t, configProvider.HostConfig().Binds, expectedBinds...)

				h.AssertEq(t, len(configProvider.ContainerOps()), 1)
				h.AssertFunctionName(t, configProvider.ContainerOps()[0], "WriteStackToml")
			})

			it("configures the phase with default process type", func() {
				lifecycle := newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
					options.DefaultProcessType = "test-process"
				})
				fakePhaseFactory := fakes.NewFakePhaseFactory()
				expectedDefaultProc := []string{"-process-type", "test-process"}

				err := lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
				h.AssertNil(t, err)

				lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
				h.AssertNotEq(t, lastCallIndex, -1)

				configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
				h.AssertIncludeAllExpectedPatterns(t, configProvider.ContainerConfig().Cmd, expectedDefaultProc)
			})

			when("platform 0.3", func() {
				it("doesn't hint at default process type", func() {
					fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("0.3")}))
					h.AssertNil(t, err)
					lifecycle := newTestLifecycleExec(t, true, fakes.WithBuilder(fakeBuilder))
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err = lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertSliceNotContains(t, configProvider.ContainerConfig().Cmd, "-process-type")
				})
			})

			when("platform 0.4", func() {
				it("hints at default process type", func() {
					fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithSupportedPlatformAPIs([]*api.Version{api.MustParse("0.4")}))
					h.AssertNil(t, err)
					lifecycle := newTestLifecycleExec(t, true, fakes.WithBuilder(fakeBuilder))
					fakePhaseFactory := fakes.NewFakePhaseFactory()

					err = lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)

					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)

					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertIncludeAllExpectedPatterns(t, configProvider.ContainerConfig().Cmd, []string{"-process-type", "web"})
				})
			})
		})

		when("override GID", func() {
			var (
				lifecycle        *build.LifecycleExecution
				fakePhaseFactory *fakes.FakePhaseFactory
			)
			fakePhase := &fakes.FakePhase{}
			fakePhaseFactory = fakes.NewFakePhaseFactory(fakes.WhichReturnsForNew(fakePhase))

			when("override GID is provided", func() {
				it.Before(func() {
					lifecycle = newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
						options.GID = 2
					})
				})
				it("configures the phase with the expected arguments", func() {
					err := lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)
					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)
					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertIncludeAllExpectedPatterns(t,
						configProvider.ContainerConfig().Cmd,
						[]string{"-gid", "2"},
					)
				})
			})
			when("override GID is not provided", func() {
				it.Before(func() {
					lifecycle = newTestLifecycleExec(t, true, func(options *build.LifecycleOptions) {
						options.GID = -1
					})
				})
				it("gid is not added to the expected arguments", func() {
					err := lifecycle.Export(context.Background(), "test", "test", false, "", "test", fakeBuildCache, fakeLaunchCache, []string{}, fakePhaseFactory)
					h.AssertNil(t, err)
					lastCallIndex := len(fakePhaseFactory.NewCalledWithProvider) - 1
					h.AssertNotEq(t, lastCallIndex, -1)
					configProvider := fakePhaseFactory.NewCalledWithProvider[lastCallIndex]
					h.AssertSliceNotContains(t, configProvider.ContainerConfig().Cmd, "-gid")
				})
			})
		})
	})
}

func newTestLifecycleExecErr(t *testing.T, logVerbose bool, ops ...func(*build.LifecycleOptions)) (*build.LifecycleExecution, error) {
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)

	var outBuf bytes.Buffer
	logger := ilogging.NewLogWithWriters(&outBuf, &outBuf)
	if logVerbose {
		logger.Level = log.DebugLevel
	}

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

	return build.NewLifecycleExecution(logger, docker, opts)
}

func newTestLifecycleExec(t *testing.T, logVerbose bool, ops ...func(*build.LifecycleOptions)) *build.LifecycleExecution {
	t.Helper()

	lifecycleExec, err := newTestLifecycleExecErr(t, logVerbose, ops...)
	h.AssertNil(t, err)
	return lifecycleExec
}
