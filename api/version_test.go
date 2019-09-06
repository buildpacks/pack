package api_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/api"
	h "github.com/buildpack/pack/testhelpers"
)

func TestAPIVersion(t *testing.T) {
	spec.Run(t, "APIVersion", testAPIVersion, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAPIVersion(t *testing.T, when spec.G, it spec.S) {
	when("#Equal", func() {
		it("is equal to comparison", func() {
			subject := api.MustParse("0.2")
			comparison := api.MustParse("0.2")

			h.AssertEq(t, subject.Equal(comparison), true)
		})

		it("is not equal to comparison", func() {
			subject := api.MustParse("0.2")
			comparison := api.MustParse("0.3")

			h.AssertEq(t, subject.Equal(comparison), false)
		})
	})

	when("#SupportsVersion", func() {
		it("is none-stable with matching minor value", func() {
			subject := api.MustParse("0.2")
			comparison := api.MustParse("0.2")

			h.AssertEq(t, subject.SupportsVersion(comparison), true)
		})
		it("is none-stable with subject minor > comparison minor", func() {
			subject := api.MustParse("0.2")
			comparison := api.MustParse("0.1")

			h.AssertEq(t, subject.SupportsVersion(comparison), false)
		})

		it("is none-stable with subject minor < comparison minor", func() {
			subject := api.MustParse("0.1")
			comparison := api.MustParse("0.2")

			h.AssertEq(t, subject.SupportsVersion(comparison), false)
		})

		it("is stable with matching major and minor", func() {
			subject := api.MustParse("1.2")
			comparison := api.MustParse("1.2")

			h.AssertEq(t, subject.SupportsVersion(comparison), true)
		})

		it("is stable with matching major but minor > comparison minor", func() {
			subject := api.MustParse("1.2")
			comparison := api.MustParse("1.1")

			h.AssertEq(t, subject.SupportsVersion(comparison), true)
		})

		it("is stable with matching major but minor < comparison minor", func() {
			subject := api.MustParse("1.1")
			comparison := api.MustParse("1.2")

			h.AssertEq(t, subject.SupportsVersion(comparison), true)
		})

		it("is stable with major < comparison major", func() {
			subject := api.MustParse("1.0")
			comparison := api.MustParse("2.0")

			h.AssertEq(t, subject.SupportsVersion(comparison), false)
		})

		it("is stable with major > comparison major", func() {
			subject := api.MustParse("2.0")
			comparison := api.MustParse("1.0")

			h.AssertEq(t, subject.SupportsVersion(comparison), false)
		})
	})
}
