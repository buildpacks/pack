package config_test

import (
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/testhelpers"
)

func TestOS(t *testing.T) {
	spec.Run(t, "ConfigOS", testOS)
}

func testOS(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = testhelpers.NewAssertionManager(t)
	)
	when("#ValidateOS", func() {
		it("validates 'linux' as an os", func() {
			assert.Succeeds(config.ValidateOS("linux"))
		})

		it("validates 'windows' as an os", func() {
			assert.Succeeds(config.ValidateOS("windows"))
		})

		when("failure cases", func() {
			it("returns an error when passed an undefined OS", func() {
				err := config.ValidateOS("invalid-os")
				assert.ErrorContains(err, `unknown os type: "invalid-os"`)
			})
		})
	})
}
