package pack

import (
	"bytes"
	"context"
	"testing"

	"github.com/buildpacks/pack/config"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestRebase(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "rebase_factory", testRebase, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRebase(t *testing.T, when spec.G, it spec.S) {
	when("#Rebase", func() {
		var (
			fakeImageFetcher   *ifakes.FakeImageFetcher
			subject            *Client
			fakeAppImage       *fakes.Image
			fakeRunImage       *fakes.Image
			fakeRunImageMirror *fakes.Image
			out                bytes.Buffer
		)

		it.Before(func() {
			fakeImageFetcher = ifakes.NewFakeImageFetcher()

			fakeAppImage = fakes.NewImage("some/app", "", &fakeIdentifier{name: "app-image"})
			h.AssertNil(t, fakeAppImage.SetLabel("io.buildpacks.lifecycle.metadata",
				`{"stack":{"runImage":{"image":"some/run", "mirrors":["example.com/some/run"]}}}`))
			h.AssertNil(t, fakeAppImage.SetLabel("io.buildpacks.stack.id", "io.buildpacks.stacks.bionic"))
			fakeImageFetcher.LocalImages["some/app"] = fakeAppImage

			fakeRunImage = fakes.NewImage("some/run", "run-image-top-layer-sha", &fakeIdentifier{name: "run-image-digest"})
			h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "io.buildpacks.stacks.bionic"))
			fakeImageFetcher.LocalImages["some/run"] = fakeRunImage

			fakeRunImageMirror = fakes.NewImage("example.com/some/run", "mirror-top-layer-sha", &fakeIdentifier{name: "mirror-digest"})
			h.AssertNil(t, fakeRunImageMirror.SetLabel("io.buildpacks.stack.id", "io.buildpacks.stacks.bionic"))
			fakeImageFetcher.LocalImages["example.com/some/run"] = fakeRunImageMirror

			fakeLogger := logging.NewLogWithWriters(&out, &out)
			subject = &Client{
				logger:       fakeLogger,
				imageFetcher: fakeImageFetcher,
			}
		})

		it.After(func() {
			h.AssertNilE(t, fakeAppImage.Cleanup())
			h.AssertNilE(t, fakeRunImage.Cleanup())
			h.AssertNilE(t, fakeRunImageMirror.Cleanup())
		})

		when("#Rebase", func() {
			when("run image is provided by the user", func() {
				when("the image has a label with a run image specified", func() {
					var fakeCustomRunImage *fakes.Image

					it.Before(func() {
						fakeCustomRunImage = fakes.NewImage("custom/run", "custom-base-top-layer-sha", &fakeIdentifier{name: "custom-base-digest"})
						h.AssertNil(t, fakeCustomRunImage.SetLabel("io.buildpacks.stack.id", "io.buildpacks.stacks.bionic"))
						fakeImageFetcher.LocalImages["custom/run"] = fakeCustomRunImage
					})

					it.After(func() {
						h.AssertNilE(t, fakeCustomRunImage.Cleanup())
					})

					it("uses the run image provided by the user", func() {
						h.AssertNil(t, subject.Rebase(context.TODO(),
							RebaseOptions{
								RunImage: "custom/run",
								RepoName: "some/app",
							}))
						h.AssertEq(t, fakeAppImage.Base(), "custom/run")
						lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertContains(t, lbl, `"runImage":{"topLayer":"custom-base-top-layer-sha","reference":"custom-base-digest"`)
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
						h.AssertContains(t, lbl, `"runImage":{"topLayer":"run-image-top-layer-sha","reference":"run-image-digest"`)
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
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"mirror-top-layer-sha","reference":"mirror-digest"`)
						})
					})

					when("there are user provided mirrors", func() {
						var (
							fakeLocalMirror *fakes.Image
						)
						it.Before(func() {
							fakeImageFetcher.LocalImages["example.com/some/app"] = fakeAppImage
							fakeLocalMirror = fakes.NewImage("example.com/some/local-run", "local-mirror-top-layer-sha", &fakeIdentifier{name: "local-mirror-digest"})
							h.AssertNil(t, fakeLocalMirror.SetLabel("io.buildpacks.stack.id", "io.buildpacks.stacks.bionic"))
							fakeImageFetcher.LocalImages["example.com/some/local-run"] = fakeLocalMirror
						})

						it.After(func() {
							h.AssertNilE(t, fakeLocalMirror.Cleanup())
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
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"local-mirror-top-layer-sha","reference":"local-mirror-digest"`)
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
					fakeRemoteRunImage = fakes.NewImage("some/run", "remote-top-layer-sha", &fakeIdentifier{name: "remote-digest"})
					h.AssertNil(t, fakeRemoteRunImage.SetLabel("io.buildpacks.stack.id", "io.buildpacks.stacks.bionic"))
					fakeImageFetcher.RemoteImages["some/run"] = fakeRemoteRunImage
				})

				it.After(func() {
					h.AssertNilE(t, fakeRemoteRunImage.Cleanup())
				})

				when("is false", func() {
					when("pull policy is always", func() {
						it("updates the local image", func() {
							h.AssertNil(t, subject.Rebase(context.TODO(), RebaseOptions{
								RepoName:   "some/app",
								PullPolicy: config.PullAlways,
							}))
							h.AssertEq(t, fakeAppImage.Base(), "some/run")
							lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"remote-top-layer-sha","reference":"remote-digest"`)
						})
					})

					when("pull policy is never", func() {
						it("uses local image", func() {
							h.AssertNil(t, subject.Rebase(context.TODO(), RebaseOptions{
								RepoName:   "some/app",
								PullPolicy: config.PullNever,
							}))
							h.AssertEq(t, fakeAppImage.Base(), "some/run")
							lbl, _ := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"run-image-top-layer-sha","reference":"run-image-digest"`)
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
							h.AssertContains(t, lbl, `"runImage":{"topLayer":"remote-top-layer-sha","reference":"remote-digest"`)
						})
					})
				})
			})
		})
	})
}

type fakeIdentifier struct {
	name string
}

func (f *fakeIdentifier) String() string {
	return f.name
}
