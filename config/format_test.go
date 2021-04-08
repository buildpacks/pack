package config_test

import (
	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/testhelpers"
	"github.com/sclevine/spec"
	"testing"
)

func TestFormat(t *testing.T) {
	spec.Run(t, "ConfigFormat", testFormat)
}

func testFormat(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = testhelpers.NewAssertionManager(t)
	)
	when("#ValidateFormat", func() {
		it("validates 'file' as a valid format", func() {
			assert.Succeeds(config.ValidateFormat("image"))
		})

		it("validates 'file' as a valid format", func() {
			assert.Succeeds(config.ValidateFormat("file"))
		})

		when("failure cases", func() {
			it("returns an error when passed an undefined format", func() {
				err := config.ValidateFormat("invalid-format")
				assert.ErrorContains(err, `unknown format type: "invalid-format"`)
			})
		})
	})
}
