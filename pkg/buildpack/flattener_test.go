package buildpack_test

import (
	"github.com/buildpacks/pack/pkg/archive"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestFlattener(t *testing.T) {
	spec.Run(t, "Flattener", testFlattener, spec.Report(report.Terminal{}))
}

func testFlattener(t *testing.T, when spec.G, it spec.S) {
	var (
		bp1 buildpack.BuildModule
		bp2 buildpack.BuildModule
		err error
	)

	it.Before(func() {
		bp1, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version",
			},
		}, 0644)
		h.AssertNil(t, err)

		bp2, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "buildpack-2-id",
				Version: "buildpack-2-version",
			},
		}, 0644)
		h.AssertNil(t, err)
	})

	when("Create a NewBuildpackFlattenModule", func() {
		it("creates a buildpackFlattenModule with the correct information", func() {
			buildpackFlattenerModule := buildpack.NewBuildpacksFlattenerModule([]buildpack.BuildModule{bp1, bp2})

			h.AssertEq(t, len(buildpackFlattenerModule.Descriptors()), 2)
		})
	})

	when("Open is called", func() {
		var writeBlobToFile = func(bp buildpack.BuildFlattenModule) string {
			t.Helper()

			bpReader, err := bp.Open()
			h.AssertNil(t, err)

			tmpDir, err := os.MkdirTemp("", "")
			h.AssertNil(t, err)

			p := filepath.Join(tmpDir, "bp.tar")
			bpWriter, err := os.Create(p)
			h.AssertNil(t, err)

			_, err = io.Copy(bpWriter, bpReader)
			h.AssertNil(t, err)

			err = bpReader.Close()
			h.AssertNil(t, err)

			return p
		}

		it("Returns the reader about the content of any flatten buildpack", func() {
			buildpackFlattenerModule := buildpack.NewBuildpacksFlattenerModule([]buildpack.BuildModule{bp1, bp2})

			tarPath := writeBlobToFile(buildpackFlattenerModule)

			assertDirExists(t, tarPath, "/cnb/buildpacks/buildpack-1-id")
			assertDirExists(t, tarPath, "/cnb/buildpacks/buildpack-1-id/buildpack-1-version")
			assertDirExists(t, tarPath, "/cnb/buildpacks/buildpack-1-id/buildpack-1-version/bin")
			assertExecutableExists(t, tarPath, "/cnb/buildpacks/buildpack-1-id/buildpack-1-version/bin/build", "build-contents")
			assertExecutableExists(t, tarPath, "/cnb/buildpacks/buildpack-1-id/buildpack-1-version/bin/detect", "detect-contents")
			assertExecutableExists(t, tarPath, "/cnb/buildpacks/buildpack-2-id/buildpack-2-version/bin/build", "build-contents")
			assertExecutableExists(t, tarPath, "/cnb/buildpacks/buildpack-2-id/buildpack-2-version/bin/detect", "detect-contents")
		})
	})
}

func assertExecutableExists(t *testing.T, tarPath, entryPath, content string) {
	h.AssertOnTarEntry(t, tarPath, entryPath,
		h.HasFileMode(0755),
		h.HasModTime(archive.NormalizedDateTime),
		h.ContentEquals(content))
}

func assertDirExists(t *testing.T, tarPath string, entryPath string) {
	h.AssertOnTarEntry(t, tarPath, entryPath,
		h.IsDirectory(),
		h.HasFileMode(0755),
		h.HasModTime(archive.NormalizedDateTime),
	)
}
