package pack_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"testing"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/mocks"
	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/golang/mock/gomock"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestRebase(t *testing.T) {
	spec.Run(t, "rebase", testRebase, spec.Parallel(), spec.Report(report.Terminal{}))
}

//go:generate mockgen -package mocks -destination mocks/writablestore.go github.com/buildpack/pack WritableStore
//go:generate mockgen -package mocks -destination mocks/layer.go github.com/google/go-containerregistry/pkg/v1 Layer

func testRebase(t *testing.T, when spec.G, it spec.S) {
	when("#RebaseFactory", func() {
		var (
			mockController *gomock.Controller
			mockDocker     *mocks.MockDocker
			mockImages     *mocks.MockImages
			mockRepoStore  *mocks.MockStore
			mockRepoImage  *mocks.MockImage
			mockBaseImage  *mocks.MockImage
			factory        pack.RebaseFactory
		)
		it.Before(func() {
			mockController = gomock.NewController(t)
			mockDocker = mocks.NewMockDocker(mockController)
			mockImages = mocks.NewMockImages(mockController)
			mockRepoStore = mocks.NewMockStore(mockController)
			mockRepoImage = mocks.NewMockImage(mockController)
			mockBaseImage = mocks.NewMockImage(mockController)

			factory = pack.RebaseFactory{
				Docker: mockDocker,
				Log:    log.New(ioutil.Discard, "", log.LstdFlags),
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
				Images: mockImages,
			}
		})

		it.After(func() {
			mockController.Finish()
		})

		when("#RebaseConfigFromFlags", func() {
			var layer1, layer2, layer3 *mocks.MockLayer
			it.Before(func() {
				layer1 = mocks.NewMockLayer(mockController)
				layer1.EXPECT().DiffID().Return(v1.Hash{Algorithm: "sha256", Hex: "12345"}, nil).AnyTimes()
				layer2 = mocks.NewMockLayer(mockController)
				layer2.EXPECT().DiffID().Return(v1.Hash{Algorithm: "sha256", Hex: "abcdef"}, nil).AnyTimes()
				layer3 = mocks.NewMockLayer(mockController)
				mockRepoImage.EXPECT().Layers().Return([]v1.Layer{layer1, layer2, layer3}, nil).AnyTimes()
			})

			when("publish is false", func() {
				it.Before(func() {
					mockImages.EXPECT().ReadImage("default/build", true).Return(mockBaseImage, nil)
					mockImages.EXPECT().RepoStore("myorg/myrepo", true).Return(mockRepoStore, nil)
					mockImages.EXPECT().ReadImage("myorg/myrepo", true).Return(mockRepoImage, nil)

					mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "myorg/myrepo").Return(dockertypes.ImageInspect{
						Config: &dockercontainer.Config{
							Labels: map[string]string{
								"io.buildpacks.stack.id":           "some.default.stack",
								"io.buildpacks.lifecycle.metadata": `{"runimage":{"sha":"sha256:abcdef"}}`,
							},
						},
					}, nil, nil).AnyTimes()
				})

				when("no-pull is false", func() {
					it("XXXX", func() {
						mockDocker.EXPECT().PullImage("default/build")
						mockDocker.EXPECT().PullImage("myorg/myrepo")

						cfg, err := factory.RebaseConfigFromFlags(pack.RebaseFlags{
							RepoName: "myorg/myrepo",
							Publish:  false,
							NoPull:   false,
						})
						assertNil(t, err)

						assertEq(t, cfg.RepoName, "myorg/myrepo")
						assertEq(t, cfg.Publish, false)
						assertSameInstance(t, cfg.Repo, mockRepoStore)
						assertSameInstance(t, cfg.RepoImage, mockRepoImage)
						assertLayers(t, cfg.OldBase, []v1.Layer{layer1, layer2})
						assertSameInstance(t, cfg.NewBase, mockBaseImage)
					})
				})

				when("no-pull is true", func() {
					it("XXXX", func() {
						cfg, err := factory.RebaseConfigFromFlags(pack.RebaseFlags{
							RepoName: "myorg/myrepo",
							Publish:  false,
							NoPull:   true,
						})
						assertNil(t, err)

						assertEq(t, cfg.RepoName, "myorg/myrepo")
						assertEq(t, cfg.Publish, false)
						assertSameInstance(t, cfg.Repo, mockRepoStore)
						assertSameInstance(t, cfg.RepoImage, mockRepoImage)
						assertLayers(t, cfg.OldBase, []v1.Layer{layer1, layer2})
						assertSameInstance(t, cfg.NewBase, mockBaseImage)
					})
				})
			})

			when("publish is true", func() {
				it.Before(func() {
					mockImages.EXPECT().ReadImage("default/build", false).Return(mockBaseImage, nil).AnyTimes()
					mockImages.EXPECT().RepoStore("myorg/myrepo", false).Return(mockRepoStore, nil).AnyTimes()
					mockImages.EXPECT().ReadImage("myorg/myrepo", false).Return(mockRepoImage, nil).AnyTimes()

					mockRepoImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{
						Config: v1.Config{
							Labels: map[string]string{
								"io.buildpacks.stack.id":           "some.default.stack",
								"io.buildpacks.lifecycle.metadata": `{"runimage":{"sha":"sha256:abcdef"}}`,
							},
						},
					}, nil).AnyTimes()
				})

				when("no-pull is anything", func() {
					it("XXXX", func() {
						cfg, err := factory.RebaseConfigFromFlags(pack.RebaseFlags{
							RepoName: "myorg/myrepo",
							Publish:  true,
							NoPull:   false,
						})
						assertNil(t, err)

						assertEq(t, cfg.RepoName, "myorg/myrepo")
						assertEq(t, cfg.Publish, true)
						assertSameInstance(t, cfg.Repo, mockRepoStore)
						assertSameInstance(t, cfg.RepoImage, mockRepoImage)
						assertLayers(t, cfg.OldBase, []v1.Layer{layer1, layer2})
						assertSameInstance(t, cfg.NewBase, mockBaseImage)
					})
				})
			})
		})

		when("#Rebase", func() {
			var oldBaseLayer1, oldBaseLayer2, newBaseLayer1, newBaseLayer2, appLayer1, appLayer2 *mocks.MockLayer
			var mockOldBaseImage, mockNewBaseImage *mocks.MockImage
			var rebaseConfig *pack.RebaseConfig
			var savedImage v1.Image
			it.Before(func() {
				oldBaseLayer1 = mocks.NewMockLayer(mockController)
				oldBaseLayer2 = mocks.NewMockLayer(mockController)
				newBaseLayer1 = mocks.NewMockLayer(mockController)
				newBaseLayer2 = mocks.NewMockLayer(mockController)
				appLayer1 = mocks.NewMockLayer(mockController)
				appLayer2 = mocks.NewMockLayer(mockController)
				for i, layer := range []*mocks.MockLayer{oldBaseLayer1, oldBaseLayer2, newBaseLayer1, newBaseLayer2, appLayer1, appLayer2} {
					layer.EXPECT().Digest().Return(v1.Hash{Algorithm: "sha", Hex: fmt.Sprintf("%d", i)}, nil).AnyTimes()
					layer.EXPECT().DiffID().Return(v1.Hash{Algorithm: "sha", Hex: fmt.Sprintf("%d", i)}, nil).AnyTimes()
					layer.EXPECT().Size().Return(int64(1022), nil).AnyTimes()
				}

				mockRepoImage.EXPECT().Layers().Return([]v1.Layer{
					oldBaseLayer1, oldBaseLayer2,
					appLayer1, appLayer2,
				}, nil).AnyTimes()

				mockOldBaseImage = mocks.NewMockImage(mockController)
				mockOldBaseImage.EXPECT().Layers().Return([]v1.Layer{
					oldBaseLayer1, oldBaseLayer2,
				}, nil).AnyTimes()

				mockNewBaseImage = mocks.NewMockImage(mockController)
				mockNewBaseImage.EXPECT().Layers().Return([]v1.Layer{
					newBaseLayer1, newBaseLayer2,
				}, nil).AnyTimes()

				rebaseConfig = &pack.RebaseConfig{
					Repo:      mockRepoStore,
					RepoImage: mockRepoImage,
					OldBase:   mockOldBaseImage,
					NewBase:   mockNewBaseImage,
				}

				mockRepoImage.EXPECT().ConfigFile().DoAndReturn(func() (*v1.ConfigFile, error) {
					return &v1.ConfigFile{
						History: []v1.History{{}, {}, {}, {}, {}, {}, {}, {}},
						Config: v1.Config{
							Labels: map[string]string{
								"io.buildpacks.stack.id":           "some.default.stack",
								"io.buildpacks.lifecycle.metadata": `{"runimage":{"sha":"sha256:abcdef"},"otherkey":"randomvalue"}`,
							},
						},
					}, nil
				}).AnyTimes()
				mockOldBaseImage.EXPECT().ConfigFile().DoAndReturn(func() (*v1.ConfigFile, error) {
					return &v1.ConfigFile{
						History: []v1.History{{}, {}, {}, {}, {}, {}, {}, {}},
					}, nil
				}).AnyTimes()
				mockNewBaseImage.EXPECT().ConfigFile().DoAndReturn(func() (*v1.ConfigFile, error) {
					return &v1.ConfigFile{
						History: []v1.History{{}, {}, {}, {}, {}, {}, {}, {}},
					}, nil
				}).AnyTimes()

				mockRepoStore.EXPECT().Write(gomock.Any()).DoAndReturn(func(i v1.Image) error {
					savedImage = i
					return nil
				}).AnyTimes()
			})

			it("swaps the old base for the new base", func() {
				err := factory.Rebase(*rebaseConfig)
				assertNil(t, err)

				assertLayers(t, savedImage, []v1.Layer{
					newBaseLayer1, newBaseLayer2,
					appLayer1, appLayer2,
				})
			})

			it("stores new sha for new runimage", func() {
				err := factory.Rebase(*rebaseConfig)
				assertNil(t, err)

				cfg, err := savedImage.ConfigFile()
				assertNil(t, err)
				var metadata struct {
					RunImage struct {
						SHA string `json:"sha"`
					} `json:"runimage"`
					OtherKey string `json:"otherkey"`
				}
				assertNil(t, json.Unmarshal([]byte(cfg.Config.Labels["io.buildpacks.lifecycle.metadata"]), &metadata))

				newBaseTopSHA, err := newBaseLayer2.DiffID()
				assertNil(t, err)
				assertEq(t, metadata.RunImage.SHA, newBaseTopSHA.String())
				assertEq(t, metadata.OtherKey, "randomvalue")
			})
		})
	})
}

func assertLayers(t *testing.T, actual v1.Image, expected []v1.Layer) {
	t.Helper()
	assertNotNil(t, actual)
	actualLayers, err := actual.Layers()
	assertNil(t, err)
	assertEq(t, len(actualLayers), len(expected))
	for i, _ := range actualLayers {
		assertSameInstance(t, actualLayers[i], expected[i])
	}
}
