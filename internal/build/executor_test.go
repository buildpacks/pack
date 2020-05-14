package build_test

import (
	"bytes"
	"context"
	ilogging "github.com/buildpacks/pack/internal/logging"
	dockercli "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"math/rand"
	"testing"
	"time"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/build/fakes"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestExecutor(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "executor", testExecutor, spec.Report(report.Terminal{}), spec.Sequential())
}

func testExecutor(t *testing.T, when spec.G, it spec.S) {
	when("#Execute", func() {
		var logger *ilogging.LogWithWriters
		var imageRef name.Reference

		it.Before(func() {
			var err error

			var outBuf bytes.Buffer
			logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

			imageRef, err = name.ParseReference("some-image") // TODO: cleanup the volumes that get created for the build and launch caches
			h.AssertNil(t, err)
		})

		when("creator is supported", func() {
			when("publish is true", func() {
				when("builder is trusted", func() {
					it("uses the creator", func() {
						creator := &fakes.FakeCreator{}
						executor := build.Executor{}

						executor.Execute(
							context.TODO(),
							build.LifecycleOptions{Image: imageRef, Publish: true, TrustBuilder: true},
							creator,
							"0.3",
							&build.DefaultPhaseFactory{},
							*new(dockercli.CommonAPIClient),
							logger,
						)

						h.AssertEq(t, creator.CreateCallCount, 1)
					})
				})

				when("builder is untrusted", func() {
					it("runs the 5 phases", func() {
						creator := &fakes.FakeCreator{}
						executor := build.Executor{}

						executor.Execute(
							context.TODO(),
							build.LifecycleOptions{Image: imageRef, Publish: true, TrustBuilder: false},
							creator,
							"0.3",
							&build.DefaultPhaseFactory{},
							*new(dockercli.CommonAPIClient),
							logger,
						)

						h.AssertEq(t, creator.DetectCallCount, 1)
						h.AssertEq(t, creator.AnalyzeCallCount, 1)
						h.AssertEq(t, creator.RestoreCallCount, 1)
						h.AssertEq(t, creator.BuildCallCount, 1)
						h.AssertEq(t, creator.ExportCallCount, 1)
					})
				})
			})

			when("publish is false", func() {
				it("uses the creator", func() {
					creator := &fakes.FakeCreator{}
					executor := build.Executor{}

					executor.Execute(
						context.TODO(),
						build.LifecycleOptions{Image: imageRef, Publish: false, TrustBuilder: false},
						creator,
						"0.3",
						&build.DefaultPhaseFactory{},
						*new(dockercli.CommonAPIClient),
						logger,
					)

					h.AssertEq(t, creator.CreateCallCount, 1)
				})
			})
		})

		when("creator is not supported", func() {
			it("runs the 5 phases", func() {
				creator := &fakes.FakeCreator{}
				executor := build.Executor{}

				executor.Execute(
					context.TODO(),
					build.LifecycleOptions{Image: imageRef, Publish: true, TrustBuilder: false},
					creator,
					"0.2",
					&build.DefaultPhaseFactory{},
					*new(dockercli.CommonAPIClient),
					logger,
				)

				h.AssertEq(t, creator.DetectCallCount, 1)
				h.AssertEq(t, creator.AnalyzeCallCount, 1)
				h.AssertEq(t, creator.RestoreCallCount, 1)
				h.AssertEq(t, creator.BuildCallCount, 1)
				h.AssertEq(t, creator.ExportCallCount, 1)
			})
		})
	})
}