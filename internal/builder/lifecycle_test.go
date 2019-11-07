package builder_test

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/internal/blob"
	"github.com/buildpack/pack/internal/builder"
	h "github.com/buildpack/pack/testhelpers"
)

func TestLifecycle(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "testLifecycle", testLifecycle, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLifecycle(t *testing.T, when spec.G, it spec.S) {
	when("#NewLifecycle", func() {
		it("makes a lifecycle from a blob", func() {
			lifecycle, err := builder.NewLifecycle(blob.NewBlob(filepath.Join("testdata", "lifecycle")))
			h.AssertNil(t, err)
			h.AssertEq(t, lifecycle.Descriptor().Info.Version.String(), "1.2.3")
			h.AssertEq(t, lifecycle.Descriptor().API.PlatformVersion.String(), "0.2")
			h.AssertEq(t, lifecycle.Descriptor().API.BuildpackVersion.String(), "0.3")
		})

		when("there is no descriptor file", func() {
			it("assumes 0.1 API versions", func() {
				lifecycle, err := builder.NewLifecycle(&fakeEmptyBlob{})
				h.AssertNil(t, err)
				h.AssertEq(t, lifecycle.Descriptor().Info.Version.String(), "0.3.0")
				h.AssertEq(t, lifecycle.Descriptor().API.PlatformVersion.String(), "0.1")
				h.AssertEq(t, lifecycle.Descriptor().API.BuildpackVersion.String(), "0.1")
			})
		})

		when("the lifecycle has incomplete list of binaries", func() {
			var tmpDir string

			it.Before(func() {
				var err error
				tmpDir, err = ioutil.TempDir("", "")
				h.AssertNil(t, err)

				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "lifecycle.toml"), []byte(`
[api]
  platform = "0.2"
  buildpack = "0.3"

[lifecycle]
  version = "1.2.3"
`), os.ModePerm))

				h.AssertNil(t, os.Mkdir(filepath.Join(tmpDir, "lifecycle"), os.ModePerm))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "lifecycle", "analyzer"), []byte("content"), os.ModePerm))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "lifecycle", "detector"), []byte("content"), os.ModePerm))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "lifecycle", "builder"), []byte("content"), os.ModePerm))
			})

			it.After(func() {
				h.AssertNil(t, os.RemoveAll(tmpDir))
			})

			it("returns an error", func() {
				_, err := builder.NewLifecycle(blob.NewBlob(tmpDir))
				h.AssertError(t, err, "validating binaries")
			})
		})
	})
}

type fakeEmptyBlob struct {
}

func (f *fakeEmptyBlob) Open() (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		tw := tar.NewWriter(pw)
		defer tw.Close()
	}()
	return pr, nil
}
