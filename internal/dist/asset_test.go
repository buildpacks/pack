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
	when("Asset#ToAssetValue", func() {
		it("converts Asset to AssetValue", func() {
			a := dist.Asset{
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

	when("Assets#ToIncompleteAssetMap", func() {
		it("converts Assets to mapping of Sha256 => AssetValue", func() {
			a := dist.Assets{
				{
					Sha256:      "A-sha256",
					ID:          "A-id",
					Version:     "A-version",
					Name:        "A-Name",
					URI:         "A-URI",
					Licenses:    []string{"L1", "L2"},
					Description: "A-description",
					Homepage:    "A-homepage",
					Stacks:      []string{"S1", "S2"},
					Metadata: map[string]interface{}{
						"cool": "beans",
					},
				},
				{
					Sha256:      "B-sha256",
					ID:          "B-id",
					Version:     "B-version",
					Name:        "B-Name",
					URI:         "B-URI",
					Licenses:    []string{"L1", "L2"},
					Description: "B-description",
					Homepage:    "B-homepage",
					Stacks:      []string{"S1", "S2"},
					Metadata: map[string]interface{}{
						"cool": "beans",
					},
				},
			}

			subject := a.ToIncompleteAssetMap()
			assert.Equal(subject, dist.AssetMap{
				"A-sha256": {
					ID:          "A-id",
					Version:     "A-version",
					Name:        "A-Name",
					URI:         "A-URI",
					Licenses:    []string{"L1", "L2"},
					Description: "A-description",
					Homepage:    "A-homepage",
					Stacks:      []string{"S1", "S2"},
					Metadata: map[string]interface{}{
						"cool": "beans",
					},
				},
				"B-sha256": {
					ID:          "B-id",
					Version:     "B-version",
					Name:        "B-Name",
					URI:         "B-URI",
					Licenses:    []string{"L1", "L2"},
					Description: "B-description",
					Homepage:    "B-homepage",
					Stacks:      []string{"S1", "S2"},
					Metadata: map[string]interface{}{
						"cool": "beans",
					},
				},
			})
		})
	})
}
