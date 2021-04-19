package dist_test

import (
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/testhelpers"
)

func TestAsset(t *testing.T) {
	spec.Run(t, "asset test", testAsset)
}

func testAsset(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = testhelpers.NewAssertionManager(t)
	)
	when("AssetInfo#ToAssetValue", func() {
		it("converts AssetInfo to AssetValue", func() {
			a := dist.AssetInfo{
				Sha256:      "sha256",
				ID:          "id",
				Version:     "version",
				Name:        "Name",
				URI:         "URI",
				Licenses:    []string{"L1", "L2"},
				Description: "description",
				Homepage:    "homepage",
				Stacks:      []string{"S1", "S2"},
				Metadata: map[string]interface{}{
					"cool": "beans",
				},
			}

			subject := a.ToAssetValue("layer-diff-id")
			assert.Equal(subject, dist.AssetValue{
				ID:          "id",
				Version:     "version",
				Name:        "Name",
				LayerDiffID: "layer-diff-id",
				URI:         "URI",
				Licenses:    []string{"L1", "L2"},
				Description: "description",
				Homepage:    "homepage",
				Stacks:      []string{"S1", "S2"},
				Metadata: map[string]interface{}{
					"cool": "beans",
				},
			})
		})
	})
}
