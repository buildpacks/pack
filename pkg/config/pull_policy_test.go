package config_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/config"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestPullPolicy(t *testing.T) {
	spec.Run(t, "PullPolicy", testPullPolicy, spec.Report(report.Terminal{}))
}

func testPullPolicy(t *testing.T, when spec.G, it spec.S) {
	when("#ParsePullPolicy", func() {
		it("returns PullNever for never", func() {
			policy, err := config.ParsePullPolicy("never")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, config.PullNever)
		})

		it("returns PullAlways for always", func() {
			policy, err := config.ParsePullPolicy("always")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, config.PullAlways)
		})

		it("returns PullIfNotPresent for if-not-present", func() {
			policy, err := config.ParsePullPolicy("if-not-present")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, config.PullIfNotPresent)
		})

		it("defaults to PullAlways, if empty string", func() {
			policy, err := config.ParsePullPolicy("")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, config.PullAlways)
		})

		it("returns error for unknown string", func() {
			_, err := config.ParsePullPolicy("fake-policy-here")
			h.AssertError(t, err, "invalid pull policy")
		})
	})

	when("#String", func() {
		it("returns the right String value", func() {
			h.AssertEq(t, config.PullAlways.String(), "always")
			h.AssertEq(t, config.PullNever.String(), "never")
			h.AssertEq(t, config.PullIfNotPresent.String(), "if-not-present")
		})
	})
}
