package dist_test

import (
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/testhelpers"
	"github.com/sclevine/spec"
	"testing"
)

func TestAssetMap(t *testing.T) {
	spec.Run(t, "asset test", testAssetMap)
}

func testAssetMap(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = testhelpers.NewAssertionManager(t)
	)
	when("AssetValue#ToAsset", func() {
		it("converts AssetValue to Asset object", func() {
			a := dist.AssetValue{
				ID:          "id",
				Version:     "version",
				Name:        "Name",
				URI:         "URI",
				LayerDiffID: "layerDiff",
				Licenses:    []string{"L1", "L2"},
				Description: "description",
				Homepage:    "homepage",
				Stacks:      []string{"S1", "S2"},
				Metadata: map[string]interface{}{
					"cool": "beans",
				},
			}

			subject := a.ToAsset("sha256")
			assert.Equal(subject, dist.Asset{
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
			})
		})
	})

	when("AssetMap#ToAssets", func() {
		it("converts an asset map to an Assets array", func() {
			a := dist.AssetMap{
				"A-sha256": {
					ID:          "A-id",
					Version:     "A-version",
					Name:        "A-Name",
					URI:         "A-URI",
					LayerDiffID: "A-layerDiffID",
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
					LayerDiffID: "B-layerDiffID",
					Licenses:    []string{"L1", "L2"},
					Description: "B-description",
					Homepage:    "B-homepage",
					Stacks:      []string{"S1", "S2"},
					Metadata: map[string]interface{}{
						"cool": "beans",
					},
				},
			}

			subject := a.ToAssets()
			assert.Equal(subject, dist.Assets{
				{
					ID:          "A-id",
					Sha256:      "A-sha256",
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
				}, {
					ID:          "B-id",
					Sha256:      "B-sha256",
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

	when("AssetMap#Keys", func() {
		it("returns sorted map keys", func() {
			a := dist.AssetMap{
				"A-sha256": dist.AssetValue{},
				"B-sha256": dist.AssetValue{},
				"C-sha256": dist.AssetValue{},
				"D-sha256": dist.AssetValue{},
				"E-sha256": dist.AssetValue{},
			}

			keys := a.Keys()
			assert.Equal(keys, []string{
				"A-sha256",
				"B-sha256",
				"C-sha256",
				"D-sha256",
				"E-sha256",
			})
		})
	})

	when("AssetMap#Filter", func() {
		it("filters out all but the sepecified keys", func() {

			a := dist.AssetMap{
				"A-sha256": dist.AssetValue{},
				"B-sha256": dist.AssetValue{},
				"C-sha256": dist.AssetValue{},
				"D-sha256": dist.AssetValue{},
				"E-sha256": dist.AssetValue{},
			}
			a.Filter([]string{"B-sha256", "D-sha256", "G-sha256"})

			assert.Equal(a, dist.AssetMap{
				"B-sha256": dist.AssetValue{},
				"D-sha256": dist.AssetValue{},
			})
		})
	})
}
