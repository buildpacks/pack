package writer_test

import (
	pubbldr "github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/builder/writer"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
)

var (
	testTopNestedBuildpack = dist.BuildpackInfo{
		ID:      "test.top.nested",
		Version: "test.top.nested.version",
	}
	testNestedBuildpack = dist.BuildpackInfo{
		ID:       "test.nested",
		Homepage: "http://geocities.com/top-bp",
	}
	testBuildpackOne = dist.BuildpackInfo{
		ID:       "test.bp.one",
		Version:  "test.bp.one.version",
		Homepage: "http://geocities.com/cool-bp",
	}
	testBuildpackTwo = dist.BuildpackInfo{
		ID:      "test.bp.two",
		Version: "test.bp.two.version",
	}
	testBuildpackThree = dist.BuildpackInfo{
		ID:      "test.bp.three",
		Version: "test.bp.three.version",
	}
	testNestedBuildpackTwo = dist.BuildpackInfo{
		ID:      "test.nested.two",
		Version: "test.nested.two.version",
	}

	buildpacks = []dist.BuildpackInfo{
		testTopNestedBuildpack,
		testNestedBuildpack,
		testBuildpackOne,
		testBuildpackTwo,
		testBuildpackThree,
	}

	order = pubbldr.DetectionOrder{
		pubbldr.DetectionOrderEntry{
			GroupDetectionOrder: pubbldr.DetectionOrder{
				pubbldr.DetectionOrderEntry{
					BuildpackRef: dist.BuildpackRef{
						BuildpackInfo: testTopNestedBuildpack,
					},
					GroupDetectionOrder: pubbldr.DetectionOrder{
						pubbldr.DetectionOrderEntry{
							BuildpackRef: dist.BuildpackRef{BuildpackInfo: testNestedBuildpack},
							GroupDetectionOrder: pubbldr.DetectionOrder{
								pubbldr.DetectionOrderEntry{
									BuildpackRef: dist.BuildpackRef{
										BuildpackInfo: testBuildpackOne,
										Optional:      true,
									},
								},
							},
						},
						pubbldr.DetectionOrderEntry{
							BuildpackRef: dist.BuildpackRef{
								BuildpackInfo: testBuildpackThree,
								Optional:      true,
							},
						},
						pubbldr.DetectionOrderEntry{
							BuildpackRef: dist.BuildpackRef{BuildpackInfo: testNestedBuildpackTwo},
							GroupDetectionOrder: pubbldr.DetectionOrder{
								pubbldr.DetectionOrderEntry{
									BuildpackRef: dist.BuildpackRef{
										BuildpackInfo: testBuildpackOne,
										Optional:      true,
									},
									Cyclical: true,
								},
							},
						},
					},
				},
				pubbldr.DetectionOrderEntry{
					BuildpackRef: dist.BuildpackRef{
						BuildpackInfo: testBuildpackTwo,
						Optional:      true,
					},
				},
			},
		},
		pubbldr.DetectionOrderEntry{
			BuildpackRef: dist.BuildpackRef{
				BuildpackInfo: testBuildpackThree,
			},
		},
	}

	sharedBuilderInfo = writer.SharedBuilderInfo{
		Name:      "test-builder",
		Trusted:   false,
		IsDefault: false,
	}

	localRunImages = []config.RunImage{
		{Image: "some/run-image", Mirrors: []string{"first/local", "second/local"}},
	}
)
