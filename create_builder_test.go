package pack_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/mocks"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestCreateBuilder(t *testing.T) {
	spec.Run(t, "create-builder", testCreateBuilder, spec.Sequential(), spec.Report(report.Terminal{}))
}

//go:generate mockgen -package mocks -destination mocks/img.go github.com/google/go-containerregistry/pkg/v1 Image
//go:generate mockgen -package mocks -destination mocks/store.go github.com/buildpack/lifecycle/img Store

func testCreateBuilder(t *testing.T, when spec.G, it spec.S) {
	when("#BuilderFactory", func() {
		var (
			mockController *gomock.Controller
			mockDocker     *mocks.MockDocker
			mockImages     *mocks.MockImages
			factory        pack.BuilderFactory
			buf            bytes.Buffer
		)
		it.Before(func() {
			mockController = gomock.NewController(t)
			mockDocker = mocks.NewMockDocker(mockController)
			mockImages = mocks.NewMockImages(mockController)

			factory = pack.BuilderFactory{
				FS:     &fs.FS{},
				Docker: mockDocker,
				Log:    log.New(&buf, "", log.LstdFlags),
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

			output, err := exec.Command("docker", "pull", "packs/build").CombinedOutput()
			if err != nil {
				t.Fatalf("Failed to pull the base image in test setup: %s: %s", output, err)
			}
		})

		it.After(func() {
			mockController.Finish()
		})

		when("#BuilderConfigFromFlags", func() {
			it("uses default stack build image as base image", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageStore := mocks.NewMockStore(mockController)
				mockDocker.EXPECT().PullImage("default/build")
				mockImages.EXPECT().ReadImage("default/build", true).Return(mockBaseImage, nil)
				mockImages.EXPECT().RepoStore("some/image", true).Return(mockImageStore, nil)

				config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
				})
				if err != nil {
					t.Fatalf("error creating builder config: %s", err)
				}
				assertSameInstance(t, config.BaseImage, mockBaseImage)
				assertSameInstance(t, config.Repo, mockImageStore)
				checkBuildpacks(t, config.Buildpacks)
				checkGroups(t, config.Groups)
				assertEq(t, config.BuilderDir, "testdata")
				assertEq(t, config.RepoName, "some/image")
			})

			it("select the build image with matching registry", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageStore := mocks.NewMockStore(mockController)
				mockDocker.EXPECT().PullImage("registry.com/build/image")
				mockImages.EXPECT().ReadImage("registry.com/build/image", true).Return(mockBaseImage, nil)
				mockImages.EXPECT().RepoStore("registry.com/some/image", true).Return(mockImageStore, nil)

				config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "registry.com/some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
				})
				if err != nil {
					t.Fatalf("error creating builder config: %s", err)
				}
				assertSameInstance(t, config.BaseImage, mockBaseImage)
				assertSameInstance(t, config.Repo, mockImageStore)
				checkBuildpacks(t, config.Buildpacks)
				checkGroups(t, config.Groups)
				assertEq(t, config.BuilderDir, "testdata")
				assertEq(t, config.RepoName, "registry.com/some/image")
			})

			it("doesn't pull base a new image when --no-pull flag is provided", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageStore := mocks.NewMockStore(mockController)
				mockImages.EXPECT().ReadImage("default/build", true).Return(mockBaseImage, nil)
				mockImages.EXPECT().RepoStore("some/image", true).Return(mockImageStore, nil)

				config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
					NoPull:          true,
				})
				if err != nil {
					t.Fatalf("error creating builder config: %s", err)
				}
				assertSameInstance(t, config.BaseImage, mockBaseImage)
				assertSameInstance(t, config.Repo, mockImageStore)
				checkBuildpacks(t, config.Buildpacks)
				checkGroups(t, config.Groups)
				assertEq(t, config.BuilderDir, "testdata")
			})

			it("fails if the base image cannot be found", func() {
				mockImages.EXPECT().ReadImage("default/build", true).Return(nil, nil)

				_, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
					NoPull:          true,
				})
				if err == nil {
					t.Fatalf("Expected error when base image is missing from daemon")
				}
			})

			it("fails if the base image cannot be pulled", func() {
				mockDocker.EXPECT().PullImage("default/build").Return(fmt.Errorf("some-error"))

				_, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
				})
				if err == nil {
					t.Fatalf("Expected error when base image is missing from daemon")
				}
			})

			it("fails if there is no build image for the stack", func() {
				factory.Config = &config.Config{
					DefaultStackID: "some.bad.stack",
					Stacks: []config.Stack{
						{
							ID: "some.bad.stack",
						},
					},
				}
				_, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
					NoPull:          true,
				})
				assertError(t, err, `Invalid stack: stack "some.bad.stack" requires at least one build image`)
			})

			it("uses the build image that matches the repoName registry", func() {})

			when("-s flag is provided", func() {
				it("used the build image from the selected stack", func() {
					mockBaseImage := mocks.NewMockImage(mockController)
					mockImageStore := mocks.NewMockStore(mockController)
					mockDocker.EXPECT().PullImage("other/build")
					mockImages.EXPECT().ReadImage("other/build", true).Return(mockBaseImage, nil)
					mockImages.EXPECT().RepoStore("some/image", true).Return(mockImageStore, nil)

					config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
						RepoName:        "some/image",
						BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
						StackID:         "some.other.stack",
					})
					if err != nil {
						t.Fatalf("error creating builder config: %s", err)
					}
					assertSameInstance(t, config.BaseImage, mockBaseImage)
					assertSameInstance(t, config.Repo, mockImageStore)
					checkBuildpacks(t, config.Buildpacks)
					checkGroups(t, config.Groups)
				})

				it("fails if the provided stack id does not exist", func() {
					_, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
						RepoName:        "some/image",
						BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
						NoPull:          true,
						StackID:         "some.missing.stack",
					})
					assertError(t, err, `Missing stack: stack with id "some.missing.stack" not found in pack config.toml`)
				})
			})

			when("--publish is passed", func() {
				it("uses a registry store and doesn't pull base image", func() {
					mockBaseImage := mocks.NewMockImage(mockController)
					mockImageStore := mocks.NewMockStore(mockController)
					mockImages.EXPECT().ReadImage("default/build", false).Return(mockBaseImage, nil)
					mockImages.EXPECT().RepoStore("some/image", false).Return(mockImageStore, nil)

					config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
						RepoName:        "some/image",
						BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
						Publish:         true,
					})
					if err != nil {
						t.Fatalf("error creating builder config: %s", err)
					}
					assertSameInstance(t, config.BaseImage, mockBaseImage)
					assertSameInstance(t, config.Repo, mockImageStore)
					checkBuildpacks(t, config.Buildpacks)
					checkGroups(t, config.Groups)
					assertEq(t, config.BuilderDir, "testdata")
				})
			})
		})

		when("#Create", func() {
			when("successful", func() {
				it("logs usage tip", func() {
					mockBaseImage := mocks.NewMockImage(mockController)
					mockImageStore := mocks.NewMockStore(mockController)

					mockBaseImage.EXPECT().Manifest().Return(&v1.Manifest{}, nil)
					mockBaseImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{}, nil)
					mockImageStore.EXPECT().Write(gomock.Any())

					err := factory.Create(pack.BuilderConfig{
						RepoName:   "myorg/mybuilder",
						Repo:       mockImageStore,
						Buildpacks: []pack.Buildpack{},
						Groups:     []lifecycle.BuildpackGroup{},
						BaseImage:  mockBaseImage,
						BuilderDir: "",
					})
					assertNil(t, err)

					assertContains(t, buf.String(), "Successfully created builder image: myorg/mybuilder")
					assertContains(t, buf.String(), `Tip: Run "pack build <image name> --builder <builder image> --path <app source code>" to use this builder`)
				})
			})
		})
		when("a buildpack location uses no scheme uris", func() {
			it("supports relative directories", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockBaseImage.EXPECT().Manifest().Return(&v1.Manifest{}, nil)
				mockBaseImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{}, nil)
				mockImageStore.EXPECT().Write(gomock.Any())

				err := factory.Create(pack.BuilderConfig{
					RepoName:   "myorg/mybuilder",
					Repo:       mockImageStore,
					Buildpacks: []pack.Buildpack{{ID: "com.acme.foobar", Dir: "testdata/foobar"}},
					Groups:     []lifecycle.BuildpackGroup{},
					BaseImage:  mockBaseImage,
					BuilderDir: "",
				})
				assertNil(t, err)

				assertContains(t, buf.String(), "Successfully created builder image: myorg/mybuilder")
				assertContains(t, buf.String(), `Tip: Run "pack build <image name> --builder <builder image> --path <app source code>" to use this builder`)
			})
			it("supports relative archives", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockBaseImage.EXPECT().Manifest().Return(&v1.Manifest{}, nil)
				mockBaseImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{}, nil)
				mockImageStore.EXPECT().Write(gomock.Any())

				err := factory.Create(pack.BuilderConfig{
					RepoName:   "myorg/mybuilder",
					Repo:       mockImageStore,
					Buildpacks: []pack.Buildpack{{ID: "com.acme.foobar", Dir: "testdata/foobar.tgz"}},
					Groups:     []lifecycle.BuildpackGroup{},
					BaseImage:  mockBaseImage,
					BuilderDir: "",
				})
				assertNil(t, err)

				assertContains(t, buf.String(), "Successfully created builder image: myorg/mybuilder")
				assertContains(t, buf.String(), `Tip: Run "pack build <image name> --builder <builder image> --path <app source code>" to use this builder`)
			})
			it("supports absolute directories", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockBaseImage.EXPECT().Manifest().Return(&v1.Manifest{}, nil)
				mockBaseImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{}, nil)
				mockImageStore.EXPECT().Write(gomock.Any())

				uri, err := filepath.Abs("testdata/foobar")
				assertNil(t, err)

				err = factory.Create(pack.BuilderConfig{
					RepoName:   "myorg/mybuilder",
					Repo:       mockImageStore,
					Buildpacks: []pack.Buildpack{{ID: "com.acme.foobar", Dir: uri}},
					Groups:     []lifecycle.BuildpackGroup{},
					BaseImage:  mockBaseImage,
					BuilderDir: "",
				})
				assertNil(t, err)

				assertContains(t, buf.String(), "Successfully created builder image: myorg/mybuilder")
				assertContains(t, buf.String(), `Tip: Run "pack build <image name> --builder <builder image> --path <app source code>" to use this builder`)
			})
			it("supports absolute archives", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockBaseImage.EXPECT().Manifest().Return(&v1.Manifest{}, nil)
				mockBaseImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{}, nil)
				mockImageStore.EXPECT().Write(gomock.Any())

				uri, err := filepath.Abs("testdata/foobar.tgz")
				assertNil(t, err)


				err = factory.Create(pack.BuilderConfig{
					RepoName:   "myorg/mybuilder",
					Repo:       mockImageStore,
					Buildpacks: []pack.Buildpack{{ID: "com.acme.foobar", Dir: uri}},
					Groups:     []lifecycle.BuildpackGroup{},
					BaseImage:  mockBaseImage,
					BuilderDir: "",
				})
				assertNil(t, err)

				assertContains(t, buf.String(), "Successfully created builder image: myorg/mybuilder")
				assertContains(t, buf.String(), `Tip: Run "pack build <image name> --builder <builder image> --path <app source code>" to use this builder`)
			})
		})
		when("a buildpack location uses file:// uris", func() {
			it("supports directories", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockBaseImage.EXPECT().Manifest().Return(&v1.Manifest{}, nil)
				mockBaseImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{}, nil)
				mockImageStore.EXPECT().Write(gomock.Any())

				uri, err := filepath.Abs("testdata/foobar")
				assertNil(t, err)
				uri = "file://" + uri


				err = factory.Create(pack.BuilderConfig{
					RepoName:   "myorg/mybuilder",
					Repo:       mockImageStore,
					Buildpacks: []pack.Buildpack{{ID: "com.acme.foobar", Dir: uri}},
					Groups:     []lifecycle.BuildpackGroup{},
					BaseImage:  mockBaseImage,
					BuilderDir: "",
				})
				assertNil(t, err)

				assertContains(t, buf.String(), "Successfully created builder image: myorg/mybuilder")
				assertContains(t, buf.String(), `Tip: Run "pack build <image name> --builder <builder image> --path <app source code>" to use this builder`)
			})
			it("supports archives", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockBaseImage.EXPECT().Manifest().Return(&v1.Manifest{}, nil)
				mockBaseImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{}, nil)
				mockImageStore.EXPECT().Write(gomock.Any())

				uri, err := filepath.Abs("testdata/foobar.tgz")
				assertNil(t, err)
				uri = "file://" + uri

				err = factory.Create(pack.BuilderConfig{
					RepoName:   "myorg/mybuilder",
					Repo:       mockImageStore,
					Buildpacks: []pack.Buildpack{{ID: "com.acme.foobar", Dir: uri}},
					Groups:     []lifecycle.BuildpackGroup{},
					BaseImage:  mockBaseImage,
					BuilderDir: "",
				})
				assertNil(t, err)

				assertContains(t, buf.String(), "Successfully created builder image: myorg/mybuilder")
				assertContains(t, buf.String(), `Tip: Run "pack build <image name> --builder <builder image> --path <app source code>" to use this builder`)
			})
		})
		when.Focus("a buildpack location uses http(s):// uris", func() {
			var (
				server *http.Server
			)
			it("downloads an archive", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockBaseImage.EXPECT().Manifest().Return(&v1.Manifest{}, nil)
				mockBaseImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{}, nil)
				mockImageStore.EXPECT().Write(gomock.Any())

				port := rand.Int31n(65536 - 1024) + 1024
				fs := http.FileServer(http.Dir("testdata"))

				server = &http.Server{Addr:fmt.Sprintf(":%d", port), Handler:fs}
				go server.ListenAndServe()

				err := factory.Create(pack.BuilderConfig{
					RepoName:   "myorg/mybuilder",
					Repo:       mockImageStore,
					Buildpacks: []pack.Buildpack{{ID: "com.acme.foobar", Dir: fmt.Sprintf("http://localhost:%d/foobar.tgz", port)}},
					Groups:     []lifecycle.BuildpackGroup{},
					BaseImage:  mockBaseImage,
					BuilderDir: "",
				})
				assertNil(t, err)

				assertContains(t, buf.String(), "Successfully created builder image: myorg/mybuilder")
				assertContains(t, buf.String(), `Tip: Run "pack build <image name> --builder <builder image> --path <app source code>" to use this builder`)
			})
			it.After(func() {
				if server != nil {
					ctx, _ := context.WithTimeout(context.Background(), 2*time.Second)
					server.Shutdown(ctx)
				}
			})
		})
	})
}

