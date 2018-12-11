package pack_test

import (
	"bytes"
	"encoding/json"
	"log"
	"testing"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestRebase(t *testing.T) {
	spec.Run(t, "rebase", testRebase, spec.Parallel(), spec.Report(report.Terminal{}))
}

//move somewhere else
//go:generate mockgen -package mocks -destination mocks/image.go github.com/buildpack/lifecycle/image Image

func testRebase(t *testing.T, when spec.G, it spec.S) {
	when("#RebaseFactory", func() {
		var (
			mockController   *gomock.Controller
			mockImageFactory *mocks.MockImageFactory
			factory          pack.RebaseFactory
			buf              bytes.Buffer
		)
		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImageFactory = mocks.NewMockImageFactory(mockController)

			factory = pack.RebaseFactory{
				Log: log.New(&buf, "", log.LstdFlags),
				Config: &config.Config{
					DefaultStackID: "some.default.stack",
					Stacks: []config.Stack{
						{
							ID:          "some.default.stack",
							BuildImages: []string{"default/build", "registry.com/build/image"},
							RunImages:   []string{"default/run"},
						},
						{
							ID:          "some.other.stack",
							BuildImages: []string{"other/build"},
							RunImages:   []string{"other/run"},
						},
					},
				},
				ImageFactory: mockImageFactory,
			}
		})

		it.After(func() {
			mockController.Finish()
		})

		when("#RebaseConfigFromFlags", func() {
			when("publish is false", func() {
				when("no-pull is false", func() {
					it("XXXX", func() {
						mockBaseImage := mocks.NewMockImage(mockController)
						mockImage := mocks.NewMockImage(mockController)
						mockImageFactory.EXPECT().NewLocal("default/run", true).Return(mockBaseImage, nil)
						mockImageFactory.EXPECT().NewLocal("myorg/myrepo", true).Return(mockImage, nil)
						mockImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.default.stack", nil)

						cfg, err := factory.RebaseConfigFromFlags(pack.RebaseFlags{
							RepoName: "myorg/myrepo",
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
						mockImageFactory.EXPECT().NewLocal("default/run", false).Return(mockBaseImage, nil)
						mockImageFactory.EXPECT().NewLocal("myorg/myrepo", false).Return(mockImage, nil)
						mockImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.default.stack", nil)

						cfg, err := factory.RebaseConfigFromFlags(pack.RebaseFlags{
							RepoName: "myorg/myrepo",
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
						mockImageFactory.EXPECT().NewRemote("default/run").Return(mockBaseImage, nil)
						mockImageFactory.EXPECT().NewRemote("myorg/myrepo").Return(mockImage, nil)
						mockImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.default.stack", nil)

						cfg, err := factory.RebaseConfigFromFlags(pack.RebaseFlags{
							RepoName: "myorg/myrepo",
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
				mockImage.EXPECT().Name().Return("my-org/my-repo")
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
				h.AssertContains(t, buf.String(), "Successfully replaced my-org/my-repo with some-digest\n")
			})
		})
	})
}
