package config_test

import (
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/testhelpers"
)

func TestImagePreference(t *testing.T) {
	spec.Run(t, "ImagePreference", testImagePreference)
}

func testImagePreference(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = testhelpers.NewAssertionManager(t)
	)
	when("#ValidateImagePreference", func() {
		it("validates 'prefer-local' as a preference", func() {
			assert.Succeeds(config.ValidateImagePreference("prefer-local"))
		})

		it("validates 'prefer-remote' as a preference", func() {
			assert.Succeeds(config.ValidateImagePreference("prefer-remote"))
		})

		it("validates 'only-local' as a preference", func() {
			assert.Succeeds(config.ValidateImagePreference("only-local"))
		})

		it("validates 'only-remote' as a preference", func() {
			assert.Succeeds(config.ValidateImagePreference("only-remote"))
		})

		when("failure cases", func() {
			it("returns an error when passed an undefined OS", func() {
				err := config.ValidateImagePreference("invalid-preference")
				assert.ErrorContains(err, `unknown image preference: "invalid-preference"`)
			})
		})
	})
}
