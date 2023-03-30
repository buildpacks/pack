package archive_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/archive"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestTarFilesMerger(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Merge tars", testTarFilesMerger, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testTarFilesMerger(t *testing.T, when spec.G, it spec.S) {
	var (
		err    error
		tmpDir string
		input1 string
		input2 string
	)

	it.Before(func() {
		tmpDir, err = os.MkdirTemp("", "test-tar-merging")
		h.AssertNil(t, err)

		input1 = filepath.Join(tmpDir, "tar1")
		err = archive.CreateSingleFileTar(input1, "/foo/hello.txt", "hello foo")
		h.AssertNil(t, err)

		input2 = filepath.Join(tmpDir, "tar2")
		err = archive.CreateSingleFileTar(input2, "/bar/hello.txt", "hello bar")
		h.AssertNil(t, err)
	})

	it.After(func() {
		err = os.RemoveAll(tmpDir)
		if runtime.GOOS != "windows" {
			h.AssertNil(t, err)
		}
	})

	when("#MergeTars", func() {
		var output string
		it.Before(func() {
			output = filepath.Join(tmpDir, "output.tar")
		})

		it.After(func() {
			err = os.RemoveAll(output)
			if runtime.GOOS != "windows" {
				h.AssertNil(t, err)
			}
		})

		it("merges two tar files with content", func() {
			err = archive.MergeTars(output, input1, input2)
			h.AssertNil(t, err)

			h.AssertTarFileContents(t, output, "/foo/hello.txt", "hello foo")
			h.AssertTarFileContents(t, output, "/bar/hello.txt", "hello bar")
		})

		it("merges one tar file with content with another empty tar", func() {
			err = archive.CreateSingleFileTar(input2, "", "")
			h.AssertNil(t, err)

			err = archive.MergeTars(output, input1, input2)
			h.AssertNil(t, err)

			h.AssertTarFileContents(t, output, "/foo/hello.txt", "hello foo")
		})
	})
}
