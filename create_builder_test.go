package pack_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/buildpack/lifecycle"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/mocks"
)

func TestCreateBuilder(t *testing.T) {
	spec.Run(t, "create-builder", testCreateBuilder, spec.Sequential(), spec.Report(report.Terminal{}))
}

//go:generate mockgen -package mocks -destination mocks/img.go -mock_names Image=MockV1Image github.com/google/go-containerregistry/pkg/v1 Image
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

			home, err := ioutil.TempDir("", ".pack")
			if err != nil {
				t.Fatalf("failed to create temp homedir: %v", err)
			}
			cfg, err := config.New(home)
			if err != nil {
				t.Fatalf("failed to create config: %v", err)
			}
			if err = cfg.Add(config.Stack{
				ID:          "some.default.stack",
				BuildImages: []string{"default/build", "registry.com/build/image"},
				RunImages:   []string{"default/run"},
			}); err != nil {
				t.Fatalf("failed to create config: %v", err)
			}
			if err = cfg.Add(config.Stack{ID: "some.other.stack",
				BuildImages: []string{"other/build"},
				RunImages:   []string{"other/run"},
			}); err != nil {
				t.Fatalf("failed to create config: %v", err)
			}
			if err = cfg.SetDefaultStack("some.default.stack"); err != nil {
				t.Fatalf("failed to create config: %v", err)
			}

			factory = pack.BuilderFactory{
				FS:     &fs.FS{},
				Docker: mockDocker,
				Log:    log.New(&buf, "", log.LstdFlags),
				Config: cfg,
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
				mockBaseImage := mocks.NewMockV1Image(mockController)
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
				mockBaseImage := mocks.NewMockV1Image(mockController)
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
				mockBaseImage := mocks.NewMockV1Image(mockController)
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
					mockBaseImage := mocks.NewMockV1Image(mockController)
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
					mockBaseImage := mocks.NewMockV1Image(mockController)
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
					mockBaseImage := mocks.NewMockV1Image(mockController)
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
			it("supports relative directories as well as archives", func() {
				mockBaseImage := mocks.NewMockV1Image(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockImages.EXPECT().ReadImage("default/build", true).Return(mockBaseImage, nil)
				mockImages.EXPECT().RepoStore("myorg/mybuilder", true).Return(mockImageStore, nil)

				flags := pack.CreateBuilderFlags{
					RepoName:        "myorg/mybuilder",
					BuilderTomlPath: "testdata/used-to-test-various-uri-schemes/builder-with-schemeless-uris.toml",
					StackID:         "some.default.stack",
					Publish:         false,
					NoPull:          true,
				}

				builderConfig, err := factory.BuilderConfigFromFlags(flags)
				assertNil(t, err)

				assertDirContainsFileWithContents(t, builderConfig.Buildpacks[0].Dir, "bin/detect", "I come from a directory")
				assertDirContainsFileWithContents(t, builderConfig.Buildpacks[1].Dir, "bin/build", "I come from an archive")
			})
			it("supports absolute directories as well as archives", func() {
				mockBaseImage := mocks.NewMockV1Image(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockImages.EXPECT().ReadImage("default/build", true).Return(mockBaseImage, nil)
				mockImages.EXPECT().RepoStore("myorg/mybuilder", true).Return(mockImageStore, nil)

				absPath, err := filepath.Abs("testdata/used-to-test-various-uri-schemes/buildpack")
				assertNil(t, err)

				f, err := ioutil.TempFile("", "*.toml")
				assertNil(t, err)
				ioutil.WriteFile(f.Name(), []byte(fmt.Sprintf(`[[buildpacks]]
id = "some.bp.with.no.uri.scheme"
uri = "%s"

[[buildpacks]]
id = "some.bp.with.no.uri.scheme.and.tgz"
uri = "%s.tgz"

[[groups]]
buildpacks = [
  { id = "some.bp.with.no.uri.scheme", version = "1.2.3" },
  { id = "some.bp.with.no.uri.scheme.and.tgz", version = "1.2.4" },
]

[[groups]]
buildpacks = [
  { id = "some.bp1", version = "1.2.3" },
]`, absPath, absPath)), 0644)
				f.Name()

				flags := pack.CreateBuilderFlags{
					RepoName:        "myorg/mybuilder",
					BuilderTomlPath: f.Name(),
					StackID:         "some.default.stack",
					Publish:         false,
					NoPull:          true,
				}

				builderConfig, err := factory.BuilderConfigFromFlags(flags)
				assertNil(t, err)

				assertDirContainsFileWithContents(t, builderConfig.Buildpacks[0].Dir, "bin/detect", "I come from a directory")
				assertDirContainsFileWithContents(t, builderConfig.Buildpacks[1].Dir, "bin/build", "I come from an archive")
			})
		})
		when("a buildpack location uses file:// uris", func() {
			it("supports absolute directories as well as archives", func() {
				mockBaseImage := mocks.NewMockV1Image(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockImages.EXPECT().ReadImage("default/build", true).Return(mockBaseImage, nil)
				mockImages.EXPECT().RepoStore("myorg/mybuilder", true).Return(mockImageStore, nil)

				absPath, err := filepath.Abs("testdata/used-to-test-various-uri-schemes/buildpack")
				assertNil(t, err)

				f, err := ioutil.TempFile("", "*.toml")
				assertNil(t, err)
				ioutil.WriteFile(f.Name(), []byte(fmt.Sprintf(`[[buildpacks]]
id = "some.bp.with.no.uri.scheme"
uri = "file://%s"

[[buildpacks]]
id = "some.bp.with.no.uri.scheme.and.tgz"
uri = "file://%s.tgz"

[[groups]]
buildpacks = [
  { id = "some.bp.with.no.uri.scheme", version = "1.2.3" },
  { id = "some.bp.with.no.uri.scheme.and.tgz", version = "1.2.4" },
]

[[groups]]
buildpacks = [
  { id = "some.bp1", version = "1.2.3" },
]`, absPath, absPath)), 0644)
				f.Name()

				flags := pack.CreateBuilderFlags{
					RepoName:        "myorg/mybuilder",
					BuilderTomlPath: f.Name(),
					StackID:         "some.default.stack",
					Publish:         false,
					NoPull:          true,
				}

				builderConfig, err := factory.BuilderConfigFromFlags(flags)
				assertNil(t, err)

				assertDirContainsFileWithContents(t, builderConfig.Buildpacks[0].Dir, "bin/detect", "I come from a directory")
				assertDirContainsFileWithContents(t, builderConfig.Buildpacks[1].Dir, "bin/build", "I come from an archive")
			})
		})
		when("a buildpack location uses http(s):// uris", func() {
			var (
				server *http.Server
			)
			it.Before(func() {
				port := 1024 + rand.Int31n(65536-1024)
				fs := http.FileServer(http.Dir("testdata"))
				server = &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", port), Handler: fs}
				go func() {
					err := server.ListenAndServe()
					if err != http.ErrServerClosed {
						t.Fatalf("could not create http server: %v", err)
					}
				}()
				serverReady := false
				for i := 0; i < 10; i++ {
					resp, err := http.Get(fmt.Sprintf("http://%s/used-to-test-various-uri-schemes/buildpack.tgz", server.Addr))
					if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
						serverReady = true
						break
					}
					fmt.Printf("Waiting for server to become ready on %s. Currently %v\n", server.Addr, err)
					time.Sleep(1 * time.Second)
				}
				if !serverReady {
					t.Fatal("http server does not seem to be up")
				}
			})
			it("downloads and extracts the archive", func() {
				mockBaseImage := mocks.NewMockV1Image(mockController)
				mockImageStore := mocks.NewMockStore(mockController)

				mockImages.EXPECT().ReadImage("default/build", true).Return(mockBaseImage, nil)
				mockImages.EXPECT().RepoStore("myorg/mybuilder", true).Return(mockImageStore, nil)

				f, err := ioutil.TempFile("", "*.toml")
				assertNil(t, err)
				ioutil.WriteFile(f.Name(), []byte(fmt.Sprintf(`[[buildpacks]]
id = "some.bp.with.no.uri.scheme"
uri = "http://%s/used-to-test-various-uri-schemes/buildpack.tgz"

[[groups]]
buildpacks = [
  { id = "some.bp.with.no.uri.scheme", version = "1.2.3" },
]

[[groups]]
buildpacks = [
  { id = "some.bp1", version = "1.2.3" },
]`, server.Addr)), 0644)
				f.Name()

				flags := pack.CreateBuilderFlags{
					RepoName:        "myorg/mybuilder",
					BuilderTomlPath: f.Name(),
					StackID:         "some.default.stack",
					Publish:         false,
					NoPull:          true,
				}

				builderConfig, err := factory.BuilderConfigFromFlags(flags)
				assertNil(t, err)

				assertDirContainsFileWithContents(t, builderConfig.Buildpacks[0].Dir, "bin/build", "I come from an archive")
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

func assertDirContainsFileWithContents(t *testing.T, dir string, file string, expected string) {
	t.Helper()
	path := filepath.Join(dir, file)
	bytes, err := ioutil.ReadFile(path)
	assertNil(t, err)
	if string(bytes) != expected {
		t.Fatalf("file %s in dir %s has wrong contents: %s != %s", file, dir, string(bytes), expected)
	}
}
