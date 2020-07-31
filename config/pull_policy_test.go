package config_test

import (
	"github.com/buildpacks/pack/config"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"testing"
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

	when("#ParsePolicyFromPull", func() {
		it("returns PullAlways if true", func() {
			h.AssertEq(t, config.ParsePolicyFromPull(true), config.PullAlways)
		})

		it("returns PullNever if false", func() {
			h.AssertEq(t, config.ParsePolicyFromPull(false), config.PullNever)
		})
	})
}