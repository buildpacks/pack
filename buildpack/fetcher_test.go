package buildpack_test

import (
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
			mockController *gomock.Controller
			mockDownloader *mocks.MockDownloader
			subject        *buildpack.Fetcher
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockDownloader = mocks.NewMockDownloader(mockController)

			subject = buildpack.NewFetcher(mockDownloader)
		})

		it.After(func() {
			mockController.Finish()
		})

		it("fetches a buildpack", func() {
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
			h.AssertNotEq(t, out.Dir, "")
			h.AssertDirContainsFileWithContents(t, out.Dir, "bin/detect", "I come from a directory\n")
			h.AssertDirContainsFileWithContents(t, out.Dir, "bin/build", "I come from a directory\n")
		})
	})
}
