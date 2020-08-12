package build_test

import (
	"math/rand"
	"testing"
	"time"

	ifakes "github.com/buildpacks/imgutil/fakes"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/build/fakes"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestPhaseConfigProvider(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "phase_config_provider", testPhaseConfigProvider, spec.Report(report.Terminal{}), spec.Sequential())
}

func testPhaseConfigProvider(t *testing.T, when spec.G, it spec.S) {
	when("#NewPhaseConfigProvider", func() {
		it("returns a phase config provider with defaults", func() {
			expectedBuilderImage := ifakes.NewImage("some-builder-name", "", nil)
			fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithImage(expectedBuilderImage))
			h.AssertNil(t, err)
			lifecycle := newTestLifecycle(t, false, fakes.WithBuilder(fakeBuilder))
			expectedPhaseName := "some-name"
			expectedCmd := strslice.StrSlice{"/cnb/lifecycle/" + expectedPhaseName}

			phaseConfigProvider := build.NewPhaseConfigProvider(expectedPhaseName, lifecycle)

			h.AssertEq(t, phaseConfigProvider.Name(), expectedPhaseName)
			h.AssertEq(t, phaseConfigProvider.ContainerConfig().Cmd, expectedCmd)
			h.AssertEq(t, phaseConfigProvider.ContainerConfig().Image, expectedBuilderImage.Name())
			h.AssertEq(t, phaseConfigProvider.ContainerConfig().Labels, map[string]string{"author": "pack"})

			// CreateFakeLifecycleExecution sets the following:
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "HTTP_PROXY=some-http-proxy")
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "http_proxy=some-http-proxy")
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "HTTPS_PROXY=some-https-proxy")
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "https_proxy=some-https-proxy")
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "NO_PROXY=some-no-proxy")
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "no_proxy=some-no-proxy")

			h.AssertSliceContainsMatch(t, phaseConfigProvider.HostConfig().Binds, "pack-layers-.*:/layers")
			h.AssertSliceContainsMatch(t, phaseConfigProvider.HostConfig().Binds, "pack-app-.*:/workspace")

			h.AssertEq(t, phaseConfigProvider.HostConfig().Isolation, container.IsolationEmpty)
		})

		when("building for Windows", func() {
			it("sets process isolation", func() {
				fakeBuilderImage := ifakes.NewImage("fake-builder", "", nil)
				fakeBuilderImage.SetPlatform("windows", "", "")
				fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithImage(fakeBuilderImage))
				h.AssertNil(t, err)
				lifecycle := newTestLifecycle(t, false, fakes.WithBuilder(fakeBuilder))

				phaseConfigProvider := build.NewPhaseConfigProvider("some-name", lifecycle)

				h.AssertEq(t, phaseConfigProvider.HostConfig().Isolation, container.IsolationProcess)
			})
		})

		when("called with WithArgs", func() {
			it("sets args on the config", func() {
				lifecycle := newTestLifecycle(t, false)
				expectedArgs := strslice.StrSlice{"some-arg-1", "some-arg-2"}

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithArgs(expectedArgs...),
				)

				cmd := phaseConfigProvider.ContainerConfig().Cmd
				h.AssertSliceContainsInOrder(t, cmd, "some-arg-1", "some-arg-2")
			})
		})

		when("called with WithFlags", func() {
			it("sets args on the config", func() {
				lifecycle := newTestLifecycle(t, false)

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithArgs("arg-1", "arg-2"),
					build.WithFlags("flag-1", "flag-2"),
				)

				cmd := phaseConfigProvider.ContainerConfig().Cmd
				h.AssertSliceContainsInOrder(t, cmd, "flag-1", "flag-2", "arg-1", "arg-2")
			})
		})

		when("called with WithBinds", func() {
			it("sets binds on the config", func() {
				lifecycle := newTestLifecycle(t, false)
				expectedBinds := []string{"some-bind-1", "some-bind-2"}

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithBinds(expectedBinds...),
				)

				h.AssertSliceContains(t, phaseConfigProvider.HostConfig().Binds, expectedBinds...)
			})
		})

		when("called with WithDaemonAccess", func() {
			when("building for non-Windows", func() {
				it("sets daemon access on the config", func() {
					lifecycle := newTestLifecycle(t, false)

					phaseConfigProvider := build.NewPhaseConfigProvider(
						"some-name",
						lifecycle,
						build.WithDaemonAccess(),
					)

					h.AssertEq(t, phaseConfigProvider.ContainerConfig().User, "root")
					h.AssertSliceContains(t, phaseConfigProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
				})
			})

			when("building for Windows", func() {
				it("sets daemon access on the config", func() {
					fakeBuilderImage := ifakes.NewImage("fake-builder", "", nil)
					fakeBuilderImage.SetPlatform("windows", "", "")
					fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithImage(fakeBuilderImage))
					h.AssertNil(t, err)
					lifecycle := newTestLifecycle(t, false, fakes.WithBuilder(fakeBuilder))

					phaseConfigProvider := build.NewPhaseConfigProvider(
						"some-name",
						lifecycle,
						build.WithDaemonAccess(),
					)

					h.AssertEq(t, phaseConfigProvider.ContainerConfig().User, "ContainerAdministrator")
					h.AssertSliceContains(t, phaseConfigProvider.HostConfig().Binds, `\\.\pipe\docker_engine:\\.\pipe\docker_engine`)
				})
			})
		})

		when("called with WithEnv", func() {
			it("sets the environment on the config", func() {
				lifecycle := newTestLifecycle(t, false)

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithEnv("SOME_VARIABLE=some-value"),
				)

				h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "SOME_VARIABLE=some-value")
			})
		})

		when("called with WithImage", func() {
			it("sets the image on the config", func() {
				lifecycle := newTestLifecycle(t, false)

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithImage("some-image-name"),
				)

				h.AssertEq(t, phaseConfigProvider.ContainerConfig().Image, "some-image-name")
			})
		})

		when("called with WithNetwork", func() {
			it("sets the network mode on the config", func() {
				lifecycle := newTestLifecycle(t, false)
				expectedNetworkMode := "some-network-mode"

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithNetwork(expectedNetworkMode),
				)

				h.AssertEq(
					t,
					phaseConfigProvider.HostConfig().NetworkMode,
					container.NetworkMode(expectedNetworkMode),
				)
			})
		})

		when("called with WithRegistryAccess", func() {
			it("sets registry access on the config", func() {
				lifecycle := newTestLifecycle(t, false)
				authConfig := "some-auth-config"

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithRegistryAccess(authConfig),
				)

				h.AssertSliceContains(
					t,
					phaseConfigProvider.ContainerConfig().Env,
					"CNB_REGISTRY_AUTH="+authConfig,
				)
			})
		})

		when("called with WithRoot", func() {
			when("building for non-Windows", func() {
				it("sets root user on the config", func() {
					lifecycle := newTestLifecycle(t, false)

					phaseConfigProvider := build.NewPhaseConfigProvider(
						"some-name",
						lifecycle,
						build.WithRoot(),
					)

					h.AssertEq(t, phaseConfigProvider.ContainerConfig().User, "root")
				})
			})

			when("building for Windows", func() {
				it("sets root user on the config", func() {
					fakeBuilderImage := ifakes.NewImage("fake-builder", "", nil)
					fakeBuilderImage.SetPlatform("windows", "", "")
					fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithImage(fakeBuilderImage))
					h.AssertNil(t, err)
					lifecycle := newTestLifecycle(t, false, fakes.WithBuilder(fakeBuilder))

					phaseConfigProvider := build.NewPhaseConfigProvider(
						"some-name",
						lifecycle,
						build.WithRoot(),
					)

					h.AssertEq(t, phaseConfigProvider.ContainerConfig().User, "ContainerAdministrator")
				})
			})
		})

		when("called with WithLogPrefix", func() {
			it("sets prefix writers", func() {
				lifecycle := newTestLifecycle(t, false)

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithLogPrefix("some-prefix"),
				)

				_, isType := phaseConfigProvider.InfoWriter().(*logging.PrefixWriter)
				h.AssertEq(t, isType, true)

				_, isType = phaseConfigProvider.ErrorWriter().(*logging.PrefixWriter)
				h.AssertEq(t, isType, true)
			})
		})
	})
}
