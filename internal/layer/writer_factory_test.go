package layer_test

import (
	"archive/tar"
	"testing"

	"github.com/buildpacks/imgutil/fakes"
	ilayer "github.com/buildpacks/imgutil/layer"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/layer"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestTarWriterFactory(t *testing.T) {
	spec.Run(t, "WriterFactory", testWriterFactory, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testWriterFactory(t *testing.T, when spec.G, it spec.S) {
	when("#NewWriter", func() {
		it("returns a regular tar writer for posix-based images", func() {
			image := fakes.NewImage("fake-image", "", nil)
			image.SetPlatform("linux", "", "")
			factory, err := layer.NewWriterFactory(image)
			h.AssertNil(t, err)

			_, ok := factory.NewWriter(nil).(*tar.Writer)
			if !ok {
				t.Fatal("returned writer was not a regular tar writer")
			}
		})

		it("returns a Windows layer writer for Windows-based images", func() {
			image := fakes.NewImage("fake-image", "", nil)
			image.SetPlatform("windows", "", "")
			factory, err := layer.NewWriterFactory(image)
			h.AssertNil(t, err)

			_, ok := factory.NewWriter(nil).(*ilayer.WindowsWriter)
			if !ok {
				t.Fatal("returned writer was not a Windows layer writer")
			}
		})
	})
}
