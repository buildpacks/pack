package dist_test

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/api"
	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestDist(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "testDist", testDist, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testDist(t *testing.T, when spec.G, it spec.S) {
	when("BuildpackLayers", func() {
		when("Get", func() {
			var (
				buildpackLayers dist.BuildpackLayers
				apiVersion      *api.Version
			)
			it.Before(func() {
				var err error
				apiVersion, err = api.NewVersion("0.0")
				h.AssertNil(t, err)

				buildpackLayers = dist.BuildpackLayers{
					"buildpack": {
						"version1": {
							API:         apiVersion,
							LayerDiffID: "buildpack-v1-diff",
						},
					},
					"other-buildpack": {
						"version1": {
							API:         apiVersion,
							LayerDiffID: "other-buildpack-v2-diff",
						},
						"version2": {
							API:         apiVersion,
							LayerDiffID: "other-buildpack-v2-diff",
						},
					},
				}
			})

			when("ID and Version are provided and present", func() {
				it("succeeds", func() {
					out, ok := buildpackLayers.Get("buildpack", "version1")
					h.AssertEq(t, ok, true)
					h.AssertEq(t, out, dist.BuildpackLayerInfo{
						API:         apiVersion,
						LayerDiffID: "buildpack-v1-diff",
					})
				})
			})

			when("ID is present, Version is left empty, but can be inferred", func() {
				it("succeeds", func() {
					out, ok := buildpackLayers.Get("buildpack", "")
					h.AssertEq(t, ok, true)
					h.AssertEq(t, out, dist.BuildpackLayerInfo{
						API:         apiVersion,
						LayerDiffID: "buildpack-v1-diff",
					})
				})
			})

			when("ID is present, Version is left empty and cannot be inferred", func() {
				it("fails", func() {
					out, ok := buildpackLayers.Get("other-buildpack", "")
					h.AssertEq(t, ok, false)
					h.AssertEq(t, out, dist.BuildpackLayerInfo{})
				})
			})

			when("ID is NOT provided", func() {
				it("fails", func() {
					out, ok := buildpackLayers.Get("missing-buildpack", "")
					h.AssertEq(t, ok, false)
					h.AssertEq(t, out, dist.BuildpackLayerInfo{})
				})
			})
		})
	})
}
