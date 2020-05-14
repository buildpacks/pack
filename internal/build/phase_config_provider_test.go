package build_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/buildpacks/pack/internal/build/fakes"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/strslice"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/build"
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
			expectedBuilderName := "some-builder-name"
			fakeBuilder, err := fakes.NewFakeBuilder(fakes.WithName(expectedBuilderName))
			h.AssertNil(t, err)
			lifecycle := newLifecycle(t, false, fakes.WithBuilder(fakeBuilder))
			expectedPhaseName := "some-name"
			expectedCmd := strslice.StrSlice{"/cnb/lifecycle/" + expectedPhaseName}

			phaseConfigProvider := build.NewPhaseConfigProvider(expectedPhaseName, lifecycle)

			h.AssertEq(t, phaseConfigProvider.Name(), expectedPhaseName)
			h.AssertEq(t, phaseConfigProvider.ContainerConfig().Cmd, expectedCmd)
			h.AssertEq(t, phaseConfigProvider.ContainerConfig().Image, expectedBuilderName)
			h.AssertEq(t, phaseConfigProvider.ContainerConfig().Labels, map[string]string{"author": "pack"})

			// CreateFakeLifecycle sets the following:
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "HTTP_PROXY=some-http-proxy")
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "http_proxy=some-http-proxy")
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "HTTPS_PROXY=some-https-proxy")
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "https_proxy=some-https-proxy")
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "NO_PROXY=some-no-proxy")
			h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Env, "no_proxy=some-no-proxy")

			h.AssertSliceContainsMatch(t, phaseConfigProvider.HostConfig().Binds, "pack-layers-.*:/layers")
			h.AssertSliceContainsMatch(t, phaseConfigProvider.HostConfig().Binds, "pack-app-.*:/workspace")
		})

		when("called with WithArgs", func() {
			it("sets args on the config", func() {
				lifecycle := newLifecycle(t, false)
				expectedArgs := strslice.StrSlice{"some-arg-1", "some-arg-2"}

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithArgs(expectedArgs...),
				)

				h.AssertSliceContains(t, phaseConfigProvider.ContainerConfig().Cmd, expectedArgs...)
			})
		})

		when("called with WithBinds", func() {
			it("sets binds on the config", func() {
				lifecycle := newLifecycle(t, false)
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
			it("sets daemon access on the config", func() {
				lifecycle := newLifecycle(t, false)

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithDaemonAccess(),
				)

				h.AssertEq(t, phaseConfigProvider.ContainerConfig().User, "root")
				h.AssertSliceContains(t, phaseConfigProvider.HostConfig().Binds, "/var/run/docker.sock:/var/run/docker.sock")
			})
		})

		when("called with WithEnv", func() {
			it("sets the environment on the config", func() {
				lifecycle := newLifecycle(t, false)

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
				lifecycle := newLifecycle(t, false)

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithImage("some-image-name"),
				)

				h.AssertEq(t, phaseConfigProvider.ContainerConfig().Image, "some-image-name")
			})
		})

		when("called with WithMounts", func() {
			it("sets the mounts on the config", func() {
				lifecycle := newLifecycle(t, false)

				expectedMount := mount.Mount{Type: "bind", Source: "some-source", Target: "some-target", ReadOnly: true}

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithMounts(expectedMount),
				)

				h.AssertEq(t, phaseConfigProvider.HostConfig().Mounts[0], expectedMount)
			})
		})

		when("called with WithNetwork", func() {
			it("sets the network mode on the config", func() {
				lifecycle := newLifecycle(t, false)
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
				lifecycle := newLifecycle(t, false)
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
			it("sets root user on the config", func() {
				lifecycle := newLifecycle(t, false)

				phaseConfigProvider := build.NewPhaseConfigProvider(
					"some-name",
					lifecycle,
					build.WithRoot(),
				)

				h.AssertEq(t, phaseConfigProvider.ContainerConfig().User, "root")
			})
		})
	})
}
