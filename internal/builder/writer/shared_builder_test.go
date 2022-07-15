package writer_test

import (
	pubbldr "github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/builder/writer"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/dist"
)

var (
	testTopNestedBuildpack = dist.ModuleInfo{
		ID:      "test.top.nested",
		Version: "test.top.nested.version",
	}
	testNestedBuildpack = dist.ModuleInfo{
		ID:       "test.nested",
		Homepage: "http://geocities.com/top-bp",
	}
	testBuildpackOne = dist.ModuleInfo{
		ID:       "test.bp.one",
		Version:  "test.bp.one.version",
		Homepage: "http://geocities.com/cool-bp",
	}
	testBuildpackTwo = dist.ModuleInfo{
		ID:      "test.bp.two",
		Version: "test.bp.two.version",
	}
	testBuildpackThree = dist.ModuleInfo{
		ID:      "test.bp.three",
		Version: "test.bp.three.version",
	}
	testNestedBuildpackTwo = dist.ModuleInfo{
		ID:      "test.nested.two",
		Version: "test.nested.two.version",
	}

	buildpacks = []dist.ModuleInfo{
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
						ModuleInfo: testTopNestedBuildpack,
					},
					GroupDetectionOrder: pubbldr.DetectionOrder{
						pubbldr.DetectionOrderEntry{
							BuildpackRef: dist.BuildpackRef{ModuleInfo: testNestedBuildpack},
							GroupDetectionOrder: pubbldr.DetectionOrder{
								pubbldr.DetectionOrderEntry{
									BuildpackRef: dist.BuildpackRef{
										ModuleInfo: testBuildpackOne,
										Optional:   true,
									},
								},
							},
						},
						pubbldr.DetectionOrderEntry{
							BuildpackRef: dist.BuildpackRef{
								ModuleInfo: testBuildpackThree,
								Optional:   true,
							},
						},
						pubbldr.DetectionOrderEntry{
							BuildpackRef: dist.BuildpackRef{ModuleInfo: testNestedBuildpackTwo},
							GroupDetectionOrder: pubbldr.DetectionOrder{
								pubbldr.DetectionOrderEntry{
									BuildpackRef: dist.BuildpackRef{
										ModuleInfo: testBuildpackOne,
										Optional:   true,
									},
									Cyclical: true,
								},
							},
						},
					},
				},
				pubbldr.DetectionOrderEntry{
					BuildpackRef: dist.BuildpackRef{
						ModuleInfo: testBuildpackTwo,
						Optional:   true,
					},
				},
			},
		},
		pubbldr.DetectionOrderEntry{
			BuildpackRef: dist.BuildpackRef{
				ModuleInfo: testBuildpackThree,
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
