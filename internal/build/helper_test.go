package build_test

import (
	"archive/tar"
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestHelper(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "helperForBuildExtension", testHelper, spec.Report(report.Terminal{}), spec.Sequential())
}

func testHelper(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir     string
		extensions build.Extensions
		logger     *logging.LogWithWriters
	)
	it.Before(func() {
		var outBuf bytes.Buffer
		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
	})
	when("set-extensions", func() {
		it("should set single extension", func() {
			tmpDir = filepath.Join(".", "testdata", "fake-tmp", "build-extension", "single")
			expectedExtension := build.Extensions{
				Extensions: []buildpack.GroupElement{
					{
						ID:       "samples/test",
						Version:  "0.0.1",
						API:      "0.9",
						Homepage: "https://github.com/buildpacks/samples/test/main/extensions/test",
					},
				},
			}
			extensions.SetExtensions(tmpDir, logger)
			h.AssertEq(t, extensions.Extensions[0].ID, expectedExtension.Extensions[0].ID)
			h.AssertEq(t, extensions.Extensions[0].Version, expectedExtension.Extensions[0].Version)
			h.AssertEq(t, extensions.Extensions[0].API, expectedExtension.Extensions[0].API)
			h.AssertEq(t, extensions.Extensions[0].Homepage, expectedExtension.Extensions[0].Homepage)
		})
		it("should set multiple extensions", func() {
			tmpDir = filepath.Join(".", "testdata", "fake-tmp", "build-extension", "multi")
			expectedExtension := build.Extensions{
				Extensions: []buildpack.GroupElement{
					{
						ID:       "samples/tree",
						Version:  "0.0.1",
						API:      "0.9",
						Homepage: "https://github.com/buildpacks/samples/tree/main/extensions/tree",
					},
					{
						ID:       "samples/test",
						Version:  "0.0.1",
						API:      "0.9",
						Homepage: "https://github.com/buildpacks/samples/test/main/extensions/test",
					},
				}}
			extensions.SetExtensions(tmpDir, logger)
			h.AssertEq(t, extensions.Extensions[0].ID, expectedExtension.Extensions[0].ID)
			h.AssertEq(t, extensions.Extensions[0].Version, expectedExtension.Extensions[0].Version)
			h.AssertEq(t, extensions.Extensions[0].API, expectedExtension.Extensions[0].API)
			h.AssertEq(t, extensions.Extensions[0].Homepage, expectedExtension.Extensions[0].Homepage)
			h.AssertEq(t, extensions.Extensions[1].ID, expectedExtension.Extensions[1].ID)
			h.AssertEq(t, extensions.Extensions[1].Version, expectedExtension.Extensions[1].Version)
			h.AssertEq(t, extensions.Extensions[1].API, expectedExtension.Extensions[1].API)
			h.AssertEq(t, extensions.Extensions[1].Homepage, expectedExtension.Extensions[1].Homepage)
		})
	})

	when("set dockerfiles", func() {
		it("should set dockerfiles for single extension", func() {
			tmpDir = filepath.Join(".", "testdata", "fake-tmp", "build-extension", "single")
			expectedDockerfile := build.DockerfileInfo{
				Info: &buildpack.DockerfileInfo{
					ExtensionID: "samples/test",
					Kind:        build.DockerfileKindBuild,
					Path:        filepath.Join(".", "testdata", "fake-tmp", "build-extension", "single", "build", "samples_test", "Dockerfile"),
				},
			}
			extensions.SetExtensions(tmpDir, logger)
			dockerfiles, err := extensions.DockerFiles(build.DockerfileKindBuild, tmpDir, logger)
			h.AssertNil(t, err)
			h.AssertEq(t, dockerfiles[0].Info.ExtensionID, expectedDockerfile.Info.ExtensionID)
			h.AssertEq(t, dockerfiles[0].Info.Kind, expectedDockerfile.Info.Kind)
			h.AssertEq(t, dockerfiles[0].Info.Path, expectedDockerfile.Info.Path)
		})
		it("should set dockerfiles for multiple extensions", func() {
			tmpDir = filepath.Join(".", "testdata", "fake-tmp", "build-extension", "multi")
			expectedDockerfiles := []build.DockerfileInfo{
				{
					Info: &buildpack.DockerfileInfo{
						ExtensionID: "samples/tree",
						Kind:        build.DockerfileKindBuild,
						Path:        filepath.Join(".", "testdata", "fake-tmp", "build-extension", "multi", "build", "samples_tree", "Dockerfile"),
					},
				},
				{
					Info: &buildpack.DockerfileInfo{
						ExtensionID: "samples/test",
						Kind:        build.DockerfileKindBuild,
						Path:        filepath.Join(".", "testdata", "fake-tmp", "build-extension", "multi", "build", "samples_test", "Dockerfile"),
					},
				},
			}
			extensions.SetExtensions(tmpDir, logger)
			dockerfiles, err := extensions.DockerFiles(build.DockerfileKindBuild, tmpDir, logger)
			h.AssertNil(t, err)
			h.AssertEq(t, dockerfiles[0].Info.ExtensionID, expectedDockerfiles[0].Info.ExtensionID)
			h.AssertEq(t, dockerfiles[0].Info.Kind, expectedDockerfiles[0].Info.Kind)
			h.AssertEq(t, dockerfiles[0].Info.Path, expectedDockerfiles[0].Info.Path)
			h.AssertEq(t, dockerfiles[1].Info.ExtensionID, expectedDockerfiles[1].Info.ExtensionID)
			h.AssertEq(t, dockerfiles[1].Info.Kind, expectedDockerfiles[1].Info.Kind)
			h.AssertEq(t, dockerfiles[1].Info.Path, expectedDockerfiles[1].Info.Path)
		})
	})

	when("create build context", func() {
		it("should create build context", func() {
			tmpDir = filepath.Join(".", "testdata", "fake-tmp", "build-extension", "single")
			extensions.SetExtensions(tmpDir, logger)
			dockerfiles, err := extensions.DockerFiles(build.DockerfileKindBuild, tmpDir, logger)
			h.AssertNil(t, err)
			buildContext, err := dockerfiles[0].CreateBuildContext(tmpDir, logger)
			h.AssertNil(t, err)
			tr := tar.NewReader(buildContext)
			checkDirectoryInTar(t, tr, "/workspace/build")
			checkFileInTar(t, tr, "Dockerfile")
		})
	})
}

func checkDirectoryInTar(t *testing.T, tr *tar.Reader, directoryName string) {
	for {
		header, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("directory %s not found", directoryName)
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == directoryName && header.Typeflag == tar.TypeDir {
			return
		}
	}
}

func checkFileInTar(t *testing.T, tr *tar.Reader, fileName string) {
	for {
		header, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("file %s not found", fileName)
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == fileName && header.Typeflag == tar.TypeReg {
			return
		}
	}
}
