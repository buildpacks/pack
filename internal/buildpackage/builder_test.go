package buildpackage_test

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/buildpacks/imgutil/layer"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/lifecycle/api"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/heroku/color"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

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
		mockController   *gomock.Controller
		mockImageFactory *testmocks.MockImageFactory
		subject          *buildpackage.PackageBuilder
		tmpDir           string
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImageFactory = testmocks.NewMockImageFactory(mockController)

		fakePackageImage := fakes.NewImage("some/package", "", nil)
		mockImageFactory.EXPECT().NewImage("some/package", true).Return(fakePackageImage, nil).AnyTimes()

		subject = buildpackage.NewBuilder(mockImageFactory)

		var err error
		tmpDir, err = ioutil.TempDir("", "package_builder_tests")
		h.AssertNil(t, err)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tmpDir))
		mockController.Finish()
	})

	when("validation", func() {
		for _, test := range []struct {
			name string
			fn   func() error
		}{
			{name: "SaveAsImage", fn: func() error {
				_, err := subject.SaveAsImage("some/package", false, "linux")
				return err
			}},
			{name: "SaveAsImage", fn: func() error {
				_, err := subject.SaveAsImage("some/package", false, "windows")
				return err
			}},
			{name: "SaveAsFile", fn: func() error {
				return subject.SaveAsFile(path.Join(tmpDir, "package.cnb"), "windows")
			}},
			{name: "SaveAsFile", fn: func() error {
				return subject.SaveAsFile(path.Join(tmpDir, "package.cnb"), "linux")
			}},
		} {
			testFn := test.fn
			when(test.name, func() {
				when("validate buildpack", func() {
					when("buildpack not set", func() {
						it("returns error", func() {
							err := testFn()
							h.AssertError(t, err, "buildpack must be set")
						})
					})

					when("there is a buildpack not referenced", func() {
						it("should error", func() {
							bp1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
								API: api.MustParse("0.2"),
								Info: dist.BuildpackInfo{
									ID:      "bp.1.id",
									Version: "bp.1.version",
								},
								Stacks: []dist.Stack{{ID: "some.stack"}},
							}, 0644)
							h.AssertNil(t, err)
							subject.SetBuildpack(bp1)

							bp2, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
								API:    api.MustParse("0.2"),
								Info:   dist.BuildpackInfo{ID: "bp.2.id", Version: "bp.2.version"},
								Stacks: []dist.Stack{{ID: "some.stack"}},
								Order:  nil,
							}, 0644)
							h.AssertNil(t, err)
							subject.AddDependency(bp2)

							err = testFn()
							h.AssertError(t, err, "buildpack 'bp.2.id@bp.2.version' is not used by buildpack 'bp.1.id@bp.1.version'")
						})
					})

					when("there is a referenced buildpack from main buildpack that is not present", func() {
						it("should error", func() {
							mainBP, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
								API: api.MustParse("0.2"),
								Info: dist.BuildpackInfo{
									ID:      "bp.1.id",
									Version: "bp.1.version",
								},
								Order: dist.Order{{
									Group: []dist.BuildpackRef{
										{BuildpackInfo: dist.BuildpackInfo{ID: "bp.present.id", Version: "bp.present.version"}},
										{BuildpackInfo: dist.BuildpackInfo{ID: "bp.missing.id", Version: "bp.missing.version"}},
									},
								}},
							}, 0644)
							h.AssertNil(t, err)
							subject.SetBuildpack(mainBP)

							presentBP, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
								API:    api.MustParse("0.2"),
								Info:   dist.BuildpackInfo{ID: "bp.present.id", Version: "bp.present.version"},
								Stacks: []dist.Stack{{ID: "some.stack"}},
								Order:  nil,
							}, 0644)
							h.AssertNil(t, err)
							subject.AddDependency(presentBP)

							err = testFn()
							h.AssertError(t, err, "buildpack 'bp.1.id@bp.1.version' references buildpack 'bp.missing.id@bp.missing.version' which is not present")
						})
					})

					when("there is a referenced buildpack from dependency buildpack that is not present", func() {
						it("should error", func() {
							mainBP, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
								API: api.MustParse("0.2"),
								Info: dist.BuildpackInfo{
									ID:      "bp.1.id",
									Version: "bp.1.version",
								},
								Order: dist.Order{{
									Group: []dist.BuildpackRef{
										{BuildpackInfo: dist.BuildpackInfo{ID: "bp.present.id", Version: "bp.present.version"}},
									},
								}},
							}, 0644)
							h.AssertNil(t, err)
							subject.SetBuildpack(mainBP)

							presentBP, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
								API:  api.MustParse("0.2"),
								Info: dist.BuildpackInfo{ID: "bp.present.id", Version: "bp.present.version"},
								Order: dist.Order{{
									Group: []dist.BuildpackRef{
										{BuildpackInfo: dist.BuildpackInfo{ID: "bp.missing.id", Version: "bp.missing.version"}},
									},
								}},
							}, 0644)
							h.AssertNil(t, err)
							subject.AddDependency(presentBP)

							err = testFn()
							h.AssertError(t, err, "buildpack 'bp.present.id@bp.present.version' references buildpack 'bp.missing.id@bp.missing.version' which is not present")
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

							err = testFn()
							h.AssertNil(t, err)
						})
					})

					when("dependencies don't have a common stack", func() {
						it("should error", func() {
							buildpack, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
								API: api.MustParse("0.2"),
								Info: dist.BuildpackInfo{
									ID:      "bp.1.id",
									Version: "bp.1.version",
								},
								Order: dist.Order{{
									Group: []dist.BuildpackRef{{
										BuildpackInfo: dist.BuildpackInfo{ID: "bp.2.id", Version: "bp.2.version"},
										Optional:      false,
									}, {
										BuildpackInfo: dist.BuildpackInfo{ID: "bp.3.id", Version: "bp.3.version"},
										Optional:      false,
									}},
								}},
							}, 0644)
							h.AssertNil(t, err)
							subject.SetBuildpack(buildpack)

							dependency1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
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
							subject.AddDependency(dependency1)

							dependency2, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
								API: api.MustParse("0.2"),
								Info: dist.BuildpackInfo{
									ID:      "bp.3.id",
									Version: "bp.3.version",
								},
								Stacks: []dist.Stack{
									{ID: "stack.id.3", Mixins: []string{"Mixin-A"}},
								},
								Order: nil,
							}, 0644)
							h.AssertNil(t, err)
							subject.AddDependency(dependency2)

							_, err = subject.SaveAsImage("some/package", false, "linux")
							h.AssertError(t, err, "no compatible stacks among provided buildpacks")
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
								Order: dist.Order{{
									Group: []dist.BuildpackRef{{
										BuildpackInfo: dist.BuildpackInfo{ID: "bp.2.id", Version: "bp.2.version"},
										Optional:      false,
									}, {
										BuildpackInfo: dist.BuildpackInfo{ID: "bp.3.id", Version: "bp.3.version"},
										Optional:      false,
									}},
								}},
							}, 0644)
							h.AssertNil(t, err)
							subject.SetBuildpack(buildpack)

							dependency1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
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
							subject.AddDependency(dependency1)

							dependency2, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
								API: api.MustParse("0.2"),
								Info: dist.BuildpackInfo{
									ID:      "bp.3.id",
									Version: "bp.3.version",
								},
								Stacks: []dist.Stack{
									{ID: "stack.id.1", Mixins: []string{"Mixin-A"}},
								},
								Order: nil,
							}, 0644)
							h.AssertNil(t, err)
							subject.AddDependency(dependency2)

							img, err := subject.SaveAsImage("some/package", false, "linux")
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

							img, err := subject.SaveAsImage("some/package", false, "linux")
							h.AssertNil(t, err)

							metadata := buildpackage.Metadata{}
							_, err = dist.GetLabel(img, "io.buildpacks.buildpackage.metadata", &metadata)
							h.AssertNil(t, err)

							h.AssertEq(t, metadata.Stacks, []dist.Stack{{ID: "stack.id.1", Mixins: []string{"Mixin-A"}}})
						})
					})
				})
			})
		}
	})

	when("#SaveAsImage", func() {
		it("sets metadata", func() {
			buildpack1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
				API: api.MustParse("0.2"),
				Info: dist.BuildpackInfo{
					ID:          "bp.1.id",
					Version:     "bp.1.version",
					Name:        "One",
					Description: "some description",
					Homepage:    "https://example.com/homepage",
					Keywords:    []string{"some-keyword"},
					Licenses: []dist.License{
						{
							Type: "MIT",
							URI:  "https://example.com/license",
						},
					},
				},
				Stacks: []dist.Stack{
					{ID: "stack.id.1"},
					{ID: "stack.id.2"},
				},
				Order: nil,
			}, 0644)
			h.AssertNil(t, err)

			subject.SetBuildpack(buildpack1)

			packageImage, err := subject.SaveAsImage("some/package", false, "linux")
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
			h.AssertEq(t, md.Keywords[0], "some-keyword")
			h.AssertEq(t, md.Homepage, "https://example.com/homepage")
			h.AssertEq(t, md.Name, "One")
			h.AssertEq(t, md.Description, "some description")
			h.AssertEq(t, md.Licenses[0].Type, "MIT")
			h.AssertEq(t, md.Licenses[0].URI, "https://example.com/license")

			osVal, err := packageImage.OS()
			h.AssertNil(t, err)
			h.AssertEq(t, osVal, "linux")
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

			packageImage, err := subject.SaveAsImage("some/package", false, "linux")
			h.AssertNil(t, err)

			var bpLayers dist.BuildpackLayers
			_, err = dist.GetLabel(packageImage, "io.buildpacks.buildpack.layers", &bpLayers)
			h.AssertNil(t, err)

			bp1Info, ok1 := bpLayers["bp.1.id"]["bp.1.version"]
			h.AssertEq(t, ok1, true)
			h.AssertEq(t, bp1Info.Stacks, []dist.Stack{{ID: "stack.id.1"}, {ID: "stack.id.2"}})
		})

		it("adds buildpack layers for linux", func() {
			buildpack1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
				API:    api.MustParse("0.2"),
				Info:   dist.BuildpackInfo{ID: "bp.1.id", Version: "bp.1.version"},
				Stacks: []dist.Stack{{ID: "stack.id.1"}, {ID: "stack.id.2"}},
				Order:  nil,
			}, 0644)
			h.AssertNil(t, err)
			subject.SetBuildpack(buildpack1)

			packageImage, err := subject.SaveAsImage("some/package", false, "linux")
			h.AssertNil(t, err)

			buildpackExists := func(name, version string) {
				t.Helper()
				dirPath := fmt.Sprintf("/cnb/buildpacks/%s/%s", name, version)
				fakePackageImage := packageImage.(*fakes.Image)
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

			fakePackageImage := packageImage.(*fakes.Image)
			h.AssertEq(t, fakePackageImage.NumberOfAddedLayers(), 1)
		})

		it("adds baselayer + buildpack layers for windows", func() {
			buildpack1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
				API:    api.MustParse("0.2"),
				Info:   dist.BuildpackInfo{ID: "bp.1.id", Version: "bp.1.version"},
				Stacks: []dist.Stack{{ID: "stack.id.1"}, {ID: "stack.id.2"}},
				Order:  nil,
			}, 0644)
			h.AssertNil(t, err)
			subject.SetBuildpack(buildpack1)

			packageImage, err := subject.SaveAsImage("some/package", false, "windows")
			h.AssertNil(t, err)

			fakePackageImage := packageImage.(*fakes.Image)

			osVal, err := fakePackageImage.OS()
			h.AssertNil(t, err)
			h.AssertEq(t, osVal, "windows")

			h.AssertEq(t, fakePackageImage.NumberOfAddedLayers(), 2)
		})
	})

	when("#SaveAsFile", func() {
		it("sets metadata", func() {
			buildpack1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
				API:    api.MustParse("0.2"),
				Info:   dist.BuildpackInfo{ID: "bp.1.id", Version: "bp.1.version"},
				Stacks: []dist.Stack{{ID: "stack.id.1"}, {ID: "stack.id.2"}},
				Order:  nil,
			}, 0644)
			h.AssertNil(t, err)
			subject.SetBuildpack(buildpack1)

			outputFile := filepath.Join(tmpDir, fmt.Sprintf("package-%s.cnb", h.RandString(10)))
			h.AssertNil(t, subject.SaveAsFile(outputFile, "linux"))

			withContents := func(fn func(data []byte)) h.TarEntryAssertion {
				return func(t *testing.T, header *tar.Header, data []byte) {
					fn(data)
				}
			}

			h.AssertOnTarEntry(t, outputFile, "/index.json",
				h.HasOwnerAndGroup(0, 0),
				h.HasFileMode(0755),
				withContents(func(data []byte) {
					index := v1.Index{}
					err := json.Unmarshal(data, &index)
					h.AssertNil(t, err)
					h.AssertEq(t, len(index.Manifests), 1)

					// manifest: application/vnd.docker.distribution.manifest.v2+json
					h.AssertOnTarEntry(t, outputFile,
						"/blobs/sha256/"+index.Manifests[0].Digest.Hex(),
						h.HasOwnerAndGroup(0, 0),
						h.IsJSON(),

						withContents(func(data []byte) {
							manifest := v1.Manifest{}
							err := json.Unmarshal(data, &manifest)
							h.AssertNil(t, err)

							// config: application/vnd.docker.container.image.v1+json
							h.AssertOnTarEntry(t, outputFile,
								"/blobs/sha256/"+manifest.Config.Digest.Hex(),
								h.HasOwnerAndGroup(0, 0),
								h.IsJSON(),
								// buildpackage metadata
								h.ContentContains(`"io.buildpacks.buildpackage.metadata":"{\"id\":\"bp.1.id\",\"version\":\"bp.1.version\",\"stacks\":[{\"id\":\"stack.id.1\"},{\"id\":\"stack.id.2\"}]}"`),
								// buildpack layers metadata
								h.ContentContains(`"io.buildpacks.buildpack.layers":"{\"bp.1.id\":{\"bp.1.version\":{\"api\":\"0.2\",\"stacks\":[{\"id\":\"stack.id.1\"},{\"id\":\"stack.id.2\"}],\"layerDiffID\":\"sha256:9fa0bb03eebdd0f8e4b6d6f50471b44be83dba750624dfce15dac45975c5707b\"}}`),
								// image os
								h.ContentContains(`"os":"linux"`),
							)
						}))
				}))
		})

		it("adds buildpack layers for linux", func() {
			buildpack1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
				API:    api.MustParse("0.2"),
				Info:   dist.BuildpackInfo{ID: "bp.1.id", Version: "bp.1.version"},
				Stacks: []dist.Stack{{ID: "stack.id.1"}, {ID: "stack.id.2"}},
				Order:  nil,
			}, 0644)
			h.AssertNil(t, err)
			subject.SetBuildpack(buildpack1)

			outputFile := filepath.Join(tmpDir, fmt.Sprintf("package-%s.cnb", h.RandString(10)))
			h.AssertNil(t, subject.SaveAsFile(outputFile, "linux"))

			h.AssertOnTarEntry(t, outputFile, "/blobs",
				h.IsDirectory(),
				h.HasOwnerAndGroup(0, 0),
				h.HasFileMode(0755))
			h.AssertOnTarEntry(t, outputFile, "/blobs/sha256",
				h.IsDirectory(),
				h.HasOwnerAndGroup(0, 0),
				h.HasFileMode(0755))

			bpReader, err := buildpack1.Open()
			h.AssertNil(t, err)
			defer bpReader.Close()

			// layer: application/vnd.docker.image.rootfs.diff.tar.gzip
			buildpackLayerSHA, err := computeLayerSHA(bpReader)
			h.AssertNil(t, err)
			h.AssertOnTarEntry(t, outputFile,
				"/blobs/sha256/"+buildpackLayerSHA,
				h.HasOwnerAndGroup(0, 0),
				h.HasFileMode(0755),
				h.IsGzipped(),
				h.AssertOnNestedTar("/cnb/buildpacks/bp.1.id",
					h.IsDirectory(),
					h.HasOwnerAndGroup(0, 0),
					h.HasFileMode(0644)),
				h.AssertOnNestedTar("/cnb/buildpacks/bp.1.id/bp.1.version/bin/build",
					h.ContentEquals("build-contents"),
					h.HasOwnerAndGroup(0, 0),
					h.HasFileMode(0644)),
				h.AssertOnNestedTar("/cnb/buildpacks/bp.1.id/bp.1.version/bin/detect",
					h.ContentEquals("detect-contents"),
					h.HasOwnerAndGroup(0, 0),
					h.HasFileMode(0644)))
		})

		it("adds baselayer + buildpack layers for windows", func() {
			buildpack1, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
				API:    api.MustParse("0.2"),
				Info:   dist.BuildpackInfo{ID: "bp.1.id", Version: "bp.1.version"},
				Stacks: []dist.Stack{{ID: "stack.id.1"}, {ID: "stack.id.2"}},
				Order:  nil,
			}, 0644)
			h.AssertNil(t, err)
			subject.SetBuildpack(buildpack1)

			outputFile := filepath.Join(tmpDir, fmt.Sprintf("package-%s.cnb", h.RandString(10)))
			h.AssertNil(t, subject.SaveAsFile(outputFile, "windows"))

			// Windows baselayer content is constant
			expectedBaseLayerReader, err := layer.WindowsBaseLayer()
			h.AssertNil(t, err)

			// layer: application/vnd.docker.image.rootfs.diff.tar.gzip
			expectedBaseLayerSHA, err := computeLayerSHA(ioutil.NopCloser(expectedBaseLayerReader))
			h.AssertNil(t, err)
			h.AssertOnTarEntry(t, outputFile,
				"/blobs/sha256/"+expectedBaseLayerSHA,
				h.HasOwnerAndGroup(0, 0),
				h.HasFileMode(0755),
				h.IsGzipped(),
			)

			bpReader, err := buildpack1.Open()
			h.AssertNil(t, err)
			defer bpReader.Close()

			buildpackLayerSHA, err := computeLayerSHA(bpReader)
			h.AssertNil(t, err)
			h.AssertOnTarEntry(t, outputFile,
				"/blobs/sha256/"+buildpackLayerSHA,
				h.HasOwnerAndGroup(0, 0),
				h.HasFileMode(0755),
				h.IsGzipped(),
			)
		})
	})
}

func computeLayerSHA(reader io.ReadCloser) (string, error) {
	bpLayer := stream.NewLayer(reader, stream.WithCompressionLevel(gzip.DefaultCompression))
	compressed, err := bpLayer.Compressed()
	if err != nil {
		return "", err
	}
	defer compressed.Close()

	if _, err := io.Copy(ioutil.Discard, compressed); err != nil {
		return "", err
	}

	digest, err := bpLayer.Digest()
	if err != nil {
		return "", err
	}

	return digest.Hex, nil
}
