package lifecycle_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver"
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
		lifecycleTgz   string
		mockController *gomock.Controller
		mockDownloader *mocks.MockDownloader
		subject        *lifecycle.Fetcher
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockDownloader = mocks.NewMockDownloader(mockController)
		subject = lifecycle.NewFetcher(mockDownloader)
		lifecycleTgz = h.CreateTgz(t, filepath.Join("testdata", "lifecycle"), "./lifecycle", 0755)
	})

	it.After(func() {
		mockController.Finish()
		h.AssertNil(t, os.Remove(lifecycleTgz))
	})

	when("#Fetch", func() {
		when("only a version is provided", func() {
			it("returns a release from github", func() {
				mockDownloader.EXPECT().
					Download("https://github.com/buildpack/lifecycle/releases/download/v1.2.3/lifecycle-v1.2.3+linux.x86-64.tgz").
					Return(lifecycleTgz, nil)

				md, err := subject.Fetch(semver.MustParse("1.2.3"), "")
				h.AssertNil(t, err)
				h.AssertEq(t, md.Version.String(), "1.2.3")
				h.AssertEq(t, md.Path, lifecycleTgz)
			})
		})

		when("only a uri is provided", func() {
			it("returns the lifecycle from the uri", func() {
				mockDownloader.EXPECT().
					Download("https://lifecycle.example.com").
					Return(lifecycleTgz, nil)

				md, err := subject.Fetch(nil, "https://lifecycle.example.com")
				h.AssertNil(t, err)
				h.AssertNil(t, md.Version)
				h.AssertEq(t, md.Path, lifecycleTgz)
			})
		})

		when("a uri and version are provided", func() {
			it("returns the lifecycle from the uri", func() {
				mockDownloader.EXPECT().
					Download("https://lifecycle.example.com").
					Return(lifecycleTgz, nil)

				md, err := subject.Fetch(semver.MustParse("1.2.3"), "https://lifecycle.example.com")
				h.AssertNil(t, err)
				h.AssertEq(t, md.Version.String(), "1.2.3")
				h.AssertEq(t, md.Path, lifecycleTgz)
			})
		})

		when("neither is uri nor version is provided", func() {
			it("returns the default lifecycle", func() {
				mockDownloader.EXPECT().
					Download(fmt.Sprintf(
						"https://github.com/buildpack/lifecycle/releases/download/v%s/lifecycle-v%s+linux.x86-64.tgz",
						lifecycle.DefaultLifecycleVersion,
						lifecycle.DefaultLifecycleVersion,
					)).
					Return(lifecycleTgz, nil)

				md, err := subject.Fetch(nil, "")
				h.AssertNil(t, err)
				h.AssertEq(t, md.Version.String(), lifecycle.DefaultLifecycleVersion)
				h.AssertEq(t, md.Path, lifecycleTgz)
			})
		})

		when("the lifecycle is missing binaries", func() {
			it("returns an error", func() {
				tmp, err := ioutil.TempDir("", "")
				h.AssertNil(t, err)
				defer os.RemoveAll(tmp)

				mockDownloader.EXPECT().
					Download(fmt.Sprintf(
						"https://github.com/buildpack/lifecycle/releases/download/v%s/lifecycle-v%s+linux.x86-64.tgz",
						lifecycle.DefaultLifecycleVersion,
						lifecycle.DefaultLifecycleVersion,
					)).
					Return(tmp, nil)

				_, err = subject.Fetch(nil, "")
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
					Download(fmt.Sprintf(
						"https://github.com/buildpack/lifecycle/releases/download/v%s/lifecycle-v%s+linux.x86-64.tgz",
						lifecycle.DefaultLifecycleVersion,
						lifecycle.DefaultLifecycleVersion,
					)).
					Return(tmp, nil)

				_, err = subject.Fetch(nil, "")
				h.AssertError(t, err, "invalid lifecycle")
			})
		})
	})
}
