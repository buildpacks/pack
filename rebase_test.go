package pack_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/buildpack/lifecycle/image/fakes"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestRebase(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "rebase_factory", testRebase, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRebase(t *testing.T, when spec.G, it spec.S) {
	when("#Rebase", func() {
		var (
			fakeImageFetcher   *h.FakeImageFetcher
			client             *pack.Client
			cfg                *config.Config
			outBuf             bytes.Buffer
			errBuff            bytes.Buffer
			fakeAppImage       *fakes.Image
			fakeRunImage       *fakes.Image
			fakeRunImageMirror *fakes.Image
		)
		it.Before(func() {
			fakeImageFetcher = h.NewFakeImageFetcher()

			fakeAppImage = fakes.NewImage(t, "some/app", "", "")
			h.AssertNil(t, fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
				`{"stack":{"runImage":{"image":"some/run", "mirrors":["example.com/some/run"]}}}`))
			fakeImageFetcher.LocalImages["some/app"] = fakeAppImage

			fakeRunImage = fakes.NewImage(t, "some/run", "run-image-top-layer-sha", "run-image-digest")
			fakeImageFetcher.LocalImages["some/run"] = fakeRunImage

			fakeRunImageMirror = fakes.NewImage(t, "example.com/some/run", "mirror-top-layer-sha", "mirror-digest")
			fakeImageFetcher.LocalImages["example.com/some/run"] = fakeRunImageMirror

			cfg = &config.Config{}
			client = pack.NewClient(
				cfg,
				logging.NewLogger(&outBuf, &errBuff, false, false),
				fakeImageFetcher,
				nil,
				nil,
				nil,
			)
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
						fakeCustomRunImage = fakes.NewImage(t, "custom/run", "custom-base-top-layer-sha", "custom-base-digest")
						fakeImageFetcher.LocalImages["custom/run"] = fakeCustomRunImage
					})

					it.After(func() {
						fakeCustomRunImage.Cleanup()
					})

					it("uses the run image provided by the user", func() {
						h.AssertNil(t, client.Rebase(context.TODO(), pack.RebaseOptions{
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
						h.AssertNil(t, client.Rebase(context.TODO(), pack.RebaseOptions{
							RepoName: "some/app",
						}))
						h.AssertEq(t, fakeAppImage.Base(), "some/run")
						lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertContains(t, lbl, `"runImage":{"topLayer":"run-image-top-layer-sha","sha":"run-image-digest"`)
					})
				})

				when("the image has a label with a run image mirrors specified", func() {
					when("there are no locally-configured mirrors", func() {
						it.Before(func() {
							fakeImageFetcher.LocalImages["example.com/some/app"] = fakeAppImage
						})

						it("chooses a matching mirror from the app image label", func() {
							h.AssertNil(t, client.Rebase(context.TODO(), pack.RebaseOptions{
								RepoName: "example.com/some/app",
							}))
							h.AssertEq(t, fakeAppImage.Base(), "example.com/some/run")
							lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"mirror-top-layer-sha","sha":"mirror-digest"`)
						})
					})

					when("there are locally-configured mirrors", func() {
						var (
							fakeLocalMirror *fakes.Image
						)
						it.Before(func() {
							fakeImageFetcher.LocalImages["example.com/some/app"] = fakeAppImage
							fakeLocalMirror = fakes.NewImage(t, "example.com/some/local-run", "local-mirror-top-layer-sha", "local-mirror-digest")
							fakeImageFetcher.LocalImages[ "example.com/some/local-run"] = fakeLocalMirror
							cfg.RunImages = []config.RunImage{
								{
									Image:   "some/run",
									Mirrors: []string{"example.com/some/local-run"},
								},
							}
						})

						it.After(func() {
							fakeLocalMirror.Cleanup()
						})

						it("chooses a matching local mirror first", func() {
							h.AssertNil(t, client.Rebase(context.TODO(), pack.RebaseOptions{
								RepoName: "example.com/some/app",
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
						err := client.Rebase(context.TODO(), pack.RebaseOptions{
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
					fakeRemoteRunImage = fakes.NewImage(t, "some/run", "remote-top-layer-sha", "remote-digest")
					fakeImageFetcher.RemoteImages["some/run"] = fakeRemoteRunImage
				})

				it.After(func() {
					fakeRemoteRunImage.Cleanup()
				})

				when("is false", func() {
					when("skip pull is false", func() {
						it("updates the local image", func() {
							h.AssertNil(t, client.Rebase(context.TODO(), pack.RebaseOptions{
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
							h.AssertNil(t, client.Rebase(context.TODO(), pack.RebaseOptions{
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
							h.AssertNil(t, client.Rebase(context.TODO(), pack.RebaseOptions{
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
