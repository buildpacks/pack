package pack_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/fatih/color"

	"github.com/buildpack/pack/logging"

	"github.com/buildpack/lifecycle"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestRebaseFactory(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "rebase_factory", testRebaseFactory, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRebaseFactory(t *testing.T, when spec.G, it spec.S) {
	when("#RebaseFactory", func() {
		var (
			mockController *gomock.Controller
			mockFetcher    *mocks.MockFetcher
			factory        pack.RebaseFactory
			outBuf         bytes.Buffer
			errBuff        bytes.Buffer
		)
		it.Before(func() {
			mockController = gomock.NewController(t)
			mockFetcher = mocks.NewMockFetcher(mockController)

			factory = pack.RebaseFactory{
				Logger: logging.NewLogger(&outBuf, &errBuff, false, false),
				Config: &config.Config{
					DefaultStackID: "some.default.stack",
					Stacks: []config.Stack{
						{
							ID:         "some.default.stack",
							BuildImage: "default/build",
							RunImages:  []string{"default/run"},
						},
						{
							ID:         "some.other.stack",
							BuildImage: "other/build",
							RunImages:  []string{"other/run"},
						},
					},
				},
				Fetcher: mockFetcher,
			}
		})

		it.After(func() {
			mockController.Finish()
		})

		when("#RebaseConfigFromFlags", func() {
			when("run image is provided by the user", func() {
				when("the image has a label with a run image specified", func() {
					it("uses the run image provided by the user", func() {
						mockBaseImage := mocks.NewMockImage(mockController)
						mockImage := mocks.NewMockImage(mockController)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "myorg/myrepo", gomock.Any()).Return(mockImage, nil)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "my/run/image", gomock.Any()).Return(mockBaseImage, nil)

						flags := pack.RebaseFlags{
							RunImage: "my/run/image",
							RepoName: "myorg/myrepo",
						}

						_, err := factory.RebaseConfigFromFlags(context.TODO(), flags)
						h.AssertNil(t, err)
					})
				})
				when("the image does not have a label with a run image specified", func() {
					it("uses the run image provided by the user", func() {
						mockBaseImage := mocks.NewMockImage(mockController)
						mockImage := mocks.NewMockImage(mockController)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "myorg/myrepo", gomock.Any()).Return(mockImage, nil)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "my/run/image", gomock.Any()).Return(mockBaseImage, nil)

						flags := pack.RebaseFlags{
							RunImage: "my/run/image",
							RepoName: "myorg/myrepo",
						}

						_, err := factory.RebaseConfigFromFlags(context.TODO(), flags)
						h.AssertNil(t, err)
					})
				})
			})
			when("run image is NOT provided by the user", func() {
				when("the image has a label with a run image specified", func() {
					it("uses the run image provided in the App image label", func() {
						mockBaseImage := mocks.NewMockImage(mockController)
						mockImage := mocks.NewMockImage(mockController)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "myorg/myrepo", gomock.Any()).Return(mockImage, nil)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/other/runimage", gomock.Any()).Return(mockBaseImage, nil)
						mockImage.EXPECT().Label("io.buildpacks.run-image").Return("some/other/runimage", nil)

						flags := pack.RebaseFlags{
							RepoName: "myorg/myrepo",
						}

						_, err := factory.RebaseConfigFromFlags(context.TODO(), flags)
						h.AssertNil(t, err)
					})
				})
				when("the image does not have a label with a run image specified", func() {
					it("returns an error", func() {
						mockImage := mocks.NewMockImage(mockController)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "myorg/myrepo", gomock.Any()).Return(mockImage, nil)
						mockImage.EXPECT().Label("io.buildpacks.run-image").Return("", nil)

						flags := pack.RebaseFlags{
							RepoName: "myorg/myrepo",
						}

						_, err := factory.RebaseConfigFromFlags(context.TODO(), flags)
						h.AssertError(t, err, "run image must be specified")
					})
				})
			})

			when("publish is false", func() {
				when("no-pull is false", func() {
					it("XXXX", func() {
						mockBaseImage := mocks.NewMockImage(mockController)
						mockImage := mocks.NewMockImage(mockController)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "default/run", gomock.Any()).Return(mockBaseImage, nil)
						mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "myorg/myrepo", gomock.Any()).Return(mockImage, nil)

						cfg, err := factory.RebaseConfigFromFlags(context.TODO(), pack.RebaseFlags{
							RepoName: "myorg/myrepo",
							RunImage: "default/run",
							Publish:  false,
							NoPull:   false,
						})
						h.AssertNil(t, err)

						h.AssertSameInstance(t, cfg.Image, mockImage)
						h.AssertSameInstance(t, cfg.NewBaseImage, mockBaseImage)
					})
				})

				when("no-pull is true", func() {
					it("XXXX", func() {
						mockBaseImage := mocks.NewMockImage(mockController)
						mockImage := mocks.NewMockImage(mockController)
						mockFetcher.EXPECT().FetchLocalImage("default/run").Return(mockBaseImage, nil)
						mockFetcher.EXPECT().FetchLocalImage("myorg/myrepo").Return(mockImage, nil)

						cfg, err := factory.RebaseConfigFromFlags(context.TODO(), pack.RebaseFlags{
							RepoName: "myorg/myrepo",
							RunImage: "default/run",
							Publish:  false,
							NoPull:   true,
						})
						h.AssertNil(t, err)

						h.AssertSameInstance(t, cfg.Image, mockImage)
						h.AssertSameInstance(t, cfg.NewBaseImage, mockBaseImage)
					})
				})
			})

			when("publish is true", func() {
				when("no-pull is anything", func() {
					it("XXXX", func() {
						mockBaseImage := mocks.NewMockImage(mockController)
						mockImage := mocks.NewMockImage(mockController)
						mockFetcher.EXPECT().FetchRemoteImage("default/run").Return(mockBaseImage, nil)
						mockFetcher.EXPECT().FetchRemoteImage("myorg/myrepo").Return(mockImage, nil)

						cfg, err := factory.RebaseConfigFromFlags(context.TODO(), pack.RebaseFlags{
							RepoName: "myorg/myrepo",
							RunImage: "default/run",
							Publish:  true,
							NoPull:   false,
						})
						h.AssertNil(t, err)

						h.AssertSameInstance(t, cfg.Image, mockImage)
						h.AssertSameInstance(t, cfg.NewBaseImage, mockBaseImage)
					})
				})
			})
		})

		when("#Rebase", func() {
			it("swaps the old base for the new base AND stores new sha for new runimage", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockBaseImage.EXPECT().TopLayer().Return("some-top-layer", nil)
				mockBaseImage.EXPECT().Digest().Return("some-sha", nil)
				mockImage := mocks.NewMockImage(mockController)
				mockImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
					Return(`{"runimage":{"topLayer":"old-top-layer"}, "app":{"sha":"data"}}`, nil)
				mockImage.EXPECT().Rebase("old-top-layer", mockBaseImage)
				setLabel := mockImage.EXPECT().SetLabel("io.buildpacks.lifecycle.metadata", gomock.Any()).
					Do(func(_, label string) {
						var metadata lifecycle.AppImageMetadata
						h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
						h.AssertEq(t, metadata.RunImage.TopLayer, "some-top-layer")
						h.AssertEq(t, metadata.RunImage.SHA, "some-sha")
						h.AssertEq(t, metadata.App.SHA, "data")
					})
				mockImage.EXPECT().Save().After(setLabel).Return("some-digest", nil)

				rebaseConfig := pack.RebaseConfig{
					Image:        mockImage,
					NewBaseImage: mockBaseImage,
				}
				err := factory.Rebase(rebaseConfig)
				h.AssertNil(t, err)
			})
		})
	})
}
