package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/lifecycle"
	"github.com/buildpack/pack/lifecycle/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestFetcher(t *testing.T) {
	spec.Run(t, "Fetcher", testFetcher, spec.Report(report.Terminal{}))
}

func testFetcher(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController *gomock.Controller
		mockDownloader *mocks.MockDownloader
		subject        *lifecycle.Fetcher
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockDownloader = mocks.NewMockDownloader(mockController)
		subject = lifecycle.NewFetcher(mockDownloader)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#Fetch", func() {
		when("only a version is provided", func() {
			it("returns a release from github", func() {
				mockDownloader.EXPECT().
					Download("https://github.com/buildpack/lifecycle/releases/download/v1.2.3/lifecycle-v1.2.3+linux.x86-64.tgz").
					Return(filepath.Join("testdata", "download-dir"), nil)

				md, err := subject.Fetch("1.2.3", "")
				h.AssertNil(t, err)
				h.AssertEq(t, md.Version, "1.2.3")
				h.AssertEq(t, md.Dir, filepath.Join("testdata", "download-dir", "fake-lifecycle"))
			})
		})

		when("only a uri is provided", func() {
			it("returns the lifecycle from the uri", func() {
				mockDownloader.EXPECT().
					Download("https://lifecycle.example.com").
					Return(filepath.Join("testdata", "download-dir"), nil)

				md, err := subject.Fetch("", "https://lifecycle.example.com")
				h.AssertNil(t, err)
				h.AssertEq(t, md.Version, "")
				h.AssertEq(t, md.Dir, filepath.Join("testdata", "download-dir", "fake-lifecycle"))
			})
		})

		when("a uri and version are provided", func() {
			it("returns the lifecycle from the uri", func() {
				mockDownloader.EXPECT().
					Download("https://lifecycle.example.com").
					Return(filepath.Join("testdata", "download-dir"), nil)

				md, err := subject.Fetch("1.2.3", "https://lifecycle.example.com")
				h.AssertNil(t, err)
				h.AssertEq(t, md.Version, "1.2.3")
				h.AssertEq(t, md.Dir, filepath.Join("testdata", "download-dir", "fake-lifecycle"))
			})
		})

		when("neither is uri nor version is provided", func() {
			it("returns the default lifecycle", func() {
				mockDownloader.EXPECT().
					Download("https://github.com/buildpack/lifecycle/releases/download/v0.1.0/lifecycle-v0.1.0+linux.x86-64.tgz").
					Return(filepath.Join("testdata", "download-dir"), nil)

				md, err := subject.Fetch("", "")
				h.AssertNil(t, err)
				h.AssertEq(t, md.Version, "0.1.0")
				h.AssertEq(t, md.Dir, filepath.Join("testdata", "download-dir", "fake-lifecycle"))
			})
		})

		when("the lifecycle is missing binaries", func() {
			it("returns an error", func() {
				tmp, err := ioutil.TempDir("", "")
				h.AssertNil(t, err)
				defer os.RemoveAll(tmp)

				mockDownloader.EXPECT().
					Download("https://github.com/buildpack/lifecycle/releases/download/v0.1.0/lifecycle-v0.1.0+linux.x86-64.tgz").
					Return(tmp, nil)

				_, err = subject.Fetch("", "")
				h.AssertError(t, err, "invalid lifecycle")
			})
		})

		when("the lifecycle has incomplete list of binaries", func() {
			it("returns an error", func() {
				tmp, err := ioutil.TempDir("", "")
				h.AssertNil(t, err)
				defer os.RemoveAll(tmp)

				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmp, "analyzer"), []byte("content"), os.ModePerm))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmp, "detector"), []byte("content"), os.ModePerm))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmp, "builder"), []byte("content"), os.ModePerm))

				mockDownloader.EXPECT().
					Download("https://github.com/buildpack/lifecycle/releases/download/v0.1.0/lifecycle-v0.1.0+linux.x86-64.tgz").
					Return(tmp, nil)

				_, err = subject.Fetch("", "")
				h.AssertError(t, err, "invalid lifecycle")
			})
		})
	})
}
