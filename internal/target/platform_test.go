package target_test

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/target"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestPlatforms(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "TestPlatforms", testPlatforms, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPlatforms(t *testing.T, when spec.G, it spec.S) {
	it.Before(func() {
		var err error
		h.AssertNil(t, err)
	})
	when("target#SupportsPlatform", func() {
		it("should return false when target not supported", func() {
			b := target.SupportsPlatform("os", "arm", "v6")
			h.AssertFalse(t, b)
		})
		it("should parse targets as expected", func() {
			b := target.SupportsPlatform("linux", "arm", "v6")
			h.AssertTrue(t, b)
		})
		it("should support the arm64 variants v8 and v9", func() {
			h.AssertTrue(t, target.SupportsPlatform("linux", "arm64", "v8"))
			h.AssertTrue(t, target.SupportsPlatform("linux", "arm64", "v9"))
		})
		it("should return false for an unknown arm64 variant", func() {
			h.AssertFalse(t, target.SupportsPlatform("linux", "arm64", "v1"))
		})
	})
}
