package api_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/internal/api"
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
		when("pre-stable", func() {
			it("matching minor value", func() {
				subject := api.MustParse("0.2")
				comparison := api.MustParse("0.2")

				h.AssertEq(t, subject.SupportsVersion(comparison), true)
			})

			it("subject minor > comparison minor", func() {
				subject := api.MustParse("0.2")
				comparison := api.MustParse("0.1")

				h.AssertEq(t, subject.SupportsVersion(comparison), false)
			})

			it("subject minor < comparison minor", func() {
				subject := api.MustParse("0.1")
				comparison := api.MustParse("0.2")

				h.AssertEq(t, subject.SupportsVersion(comparison), false)
			})
		})

		when("stable", func() {
			it("matching major and minor", func() {
				subject := api.MustParse("1.2")
				comparison := api.MustParse("1.2")

				h.AssertEq(t, subject.SupportsVersion(comparison), true)
			})

			it("matching major but minor > comparison minor", func() {
				subject := api.MustParse("1.2")
				comparison := api.MustParse("1.1")

				h.AssertEq(t, subject.SupportsVersion(comparison), true)
			})

			it("matching major but minor < comparison minor", func() {
				subject := api.MustParse("1.1")
				comparison := api.MustParse("1.2")

				h.AssertEq(t, subject.SupportsVersion(comparison), false)
			})

			it("major < comparison major", func() {
				subject := api.MustParse("1.0")
				comparison := api.MustParse("2.0")

				h.AssertEq(t, subject.SupportsVersion(comparison), false)
			})

			it("major > comparison major", func() {
				subject := api.MustParse("2.0")
				comparison := api.MustParse("1.0")

				h.AssertEq(t, subject.SupportsVersion(comparison), false)
			})
		})
	})
}
