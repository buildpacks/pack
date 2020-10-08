package builder_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/api"

	pubbldr "github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/dist"
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

			testBuildpackOne = dist.BuildpackInfo{
				ID:      "test.buildpack",
				Version: "test.buildpack.version",
			}
			testBuildpackTwo = dist.BuildpackInfo{
				ID:      "test.buildpack.2",
				Version: "test.buildpack.2.version",
			}
			testTopNestedBuildpack = dist.BuildpackInfo{
				ID:      "test.top.nested",
				Version: "test.top.nested.version",
			}
			testLevelOneNestedBuildpack = dist.BuildpackInfo{
				ID:      "test.nested.level.one",
				Version: "test.nested.level.one.version",
			}
			testLevelOneNestedBuildpackTwo = dist.BuildpackInfo{
				ID:      "test.nested.level.one.two",
				Version: "test.nested.level.one.two.version",
			}
			testLevelOneNestedBuildpackThree = dist.BuildpackInfo{
				ID:      "test.nested.level.one.three",
				Version: "test.nested.level.one.three.version",
			}
			testLevelTwoNestedBuildpack = dist.BuildpackInfo{
				ID:      "test.nested.level.two",
				Version: "test.nested.level.two.version",
			}
			topLevelOrder = dist.Order{
				{
					Group: []dist.BuildpackRef{
						{BuildpackInfo: testBuildpackOne},
						{BuildpackInfo: testBuildpackTwo},
						{BuildpackInfo: testTopNestedBuildpack},
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
									{BuildpackInfo: testLevelOneNestedBuildpack},
									{BuildpackInfo: testLevelOneNestedBuildpackTwo},
									{BuildpackInfo: testLevelOneNestedBuildpackThree},
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
									{BuildpackInfo: testLevelTwoNestedBuildpack},
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
									{BuildpackInfo: testLevelTwoNestedBuildpack},
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
							{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testBuildpackOne}},
							{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testBuildpackTwo}},
							{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testTopNestedBuildpack}},
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
							{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testBuildpackOne}},
							{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testBuildpackTwo}},
							{
								BuildpackRef: dist.BuildpackRef{BuildpackInfo: testTopNestedBuildpack},
								GroupDetectionOrder: pubbldr.DetectionOrder{
									{
										BuildpackRef: dist.BuildpackRef{BuildpackInfo: testLevelOneNestedBuildpack},
										GroupDetectionOrder: pubbldr.DetectionOrder{
											{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testLevelTwoNestedBuildpack}},
										},
									},
									{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testLevelOneNestedBuildpackTwo}},
									{
										BuildpackRef: dist.BuildpackRef{BuildpackInfo: testLevelOneNestedBuildpackThree},
										GroupDetectionOrder: pubbldr.DetectionOrder{
											{
												BuildpackRef: dist.BuildpackRef{BuildpackInfo: testLevelTwoNestedBuildpack},
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
							{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testBuildpackOne}},
							{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testBuildpackTwo}},
							{
								BuildpackRef: dist.BuildpackRef{BuildpackInfo: testTopNestedBuildpack},
								GroupDetectionOrder: pubbldr.DetectionOrder{
									{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testLevelOneNestedBuildpack}},
									{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testLevelOneNestedBuildpackTwo}},
									{BuildpackRef: dist.BuildpackRef{BuildpackInfo: testLevelOneNestedBuildpackThree}},
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
										{BuildpackInfo: testLevelOneNestedBuildpack},
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
										{BuildpackInfo: testTopNestedBuildpack},
									},
								},
							},
							LayerDiffID: "layer:diff",
						},
					},
				}
				cyclicOrder := dist.Order{
					{
						Group: []dist.BuildpackRef{{BuildpackInfo: testTopNestedBuildpack}},
					},
				}

				calculator := builder.NewDetectionOrderCalculator()
				order, err := calculator.Order(cyclicOrder, cyclicBuildpackLayers, pubbldr.OrderDetectionMaxDepth)
				assert.Nil(err)

				expectedOrder := pubbldr.DetectionOrder{
					{
						GroupDetectionOrder: pubbldr.DetectionOrder{
							{
								BuildpackRef: dist.BuildpackRef{BuildpackInfo: testTopNestedBuildpack},
								GroupDetectionOrder: pubbldr.DetectionOrder{
									{
										BuildpackRef: dist.BuildpackRef{BuildpackInfo: testLevelOneNestedBuildpack},
										GroupDetectionOrder: pubbldr.DetectionOrder{
											{
												BuildpackRef: dist.BuildpackRef{
													BuildpackInfo: testTopNestedBuildpack,
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
