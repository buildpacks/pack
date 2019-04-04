package pack_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/buildpack/lifecycle/testhelpers"
	"github.com/fatih/color"

	"github.com/buildpack/pack/config"

	"github.com/buildpack/pack/logging"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
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
			mockController *gomock.Controller
			mockFetcher    *mocks.MockFetcher
			client         *pack.Client
			cfg            *config.Config
			outBuf         bytes.Buffer
			errBuff        bytes.Buffer
		)
		it.Before(func() {
			mockController = gomock.NewController(t)
			mockFetcher = mocks.NewMockFetcher(mockController)
			cfg = &config.Config{}
			client = pack.NewClient(
				cfg,
				logging.NewLogger(&outBuf, &errBuff, false, false),
				mockFetcher,
			)
		})

		it.After(func() {
			mockController.Finish()
		})

		when("#Rebase", func() {
			when("run image is provided by the user", func() {
				when("the image has a label with a run image specified", func() {
					it("uses the run image provided by the user", func() {
						fakeNewBaseImage := testhelpers.NewFakeImage(t, "some/run", "new-base-top-layer-sha", "new-base-digest")

						fakeAppImage := testhelpers.NewFakeImage(t, "some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata", "{}")

						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/app", gomock.Any()).Return(fakeAppImage, nil)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/run", gomock.Any()).Return(fakeNewBaseImage, nil)

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
						fakeNewBaseImage := testhelpers.NewFakeImage(t, "some/run", "new-base-top-layer-sha", "new-base-digest")

						fakeAppImage := testhelpers.NewFakeImage(t, "some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata", `{"stack":{"runImage":{"image":"some/run"}}}`)

						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/app", gomock.Any()).Return(fakeAppImage, nil)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/run", gomock.Any()).Return(fakeNewBaseImage, nil)

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
					var fakeAppImage *testhelpers.FakeImage

					it.Before(func() {
						fakeAppImage = testhelpers.NewFakeImage(t, "example.com/some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
							`{"stack":{"runImage":{"image":"some/run", "mirrors":["example.com/some/run"]}}}`)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "example.com/some/app", gomock.Any()).Return(fakeAppImage, nil)
					})

					when("there are no locally-configured mirrors", func() {
						it("chooses a matching mirror from the app image label", func() {
							fakeNewBaseImage := testhelpers.NewFakeImage(t, "example.com/some/run", "new-base-top-layer-sha", "new-base-digest")
							mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "example.com/some/run", gomock.Any()).Return(fakeNewBaseImage, nil)

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
							fakeNewBaseImage := testhelpers.NewFakeImage(t, "example.com/some/local-run", "new-base-top-layer-sha", "new-base-digest")
							mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "example.com/some/local-run", gomock.Any()).Return(fakeNewBaseImage, nil)

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
						mockImage := mocks.NewMockImage(mockController)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/app", gomock.Any()).Return(mockImage, nil)
						mockImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").Return(`{}`, nil)

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
						fakeNewBaseImage := testhelpers.NewFakeImage(t, "some/run", "new-base-top-layer-sha", "new-base-digest")

						fakeAppImage := testhelpers.NewFakeImage(t, "some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
							`{"stack":{"runImage":{"image":"some/run"}}}`)

						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/app", gomock.Any()).Return(fakeAppImage, nil)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/run", gomock.Any()).Return(fakeNewBaseImage, nil)

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
						fakeNewBaseImage := testhelpers.NewFakeImage(t, "some/run", "new-base-top-layer-sha", "new-base-digest")

						fakeAppImage := testhelpers.NewFakeImage(t, "some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
							`{"stack":{"runImage":{"image":"some/run"}}}`)

						mockFetcher.EXPECT().FetchLocalImage("some/app").Return(fakeAppImage, nil)
						mockFetcher.EXPECT().FetchLocalImage("some/run").Return(fakeNewBaseImage, nil)

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
						fakeNewBaseImage := testhelpers.NewFakeImage(t, "some/run", "new-base-top-layer-sha", "new-base-digest")

						fakeAppImage := testhelpers.NewFakeImage(t, "some/app", "", "")
						fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
							`{"stack":{"runImage":{"image":"some/run"}}}`)

						mockFetcher.EXPECT().FetchRemoteImage("some/app").Return(fakeAppImage, nil)
						mockFetcher.EXPECT().FetchRemoteImage("some/run").Return(fakeNewBaseImage, nil)

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
