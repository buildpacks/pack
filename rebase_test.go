package pack_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/buildpack/lifecycle/image/fakes"
	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestRebase(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "rebase_factory", testRebase, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRebase(t *testing.T, when spec.G, it spec.S) {
	when("#Rebase", func() {
		var (
			mockController   *gomock.Controller
			mockImageFetcher *mocks.MockImageFetcher
			mockBPFetcher    *mocks.MockBuildpackFetcher
			client           *pack.Client
			cfg              *config.Config
			outBuf           bytes.Buffer
			errBuff          bytes.Buffer
		)
		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImageFetcher = mocks.NewMockImageFetcher(mockController)
			mockBPFetcher = mocks.NewMockBuildpackFetcher(mockController)

			cfg = &config.Config{}
			client = pack.NewClient(
				cfg,
				logging.NewLogger(&outBuf, &errBuff, false, false),
				mockImageFetcher,
				nil,
				mockBPFetcher,
				nil,
			)
		})

		it.After(func() {
			mockController.Finish()
		})

		when("#Rebase", func() {
			when("run image is provided by the user", func() {
				when("the image has a label with a run image specified", func() {
					it("uses the run image provided by the user", func() {
						fakeNewBaseImage := fakes.NewImage(t, "some/run", "new-base-top-layer-sha", "new-base-digest")

						fakeAppImage := fakes.NewImage(t, "some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata", "{}")

						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/app", true, true).Return(fakeAppImage, nil)
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run", true, true).Return(fakeNewBaseImage, nil)

						opts := pack.RebaseOptions{
							RunImage: "some/run",
							RepoName: "some/app",
						}

						err := client.Rebase(context.TODO(), opts)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some/run")
						lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertContains(t, lbl, `"runImage":{"topLayer":"new-base-top-layer-sha","sha":"new-base-digest"`)
					})
				})
			})

			when("run image is NOT provided by the user", func() {
				when("the image has a label with a run image specified", func() {
					it("uses the run image provided in the App image label", func() {
						fakeNewBaseImage := fakes.NewImage(t, "some/run", "new-base-top-layer-sha", "new-base-digest")

						fakeAppImage := fakes.NewImage(t, "some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata", `{"stack":{"runImage":{"image":"some/run"}}}`)

						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/app", true, true).Return(fakeAppImage, nil)
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run", true, true).Return(fakeNewBaseImage, nil)

						opts := pack.RebaseOptions{
							RepoName: "some/app",
						}

						err := client.Rebase(context.TODO(), opts)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some/run")
						lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertContains(t, lbl, `"runImage":{"topLayer":"new-base-top-layer-sha","sha":"new-base-digest"`)
					})
				})

				when("the image has a label with a run image mirrors specified", func() {
					var fakeAppImage *fakes.Image

					it.Before(func() {
						fakeAppImage = fakes.NewImage(t, "example.com/some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
							`{"stack":{"runImage":{"image":"some/run", "mirrors":["example.com/some/run"]}}}`)
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "example.com/some/app", true, true).Return(fakeAppImage, nil)
					})

					when("there are no locally-configured mirrors", func() {
						it("chooses a matching mirror from the app image label", func() {
							fakeNewBaseImage := fakes.NewImage(t, "example.com/some/run", "new-base-top-layer-sha", "new-base-digest")
							mockImageFetcher.EXPECT().Fetch(gomock.Any(), "example.com/some/run", true, true).Return(fakeNewBaseImage, nil)

							opts := pack.RebaseOptions{
								RepoName: "example.com/some/app",
							}

							err := client.Rebase(context.TODO(), opts)
							h.AssertNil(t, err)
							h.AssertEq(t, fakeAppImage.Base(), "example.com/some/run")
							lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"new-base-top-layer-sha","sha":"new-base-digest"`)
						})
					})

					when("there are locally-configured mirrors", func() {
						it.Before(func() {
							cfg.RunImages = []config.RunImage{
								{
									Image:   "some/run",
									Mirrors: []string{"example.com/some/local-run"},
								},
							}
						})

						it("chooses a matching local mirror first", func() {
							fakeNewBaseImage := fakes.NewImage(t, "example.com/some/local-run", "new-base-top-layer-sha", "new-base-digest")
							mockImageFetcher.EXPECT().Fetch(gomock.Any(), "example.com/some/local-run", true, true).Return(fakeNewBaseImage, nil)
							opts := pack.RebaseOptions{
								RepoName: "example.com/some/app",
							}

							err := client.Rebase(context.TODO(), opts)
							h.AssertNil(t, err)
							h.AssertEq(t, fakeAppImage.Base(), "example.com/some/local-run")
							lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"new-base-top-layer-sha","sha":"new-base-digest"`)
						})
					})
				})

				when("the image does not have a label with a run image specified", func() {
					it("returns an error", func() {
						fakeAppImage := fakes.NewImage(t, "some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata", "{}")
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/app", true, true).Return(fakeAppImage, nil)

						opts := pack.RebaseOptions{
							RepoName: "some/app",
						}

						err := client.Rebase(context.TODO(), opts)
						h.AssertError(t, err, "run image must be specified")
					})
				})
			})

			when("publish is false", func() {
				when("skip pull is false", func() {
					it("updates the local image", func() {
						fakeNewBaseImage := fakes.NewImage(t, "some/run", "new-base-top-layer-sha", "new-base-digest")
						fakeAppImage := fakes.NewImage(t, "some/app", "", "")

						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
							`{"stack":{"runImage":{"image":"some/run"}}}`)

						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/app", true, true).Return(fakeAppImage, nil)
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run", true, true).Return(fakeNewBaseImage, nil)

						opts := pack.RebaseOptions{
							RepoName: "some/app",
							SkipPull: false,
						}

						err := client.Rebase(context.TODO(), opts)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some/run")
						lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertContains(t, lbl, `"runImage":{"topLayer":"new-base-top-layer-sha","sha":"new-base-digest"`)
					})
				})

				when("skip pull is true", func() {
					it("uses local image", func() {
						fakeNewBaseImage := fakes.NewImage(t, "some/run", "new-base-top-layer-sha", "new-base-digest")

						fakeAppImage := fakes.NewImage(t, "some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
							`{"stack":{"runImage":{"image":"some/run"}}}`)

						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/app", true, false).Return(fakeAppImage, nil)
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run", true, false).Return(fakeNewBaseImage, nil)

						opts := pack.RebaseOptions{
							RepoName: "some/app",
							SkipPull: true,
						}

						err := client.Rebase(context.TODO(), opts)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some/run")
						lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertContains(t, lbl, `"runImage":{"topLayer":"new-base-top-layer-sha","sha":"new-base-digest"`)
					})
				})
			})

			when("publish is true", func() {
				when("skip pull is anything", func() {
					it("uses remote image", func() {
						fakeNewBaseImage := fakes.NewImage(t, "some/run", "new-base-top-layer-sha", "new-base-digest")

						fakeAppImage := fakes.NewImage(t, "some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
							`{"stack":{"runImage":{"image":"some/run"}}}`)

						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/app", false, true).Return(fakeAppImage, nil)
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run", false, true).Return(fakeNewBaseImage, nil)

						opts := pack.RebaseOptions{
							RepoName: "some/app",
							Publish:  true,
						}

						err := client.Rebase(context.TODO(), opts)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some/run")
						lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertContains(t, lbl, `"runImage":{"topLayer":"new-base-top-layer-sha","sha":"new-base-digest"`)
					})
				})
			})
		})
	})
}