func checkGroups(t *testing.T, groups []lifecycle.BuildpackGroup) {
	t.Helper()
	if diff := cmp.Diff(groups, []lifecycle.BuildpackGroup{
		{Buildpacks: []*lifecycle.Buildpack{
			{
				ID:      "some.bp1",
				Version: "1.2.3",
			},
			{
				ID:      "some.bp2",
				Version: "1.2.4",
			},
		}},
		{Buildpacks: []*lifecycle.Buildpack{
			{
				ID:      "some.bp1",
				Version: "1.2.3",
			},
		}},
	}); diff != "" {
		t.Fatalf("config has incorrect groups, %s", diff)
	}
}

func checkBuildpacks(t *testing.T, buildpacks []pack.Buildpack) {
	if diff := cmp.Diff(buildpacks, []pack.Buildpack{
		{
			ID:  "some.bp1",
			Dir: filepath.Join("testdata", "some-path-1"),
			// Latest will default to false
		},
		{
			ID:     "some.bp2",
			Dir:    filepath.Join("testdata", "some-path-2"),
			Latest: false,
		},
		{
			ID:     "some.bp2",
			Dir:    filepath.Join("testdata", "some-latest-path-2"),
			Latest: true,
		},
	}); diff != "" {
		t.Fatalf("config has incorrect buildpacks, %s", diff)
	}
}
