package pack

import (
	"bytes"
	"context"
	"testing"

	"github.com/buildpack/imgutil/fakes"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/internal/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestRebase(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "rebase_factory", testRebase, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRebase(t *testing.T, when spec.G, it spec.S) {
	when("#Rebase", func() {
		var (
			fakeImageFetcher   *mocks.FakeImageFetcher
			subject            *Client
			fakeAppImage       *fakes.Image
			fakeRunImage       *fakes.Image
			fakeRunImageMirror *fakes.Image
			out                bytes.Buffer
		)
		it.Before(func() {
			fakeImageFetcher = mocks.NewFakeImageFetcher()

			fakeAppImage = fakes.NewImage("some/app", "", "")
			h.AssertNil(t, fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
				`{"stack":{"runImage":{"image":"some/run", "mirrors":["example.com/some/run"]}}}`))
			fakeImageFetcher.LocalImages["some/app"] = fakeAppImage

			fakeRunImage = fakes.NewImage("some/run", "run-image-top-layer-sha", "run-image-digest")
			fakeImageFetcher.LocalImages["some/run"] = fakeRunImage

			fakeRunImageMirror = fakes.NewImage("example.com/some/run", "mirror-top-layer-sha", "mirror-digest")
			fakeImageFetcher.LocalImages["example.com/some/run"] = fakeRunImageMirror

			subject = &Client{
				logger:       mocks.NewMockLogger(&out),
				imageFetcher: fakeImageFetcher,
			}
		})

		it.After(func() {
			fakeAppImage.Cleanup()
			fakeRunImage.Cleanup()
			fakeRunImageMirror.Cleanup()
		})

		when("#Rebase", func() {
			when("run image is provided by the user", func() {
				when("the image has a label with a run image specified", func() {
					var fakeCustomRunImage *fakes.Image

					it.Before(func() {
						fakeCustomRunImage = fakes.NewImage("custom/run", "custom-base-top-layer-sha", "custom-base-digest")
						fakeImageFetcher.LocalImages["custom/run"] = fakeCustomRunImage
					})

					it.After(func() {
						fakeCustomRunImage.Cleanup()
					})

					it("uses the run image provided by the user", func() {
						h.AssertNil(t, subject.Rebase(context.TODO(), RebaseOptions{
							RunImage: "custom/run",
							RepoName: "some/app",
						}))
						h.AssertEq(t, fakeAppImage.Base(), "custom/run")
						lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertContains(t, lbl, `"runImage":{"topLayer":"custom-base-top-layer-sha","sha":"custom-base-digest"`)
					})
				})
			})

			when("run image is NOT provided by the user", func() {
				when("the image has a label with a run image specified", func() {
					it("uses the run image provided in the App image label", func() {
						h.AssertNil(t, subject.Rebase(context.TODO(), RebaseOptions{
							RepoName: "some/app",
						}))
						h.AssertEq(t, fakeAppImage.Base(), "some/run")
						lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertContains(t, lbl, `"runImage":{"topLayer":"run-image-top-layer-sha","sha":"run-image-digest"`)
					})
				})

				when("the image has a label with a run image mirrors specified", func() {
					when("there are no user provided mirrors", func() {
						it.Before(func() {
							fakeImageFetcher.LocalImages["example.com/some/app"] = fakeAppImage
						})

						it("chooses a matching mirror from the app image label", func() {
							h.AssertNil(t, subject.Rebase(context.TODO(), RebaseOptions{
								RepoName: "example.com/some/app",
							}))
							h.AssertEq(t, fakeAppImage.Base(), "example.com/some/run")
							lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"mirror-top-layer-sha","sha":"mirror-digest"`)
						})
					})

					when("there are user provided mirrors", func() {
						var (
							fakeLocalMirror *fakes.Image
						)
						it.Before(func() {
							fakeImageFetcher.LocalImages["example.com/some/app"] = fakeAppImage
							fakeLocalMirror = fakes.NewImage("example.com/some/local-run", "local-mirror-top-layer-sha", "local-mirror-digest")
							fakeImageFetcher.LocalImages["example.com/some/local-run"] = fakeLocalMirror
						})

						it.After(func() {
							fakeLocalMirror.Cleanup()
						})

						it("chooses a matching local mirror first", func() {
							h.AssertNil(t, subject.Rebase(context.TODO(), RebaseOptions{
								RepoName: "example.com/some/app",
								AdditionalMirrors: map[string][]string{
									"some/run": {"example.com/some/local-run"},
								},
							}))
							h.AssertEq(t, fakeAppImage.Base(), "example.com/some/local-run")
							lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"local-mirror-top-layer-sha","sha":"local-mirror-digest"`)
						})
					})
				})

				when("the image does not have a label with a run image specified", func() {
					it("returns an error", func() {
						h.AssertNil(t, fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata", "{}"))
						err := subject.Rebase(context.TODO(), RebaseOptions{
							RepoName: "some/app",
						})
						h.AssertError(t, err, "run image must be specified")
					})
				})
			})

			when("publish", func() {
				var (
					fakeRemoteRunImage *fakes.Image
				)

				it.Before(func() {
					fakeRemoteRunImage = fakes.NewImage("some/run", "remote-top-layer-sha", "remote-digest")
					fakeImageFetcher.RemoteImages["some/run"] = fakeRemoteRunImage
				})

				it.After(func() {
					fakeRemoteRunImage.Cleanup()
				})

				when("is false", func() {
					when("skip pull is false", func() {
						it("updates the local image", func() {
							h.AssertNil(t, subject.Rebase(context.TODO(), RebaseOptions{
								RepoName: "some/app",
								SkipPull: false,
							}))
							h.AssertEq(t, fakeAppImage.Base(), "some/run")
							lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"remote-top-layer-sha","sha":"remote-digest"`)
						})
					})

					when("skip pull is true", func() {
						it("uses local image", func() {
							h.AssertNil(t, subject.Rebase(context.TODO(), RebaseOptions{
								RepoName: "some/app",
								SkipPull: true,
							}))
							h.AssertEq(t, fakeAppImage.Base(), "some/run")
							lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"run-image-top-layer-sha","sha":"run-image-digest"`)
						})
					})
				})

				when("is true", func() {
					it.Before(func() {
						fakeImageFetcher.RemoteImages["some/app"] = fakeAppImage
					})

					when("skip pull is anything", func() {
						it("uses remote image", func() {
							h.AssertNil(t, subject.Rebase(context.TODO(), RebaseOptions{
								RepoName: "some/app",
								Publish:  true,
							}))
							h.AssertEq(t, fakeAppImage.Base(), "some/run")
							lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"remote-top-layer-sha","sha":"remote-digest"`)
						})
					})
				})
			})
		})
	})
}
