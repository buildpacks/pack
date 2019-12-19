package buildpackage_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/api"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
)

func TestPackageBuilder(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "PackageBuilder", testPackageBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPackageBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		fakePackageImage *fakes.Image
		mockController   *gomock.Controller
		mockImageFactory *testmocks.MockImageFactory
		subject          *buildpackage.PackageBuilder
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImageFactory = testmocks.NewMockImageFactory(mockController)

		fakePackageImage = fakes.NewImage("some/package", "", nil)
		mockImageFactory.EXPECT().NewImage("some/package", true).Return(fakePackageImage, nil).AnyTimes()

		subject = buildpackage.NewBuilder(mockImageFactory)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#Save", func() {
		when("validate buildpack", func() {
			when("buildpack not set", func() {
				it("returns error", func() {
					_, err := subject.Save(fakePackageImage.Name(), false)
					h.AssertError(t, err, "buildpack must be set")
				})
			})
		})

		when("validate stacks", func() {
			when("buildpack is meta-buildpack", func() {
				it("should succeed", func() {
					buildpack, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.1.id",
							Version: "bp.1.version",
						},
						Stacks: nil,
						Order: dist.Order{{
							Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "bp.nested.id", Version: "bp.nested.version"}},
							},
						}},
					}, 0644)
					h.AssertNil(t, err)

					subject.SetBuildpack(buildpack)

					dependency, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.nested.id",
							Version: "bp.nested.version",
						},
						Stacks: []dist.Stack{
							{ID: "stack.id.1", Mixins: []string{"Mixin-A"}},
						},
						Order: nil,
					}, 0644)
					h.AssertNil(t, err)

					subject.AddDependency(dependency)

					_, err = subject.Save("some/package", false)
					h.AssertNil(t, err)
				})
			})

			when("buildpack is meta-buildpack and its dependency is missing", func() {
				it("should return an error", func() {
					buildpack, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.1.id",
							Version: "bp.1.version",
						},
						Stacks: nil,
						Order: dist.Order{{
							Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "bp.nested.id", Version: "bp.nested.version"}},
							},
						}},
					}, 0644)
					h.AssertNil(t, err)

					subject.SetBuildpack(buildpack)

					_, err = subject.Save("some/package", false)
					h.AssertError(t, err, "no compatible stacks among provided buildpacks")
				})
			})

			when("dependency does not have any matching stack", func() {
				it("should error", func() {
					buildpack, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.1.id",
							Version: "bp.1.version",
						},
						Stacks: []dist.Stack{
							{ID: "stack.id.1", Mixins: []string{"Mixin-A"}},
						},
						Order: nil,
					}, 0644)
					h.AssertNil(t, err)

					subject.SetBuildpack(buildpack)

					dependency, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.2.id",
							Version: "bp.2.version",
						},
						Stacks: []dist.Stack{
							{ID: "stack.id.2", Mixins: []string{"Mixin-A"}},
						},
						Order: nil,
					}, 0644)
					h.AssertNil(t, err)

					subject.AddDependency(dependency)

					_, err = subject.Save("some/package", false)
					h.AssertError(t, err, "buildpack 'bp.1.id@bp.1.version' does not support any stacks from 'bp.2.id@bp.2.version'")
				})
			})

			when("dependency has stacks that aren't supported by buildpack", func() {
				it("should only support common stacks", func() {
					buildpack, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.1.id",
							Version: "bp.1.version",
						},
						Stacks: []dist.Stack{
							{ID: "stack.id.1", Mixins: []string{"Mixin-A"}},
						},
						Order: nil,
					}, 0644)
					h.AssertNil(t, err)

					subject.SetBuildpack(buildpack)

					dependency, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.2.id",
							Version: "bp.2.version",
						},
						Stacks: []dist.Stack{
							{ID: "stack.id.1", Mixins: []string{"Mixin-A"}},
							{ID: "stack.id.2", Mixins: []string{"Mixin-A"}},
						},
						Order: nil,
					}, 0644)
					h.AssertNil(t, err)

					subject.AddDependency(dependency)

					img, err := subject.Save("some/package", false)
					h.AssertNil(t, err)

					metadata := buildpackage.Metadata{}
					_, err = dist.GetLabel(img, "io.buildpacks.buildpackage.metadata", &metadata)
					h.AssertNil(t, err)

					h.AssertEq(t, metadata.Stacks, []dist.Stack{{ID: "stack.id.1", Mixins: []string{"Mixin-A"}}})
				})
			})

			when("dependency is meta-buildpack", func() {
				it("should succeed and compute common stacks", func() {
					buildpack, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.1.id",
							Version: "bp.1.version",
						},
						Stacks: nil,
						Order: dist.Order{{
							Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "bp.nested.id", Version: "bp.nested.version"}},
							},
						}},
					}, 0644)
					h.AssertNil(t, err)

					subject.SetBuildpack(buildpack)

					dependencyOrder, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.nested.id",
							Version: "bp.nested.version",
						},
						Order: dist.Order{{
							Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{
									ID:      "bp.nested.nested.id",
									Version: "bp.nested.nested.version",
								}},
							},
						}},
					}, 0644)
					h.AssertNil(t, err)

					subject.AddDependency(dependencyOrder)

					dependencyNestedNested, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.nested.nested.id",
							Version: "bp.nested.nested.version",
						},
						Stacks: []dist.Stack{
							{ID: "stack.id.1", Mixins: []string{"Mixin-A"}},
						},
						Order: nil,
					}, 0644)
					h.AssertNil(t, err)

					subject.AddDependency(dependencyNestedNested)

					img, err := subject.Save("some/package", false)
					h.AssertNil(t, err)

					metadata := buildpackage.Metadata{}
					_, err = dist.GetLabel(img, "io.buildpacks.buildpackage.metadata", &metadata)
					h.AssertNil(t, err)

					h.AssertEq(t, metadata.Stacks, []dist.Stack{{ID: "stack.id.1", Mixins: []string{"Mixin-A"}}})
				})
			})

			when("dependency is meta-buildpack and its dependency is missing", func() {
				it("should return an error", func() {
					buildpack, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.1.id",
							Version: "bp.1.version",
						},
						Stacks: nil,
						Order: dist.Order{{
							Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "bp.nested.id", Version: "bp.nested.version"}},
							},
						}},
					}, 0644)
					h.AssertNil(t, err)

					subject.SetBuildpack(buildpack)

					dependencyOrder, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
						API: api.MustParse("0.2"),
						Info: dist.BuildpackInfo{
							ID:      "bp.nested.id",
							Version: "bp.nested.version",
						},
						Order: dist.Order{{
							Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{
									ID:      "bp.nested.nested.id",
									Version: "bp.nested.nested.version",
								}},
							},
						}},
					}, 0644)
					h.AssertNil(t, err)

					subject.AddDependency(dependencyOrder)

					_, err = subject.Save("some/package", false)
					h.AssertError(t, err, "no compatible stacks among provided buildpacks")
				})
			})
		})

		it("sets metadata", func() {
			buildpack1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
				API: api.MustParse("0.2"),
				Info: dist.BuildpackInfo{
					ID:      "bp.1.id",
					Version: "bp.1.version",
				},
				Stacks: []dist.Stack{
					{ID: "stack.id.1"},
					{ID: "stack.id.2"},
				},
				Order: nil,
			}, 0644)
			h.AssertNil(t, err)

			subject.SetBuildpack(buildpack1)

			packageImage, err := subject.Save(fakePackageImage.Name(), false)
			h.AssertNil(t, err)

			labelData, err := packageImage.Label("io.buildpacks.buildpackage.metadata")
			h.AssertNil(t, err)
			var md buildpackage.Metadata
			h.AssertNil(t, json.Unmarshal([]byte(labelData), &md))

			h.AssertEq(t, md.ID, "bp.1.id")
			h.AssertEq(t, md.Version, "bp.1.version")
			h.AssertEq(t, len(md.Stacks), 2)
			h.AssertEq(t, md.Stacks[0].ID, "stack.id.1")
			h.AssertEq(t, md.Stacks[1].ID, "stack.id.2")
		})

		it("sets buildpack layers label", func() {
			buildpack1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
				API:    api.MustParse("0.2"),
				Info:   dist.BuildpackInfo{ID: "bp.1.id", Version: "bp.1.version"},
				Stacks: []dist.Stack{{ID: "stack.id.1"}, {ID: "stack.id.2"}},
				Order:  nil,
			}, 0644)
			h.AssertNil(t, err)
			subject.SetBuildpack(buildpack1)

			_, err = subject.Save(fakePackageImage.Name(), false)
			h.AssertNil(t, err)

			var bpLayers dist.BuildpackLayers
			_, err = dist.GetLabel(fakePackageImage, "io.buildpacks.buildpack.layers", &bpLayers)
			h.AssertNil(t, err)

			bp1Info, ok1 := bpLayers["bp.1.id"]["bp.1.version"]
			h.AssertEq(t, ok1, true)
			h.AssertEq(t, bp1Info.Stacks, []dist.Stack{{ID: "stack.id.1"}, {ID: "stack.id.2"}})
		})

		it("adds buildpack layers", func() {
			buildpack1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
				API:    api.MustParse("0.2"),
				Info:   dist.BuildpackInfo{ID: "bp.1.id", Version: "bp.1.version"},
				Stacks: []dist.Stack{{ID: "stack.id.1"}, {ID: "stack.id.2"}},
				Order:  nil,
			}, 0644)
			h.AssertNil(t, err)
			subject.SetBuildpack(buildpack1)

			_, err = subject.Save(fakePackageImage.Name(), false)
			h.AssertNil(t, err)

			buildpackExists := func(name, version string) {
				dirPath := fmt.Sprintf("/cnb/buildpacks/%s/%s", name, version)
				layerTar, err := fakePackageImage.FindLayerWithPath(dirPath)
				h.AssertNil(t, err)

				h.AssertOnTarEntry(t, layerTar, dirPath,
					h.IsDirectory(),
				)

				h.AssertOnTarEntry(t, layerTar, dirPath+"/bin/build",
					h.ContentEquals("build-contents"),
					h.HasOwnerAndGroup(0, 0),
					h.HasFileMode(0644),
				)

				h.AssertOnTarEntry(t, layerTar, dirPath+"/bin/detect",
					h.ContentEquals("detect-contents"),
					h.HasOwnerAndGroup(0, 0),
					h.HasFileMode(0644),
				)
			}

			buildpackExists("bp.1.id", "bp.1.version")
		})
	})
}
