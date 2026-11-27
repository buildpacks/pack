package builder_test

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	bldr "github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/config"

	h "github.com/buildpacks/pack/testhelpers"
)

func TestTrustedBuilder(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Trusted Builder", trustedBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func trustedBuilder(t *testing.T, when spec.G, it spec.S) {
	when("IsKnownTrustedBuilder", func() {
		it("matches tagless known builders against any tag in the same repository", func() {
			h.AssertTrue(t, bldr.IsKnownTrustedBuilder("paketobuildpacks/builder-jammy-base"))
			h.AssertTrue(t, bldr.IsKnownTrustedBuilder("paketobuildpacks/builder-jammy-base:latest"))
			h.AssertTrue(t, bldr.IsKnownTrustedBuilder("paketobuildpacks/builder-jammy-base:1.2.3"))
		})
		it("requires an exact tag match for tagged known builders", func() {
			h.AssertTrue(t, bldr.IsKnownTrustedBuilder("heroku/builder:24"))
			h.AssertFalse(t, bldr.IsKnownTrustedBuilder("heroku/builder"))
			h.AssertFalse(t, bldr.IsKnownTrustedBuilder("heroku/builder:99"))
		})
		it("does not match unknown builders", func() {
			h.AssertFalse(t, bldr.IsKnownTrustedBuilder("my/private/builder"))
		})
		it("does not match the same repository served by another registry", func() {
			h.AssertFalse(t, bldr.IsKnownTrustedBuilder("example.com/paketobuildpacks/builder-jammy-base"))
			h.AssertFalse(t, bldr.IsKnownTrustedBuilder("example.com/paketobuildpacks/builder-jammy-base:latest"))
			h.AssertFalse(t, bldr.IsKnownTrustedBuilder("example.com:5000/paketobuildpacks/builder-jammy-base"))
			h.AssertFalse(t, bldr.IsKnownTrustedBuilder("example.com/buildpacks/builder:google-22"))
			h.AssertFalse(t, bldr.IsKnownTrustedBuilder("example.com/heroku/builder:24"))
		})
	})

	when("IsTrustedBuilder", func() {
		it("trusts known trusted builders", func() {
			// Known builder with exact tag match
			isTrusted, err := bldr.IsTrustedBuilder(config.Config{}, "heroku/builder:24")
			h.AssertNil(t, err)
			h.AssertTrue(t, isTrusted)

			// Known builder without tag should match any tag
			isTrusted, err = bldr.IsTrustedBuilder(config.Config{}, "paketobuildpacks/builder-jammy-base:latest")
			h.AssertNil(t, err)
			h.AssertTrue(t, isTrusted)

			// Unknown builder should not be trusted
			isTrusted, err = bldr.IsTrustedBuilder(config.Config{}, "my/private/builder")
			h.AssertNil(t, err)
			h.AssertFalse(t, isTrusted)
		})

		it("does not trust a known builder's repository served by another registry", func() {
			for _, builder := range []string{
				"example.com/paketobuildpacks/builder-jammy-base",
				"example.com/paketobuildpacks/builder-jammy-base:latest",
				"example.com/heroku/builder:24",
			} {
				isTrusted, err := bldr.IsTrustedBuilder(config.Config{}, builder)
				h.AssertNil(t, err)
				h.AssertFalse(t, isTrusted)
			}
		})

		it("scopes configured builders to their registry", func() {
			cfg := config.Config{
				TrustedBuilders: []config.TrustedBuilder{
					{
						Name: "mirror.example.com/paketobuildpacks/builder-jammy-base",
					},
				},
			}

			// The mirror is named in full, so any tag of it is trusted.
			trustedBuilders := []string{
				"mirror.example.com/paketobuildpacks/builder-jammy-base",
				"mirror.example.com/paketobuildpacks/builder-jammy-base:1.2.3",
			}

			// Configuring one registry must not trust the same path elsewhere.
			untrustedBuilders := []string{
				"other.example.com/paketobuildpacks/builder-jammy-base",
				"other.example.com/paketobuildpacks/builder-jammy-base:1.2.3",
			}

			for _, builder := range trustedBuilders {
				isTrusted, err := bldr.IsTrustedBuilder(cfg, builder)
				h.AssertNil(t, err)
				h.AssertTrue(t, isTrusted)
			}

			for _, builder := range untrustedBuilders {
				isTrusted, err := bldr.IsTrustedBuilder(cfg, builder)
				h.AssertNil(t, err)
				h.AssertFalse(t, isTrusted)
			}
		})

		it("trust image without tag", func() {
			cfg := config.Config{
				TrustedBuilders: []config.TrustedBuilder{
					{
						Name: "my/trusted/builder-jammy",
					},
				},
			}

			trustedBuilders := []string{
				"my/trusted/builder-jammy",
				"my/trusted/builder-jammy:latest",
				"my/trusted/builder-jammy:1.2.3",
			}

			untrustedBuilders := []string{
				"my/private/builder",            // random builder
				"my/trusted/builder-jammy-base", // shared prefix
			}

			for _, builder := range trustedBuilders {
				isTrusted, err := bldr.IsTrustedBuilder(cfg, builder)
				h.AssertNil(t, err)
				h.AssertTrue(t, isTrusted)
			}

			for _, builder := range untrustedBuilders {
				isTrusted, err := bldr.IsTrustedBuilder(cfg, builder)
				h.AssertNil(t, err)
				h.AssertFalse(t, isTrusted)
			}
		})
		it("trust image with tag", func() {
			cfg := config.Config{
				TrustedBuilders: []config.TrustedBuilder{
					{
						Name: "my/trusted/builder-jammy:1.2.3",
					},
					{
						Name: "my/trusted/builder-jammy:latest",
					},
				},
			}

			trustedBuilders := []string{
				"my/trusted/builder-jammy:1.2.3",
				"my/trusted/builder-jammy:latest",
			}

			untrustedBuilders := []string{
				"my/private/builder",
				"my/trusted/builder-jammy",
				"my/trusted/builder-jammy:2.0.0",
				"my/trusted/builder-jammy-base",
			}

			for _, builder := range trustedBuilders {
				isTrusted, err := bldr.IsTrustedBuilder(cfg, builder)
				h.AssertNil(t, err)
				h.AssertTrue(t, isTrusted)
			}

			for _, builder := range untrustedBuilders {
				isTrusted, err := bldr.IsTrustedBuilder(cfg, builder)
				h.AssertNil(t, err)
				h.AssertFalse(t, isTrusted)
			}
		})
	})
}
