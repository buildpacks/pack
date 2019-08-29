package builder_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpack/pack/blob"

	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/builder"
	h "github.com/buildpack/pack/testhelpers"
)

func TestBuildpack(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "buildpack", testBuildpack, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildpack(t *testing.T, when spec.G, it spec.S) {
	var tmpBpDir string

	it.Before(func() {
		var err error
		tmpBpDir, err = ioutil.TempDir("", "")
		h.AssertNil(t, err)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tmpBpDir))
	})

	when("#NewBuildpack", func() {
		it("makes a buildpack from a blob", func() {
			h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpBpDir, "buildpack.toml"), []byte(`
[buildpack]
id = "bp.one"
version = "1.2.3"

[[stacks]]
id = "some.stack.id"
`), os.ModePerm))

			bp, err := builder.NewBuildpack(blob.NewBlob(tmpBpDir))
			h.AssertNil(t, err)
			h.AssertEq(t, bp.Descriptor().Info.ID, "bp.one")
			h.AssertEq(t, bp.Descriptor().Info.Version, "1.2.3")
			h.AssertEq(t, bp.Descriptor().Stacks[0].ID, "some.stack.id")
		})

		when("there is no descriptor file", func() {
			it("returns error", func() {
				_, err := builder.NewBuildpack(blob.NewBlob(tmpBpDir))
				h.AssertError(t, err, "could not find entry path 'buildpack.toml'")
			})
		})

		when("both stacks and order are present", func() {
			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpBpDir, "buildpack.toml"), []byte(`
[buildpack]
id = "bp.one"
version = "1.2.3"

[[stacks]]
id = "some.stack.id"

[[order]]
[[order.group]]
  id = "bp.nested"
  version = "bp.nested.version"
`), os.ModePerm))
			})

			it("returns error", func() {
				_, err := builder.NewBuildpack(blob.NewBlob(tmpBpDir))
				h.AssertError(t, err, "cannot have both stacks and an order defined")
			})

		})

		when("missing stacks and order", func() {

			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpBpDir, "buildpack.toml"), []byte(`
[buildpack]
id = "bp.one"
version = "1.2.3"
`), os.ModePerm))
			})

			it("returns error", func() {
				_, err := builder.NewBuildpack(blob.NewBlob(tmpBpDir))
				h.AssertError(t, err, "must have either stacks or an order defined")
			})
		})
	})
}
