package builder_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/api"

	pubbldr "github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/pkg/dist"
	h "github.com/buildpacks/pack/testhelpers"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestDetectionOrderCalculator(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "testDetectionOrderCalculator", testDetectionOrderCalculator, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testDetectionOrderCalculator(t *testing.T, when spec.G, it spec.S) {
	when("Order", func() {
		var (
			assert = h.NewAssertionManager(t)

			testBuildpackOne = dist.ModuleInfo{
				ID:      "test.buildpack",
				Version: "test.buildpack.version",
			}
			testBuildpackTwo = dist.ModuleInfo{
				ID:      "test.buildpack.2",
				Version: "test.buildpack.2.version",
			}
			testTopNestedBuildpack = dist.ModuleInfo{
				ID:      "test.top.nested",
				Version: "test.top.nested.version",
			}
			testLevelOneNestedBuildpack = dist.ModuleInfo{
				ID:      "test.nested.level.one",
				Version: "test.nested.level.one.version",
			}
			testLevelOneNestedBuildpackTwo = dist.ModuleInfo{
				ID:      "test.nested.level.one.two",
				Version: "test.nested.level.one.two.version",
			}
			testLevelOneNestedBuildpackThree = dist.ModuleInfo{
				ID:      "test.nested.level.one.three",
				Version: "test.nested.level.one.three.version",
			}
			testLevelTwoNestedBuildpack = dist.ModuleInfo{
				ID:      "test.nested.level.two",
				Version: "test.nested.level.two.version",
			}
			topLevelOrder = dist.Order{
				{
					Group: []dist.BuildpackRef{
						{ModuleInfo: testBuildpackOne},
						{ModuleInfo: testBuildpackTwo},
						{ModuleInfo: testTopNestedBuildpack},
					},
				},
			}
			buildpackLayers = dist.BuildpackLayers{
				"test.buildpack": {
					"test.buildpack.version": dist.BuildpackLayerInfo{
						API:         api.MustParse("0.2"),
						LayerDiffID: "layer:diff",
					},
				},
				"test.top.nested": {
					"test.top.nested.version": dist.BuildpackLayerInfo{
						API: api.MustParse("0.2"),
						Order: dist.Order{
							{
								Group: []dist.BuildpackRef{
									{ModuleInfo: testLevelOneNestedBuildpack},
									{ModuleInfo: testLevelOneNestedBuildpackTwo},
									{ModuleInfo: testLevelOneNestedBuildpackThree},
								},
							},
						},
						LayerDiffID: "layer:diff",
					},
				},
				"test.nested.level.one": {
					"test.nested.level.one.version": dist.BuildpackLayerInfo{
						API: api.MustParse("0.2"),
						Order: dist.Order{
							{
								Group: []dist.BuildpackRef{
									{ModuleInfo: testLevelTwoNestedBuildpack},
								},
							},
						},
						LayerDiffID: "layer:diff",
					},
				},
				"test.nested.level.one.three": {
					"test.nested.level.one.three.version": dist.BuildpackLayerInfo{
						API: api.MustParse("0.2"),
						Order: dist.Order{
							{
								Group: []dist.BuildpackRef{
									{ModuleInfo: testLevelTwoNestedBuildpack},
									{ModuleInfo: testTopNestedBuildpack},
								},
							},
						},
						LayerDiffID: "layer:diff",
					},
				},
			}
		)

		when("called with no depth", func() {
			it("returns detection order with top level order of buildpacks", func() {
				calculator := builder.NewDetectionOrderCalculator()
				order, err := calculator.Order(topLevelOrder, buildpackLayers, pubbldr.OrderDetectionNone)
				assert.Nil(err)

				expectedOrder := pubbldr.DetectionOrder{
					{
						GroupDetectionOrder: pubbldr.DetectionOrder{
							{BuildpackRef: dist.BuildpackRef{ModuleInfo: testBuildpackOne}},
							{BuildpackRef: dist.BuildpackRef{ModuleInfo: testBuildpackTwo}},
							{BuildpackRef: dist.BuildpackRef{ModuleInfo: testTopNestedBuildpack}},
						},
					},
				}

				assert.Equal(order, expectedOrder)
			})
		})

		when("called with max depth", func() {
			it("returns detection order for nested buildpacks", func() {
				calculator := builder.NewDetectionOrderCalculator()
				order, err := calculator.Order(topLevelOrder, buildpackLayers, pubbldr.OrderDetectionMaxDepth)
				assert.Nil(err)

				expectedOrder := pubbldr.DetectionOrder{
					{
						GroupDetectionOrder: pubbldr.DetectionOrder{
							{BuildpackRef: dist.BuildpackRef{ModuleInfo: testBuildpackOne}},
							{BuildpackRef: dist.BuildpackRef{ModuleInfo: testBuildpackTwo}},
							{
								BuildpackRef: dist.BuildpackRef{ModuleInfo: testTopNestedBuildpack},
								GroupDetectionOrder: pubbldr.DetectionOrder{
									{
										BuildpackRef: dist.BuildpackRef{ModuleInfo: testLevelOneNestedBuildpack},
										GroupDetectionOrder: pubbldr.DetectionOrder{
											{BuildpackRef: dist.BuildpackRef{ModuleInfo: testLevelTwoNestedBuildpack}},
										},
									},
									{BuildpackRef: dist.BuildpackRef{ModuleInfo: testLevelOneNestedBuildpackTwo}},
									{
										BuildpackRef: dist.BuildpackRef{ModuleInfo: testLevelOneNestedBuildpackThree},
										GroupDetectionOrder: pubbldr.DetectionOrder{
											{
												BuildpackRef: dist.BuildpackRef{ModuleInfo: testLevelTwoNestedBuildpack},
												Cyclical:     false,
											},
											{
												BuildpackRef: dist.BuildpackRef{ModuleInfo: testTopNestedBuildpack},
												Cyclical:     true,
											},
										},
									},
								},
							},
						},
					},
				}

				assert.Equal(order, expectedOrder)
			})
		})

		when("called with a depth of 1", func() {
			it("returns detection order for first level of nested buildpacks", func() {
				calculator := builder.NewDetectionOrderCalculator()
				order, err := calculator.Order(topLevelOrder, buildpackLayers, 1)
				assert.Nil(err)

				expectedOrder := pubbldr.DetectionOrder{
					{
						GroupDetectionOrder: pubbldr.DetectionOrder{
							{BuildpackRef: dist.BuildpackRef{ModuleInfo: testBuildpackOne}},
							{BuildpackRef: dist.BuildpackRef{ModuleInfo: testBuildpackTwo}},
							{
								BuildpackRef: dist.BuildpackRef{ModuleInfo: testTopNestedBuildpack},
								GroupDetectionOrder: pubbldr.DetectionOrder{
									{BuildpackRef: dist.BuildpackRef{ModuleInfo: testLevelOneNestedBuildpack}},
									{BuildpackRef: dist.BuildpackRef{ModuleInfo: testLevelOneNestedBuildpackTwo}},
									{BuildpackRef: dist.BuildpackRef{ModuleInfo: testLevelOneNestedBuildpackThree}},
								},
							},
						},
					},
				}

				assert.Equal(order, expectedOrder)
			})
		})

		when("a buildpack is referenced in a sub detection group", func() {
			it("marks the buildpack is cyclic and doesn't attempt to calculate that buildpacks order", func() {
				cyclicBuildpackLayers := dist.BuildpackLayers{
					"test.top.nested": {
						"test.top.nested.version": dist.BuildpackLayerInfo{
							API: api.MustParse("0.2"),
							Order: dist.Order{
								{
									Group: []dist.BuildpackRef{
										{ModuleInfo: testLevelOneNestedBuildpack},
									},
								},
							},
							LayerDiffID: "layer:diff",
						},
					},
					"test.nested.level.one": {
						"test.nested.level.one.version": dist.BuildpackLayerInfo{
							API: api.MustParse("0.2"),
							Order: dist.Order{
								{
									Group: []dist.BuildpackRef{
										{ModuleInfo: testTopNestedBuildpack},
									},
								},
							},
							LayerDiffID: "layer:diff",
						},
					},
				}
				cyclicOrder := dist.Order{
					{
						Group: []dist.BuildpackRef{{ModuleInfo: testTopNestedBuildpack}},
					},
				}

				calculator := builder.NewDetectionOrderCalculator()
				order, err := calculator.Order(cyclicOrder, cyclicBuildpackLayers, pubbldr.OrderDetectionMaxDepth)
				assert.Nil(err)

				expectedOrder := pubbldr.DetectionOrder{
					{
						GroupDetectionOrder: pubbldr.DetectionOrder{
							{
								BuildpackRef: dist.BuildpackRef{ModuleInfo: testTopNestedBuildpack},
								GroupDetectionOrder: pubbldr.DetectionOrder{
									{
										BuildpackRef: dist.BuildpackRef{ModuleInfo: testLevelOneNestedBuildpack},
										GroupDetectionOrder: pubbldr.DetectionOrder{
											{
												BuildpackRef: dist.BuildpackRef{
													ModuleInfo: testTopNestedBuildpack,
												},
												Cyclical: true,
											},
										},
									},
								},
							},
						},
					},
				}

				assert.Equal(order, expectedOrder)
			})
		})
	})
}
