package buildpack_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/lifecycle/mocks"

	"github.com/buildpack/pack/buildpack"
	h "github.com/buildpack/pack/testhelpers"
)

func TestBuildpackFetcher(t *testing.T) {
	spec.Run(t, "BuildpackFetcher", testBuildpackFetcher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildpackFetcher(t *testing.T, when spec.G, it spec.S) {
	when("#FetchBuildpack", func() {
		var (
			buildpackTgz   string
			mockController *gomock.Controller
			mockDownloader *mocks.MockDownloader
			subject        *buildpack.Fetcher
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockDownloader = mocks.NewMockDownloader(mockController)

			subject = buildpack.NewFetcher(mockDownloader)

			buildpackTgz = h.CreateTgz(t, filepath.Join("testdata", "buildpack"), "./", 0644)
		})

		it.After(func() {
			mockController.Finish()
			h.AssertNil(t, os.Remove(buildpackTgz))
		})

		it("fetches a buildpack from a directory", func() {
			downloadPath := filepath.Join("testdata", "buildpack")
			mockDownloader.EXPECT().
				Download(downloadPath).
				Return(downloadPath, nil)

			out, err := subject.FetchBuildpack(downloadPath)
			h.AssertNil(t, err)
			h.AssertEq(t, out.ID, "bp.one")
			h.AssertEq(t, out.Version, "some-buildpack-version")
			h.AssertEq(t, out.Stacks[0].ID, "some.stack.id")
			h.AssertEq(t, out.Stacks[1].ID, "other.stack.id")
			h.AssertNotEq(t, out.Path, "")
			h.AssertDirContainsFileWithContents(t, out.Path, "bin/detect", "detect")
			h.AssertDirContainsFileWithContents(t, out.Path, "bin/build", "build")
		})

		it("fetches a buildpack from a tgz", func() {
			downloadPath := filepath.Join("testdata", "buildpack.tgz")

			mockDownloader.EXPECT().
				Download(downloadPath).
				Return(buildpackTgz, nil)

			out, err := subject.FetchBuildpack(downloadPath)
			h.AssertNil(t, err)
			h.AssertEq(t, out.ID, "bp.one")
			h.AssertEq(t, out.Version, "some-buildpack-version")
			h.AssertEq(t, out.Stacks[0].ID, "some.stack.id")
			h.AssertEq(t, out.Stacks[1].ID, "other.stack.id")
			h.AssertNotEq(t, out.Path, "")
			h.AssertOnTarEntry(t, out.Path, "bin/detect", h.ContentEquals("detect"))
			h.AssertOnTarEntry(t, out.Path, "bin/build", h.ContentEquals("build"))
		})
	})
}
