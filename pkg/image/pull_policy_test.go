package image_test

import (
	"bytes"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
	fetcher_mock "github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestPullPolicy(t *testing.T) {
	spec.Run(t, "PullPolicy", testPullPolicy, spec.Report(report.Terminal{}))
}

func testPullPolicy(t *testing.T, when spec.G, it spec.S) {
	when("#ParsePullPolicy", func() {
		var (
			outBuf           bytes.Buffer
			logger           logging.Logger
			imagePullChecker image.ImagePullPolicyHandler
		)

		it.Before(func() {
			logger = logging.NewLogWithWriters(&outBuf, &outBuf)
			imagePullChecker = fetcher_mock.NewMockPullPolicyManager(mockController)
		})

		it("returns PullNever for never", func() {
			policy, err := imagePullChecker.ParsePullPolicy("never")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, image.PullNever)
		})

		it("returns PullAlways for always", func() {
			policy, err := imagePullChecker.ParsePullPolicy("always")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, image.PullAlways)
		})

		it("returns PullIfNotPresent for if-not-present", func() {
			policy, err := imagePullChecker.ParsePullPolicy("if-not-present")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, image.PullIfNotPresent)
		})

		it("returns PullHourly for hourly", func() {
			policy, err := imagePullChecker.ParsePullPolicy("hourly")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, image.PullHourly)
		})

		it("returns PullDaily for daily", func() {
			policy, err := imagePullChecker.ParsePullPolicy("daily")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, image.PullDaily)
		})

		it("returns PullWeekly for weekly", func() {
			policy, err := imagePullChecker.ParsePullPolicy("weekly")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, image.PullWeekly)
		})

		it("returns PullWithInterval for interval= format", func() {
			policy, err := imagePullChecker.ParsePullPolicy("interval=4d")
			h.AssertNil(t, err)
			h.AssertEq(t, policy, image.PullWithInterval)
		})

		it("returns error for unknown string", func() {
			_, err := imagePullChecker.ParsePullPolicy("fake-policy-here")
			h.AssertError(t, err, "invalid pull policy")
		})

		it("returns error for invalid interval format", func() {
			_, err := imagePullChecker.ParsePullPolicy("interval=invalid")
			h.AssertError(t, err, "invalid interval format")
		})
	})

	when("#String", func() {
		it("returns the right String value", func() {
			h.AssertEq(t, image.PullAlways.String(), "always")
			h.AssertEq(t, image.PullNever.String(), "never")
			h.AssertEq(t, image.PullIfNotPresent.String(), "if-not-present")
			h.AssertEq(t, image.PullHourly.String(), "hourly")
			h.AssertEq(t, image.PullDaily.String(), "daily")
			h.AssertEq(t, image.PullWeekly.String(), "weekly")
			h.AssertContains(t, image.PullWithInterval.String(), "interval=")
		})
	})
}
