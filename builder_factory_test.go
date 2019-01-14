package pack_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/buildpack/pack/logging"
	"github.com/fatih/color"
	"io/ioutil"
	"math/rand"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/buildpack/lifecycle"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestBuilderFactory(t *testing.T) {
	color.NoColor = true
	if runtime.GOOS == "windows" {
		t.Skip("create builder is not implemented on windows")
	}
	spec.Run(t, "builder_factory", testBuilderFactory, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilderFactory(t *testing.T, when spec.G, it spec.S) {
	when("#BuilderFactory", func() {
		const (
			defaultStack = "some.default.stack"
			otherStack   = "some.other.stack"
		)
		var (
			mockController   *gomock.Controller
			mockImageFactory *mocks.MockImageFactory
			factory          pack.BuilderFactory
			outBuf           bytes.Buffer
			errBuf           bytes.Buffer
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImageFactory = mocks.NewMockImageFactory(mockController)

			packHome, err := ioutil.TempDir("", ".pack")
			if err != nil {
				t.Fatalf("failed to create temp homedir: %v", err)
			}
			h.ConfigurePackHome(t, packHome, "0000")
			cfg, err := config.New(packHome)
			if err != nil {
				t.Fatalf("failed to create config: %v", err)
			}
			if err = cfg.AddStack(config.Stack{
				ID:         defaultStack,
				BuildImage: "default/build",
				RunImages:  []string{"default/run"},
			}); err != nil {
				t.Fatalf("failed to create config: %v", err)
			}
			if err = cfg.AddStack(config.Stack{ID: otherStack,
				BuildImage: "other/build",
				RunImages:  []string{"other/run", "other/run2"},
			}); err != nil {
				t.Fatalf("failed to create config: %v", err)
			}
			if err = cfg.SetDefaultStack(defaultStack); err != nil {
				t.Fatalf("failed to create config: %v", err)
			}

			factory = pack.BuilderFactory{
				FS:           &fs.FS{},
				Logger:       logging.NewLogger(&outBuf, &errBuf, true, false),
				Config:       cfg,
				ImageFactory: mockImageFactory,
			}
		})

		it.After(func() {
			mockController.Finish()
		})

		when("#BuilderConfigFromFlags", func() {
			it("uses default stack build image as base image", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("default/build", true).Return(mockBaseImage, nil)
				mockBaseImage.EXPECT().Rename("some/image")

				config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
				})
				if err != nil {
					t.Fatalf("error creating builder config: %s", err)
				}
				h.AssertSameInstance(t, config.Repo, mockBaseImage)
				checkBuildpacks(t, config.Buildpacks)
				checkGroups(t, config.Groups)
				h.AssertEq(t, config.BuilderDir, "testdata")
			})

			it("doesn't pull base a new image when --no-pull flag is provided", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("default/build", false).Return(mockBaseImage, nil)
				mockBaseImage.EXPECT().Rename("some/image")

				config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
					NoPull:          true,
				})
				if err != nil {
					t.Fatalf("error creating builder config: %s", err)
				}
				h.AssertSameInstance(t, config.Repo, mockBaseImage)
				checkBuildpacks(t, config.Buildpacks)
				checkGroups(t, config.Groups)
				h.AssertEq(t, config.BuilderDir, "testdata")
			})

			it("fails if the base image cannot be found", func() {
				mockImageFactory.EXPECT().NewLocal("default/build", true).Return(nil, fmt.Errorf("read image failed"))

				_, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
				})
				if err == nil {
					t.Fatalf("Expected error when base image is missing from daemon")
				}
			})

			it("uses the build image that matches the repoName registry", func() {})

			when("-s flag is provided", func() {
				it("used the build image from the selected stack", func() {
					mockBaseImage := mocks.NewMockImage(mockController)
					mockImageFactory.EXPECT().NewLocal("other/build", true).Return(mockBaseImage, nil)
					mockBaseImage.EXPECT().Rename("some/image")

					config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
						RepoName:        "some/image",
						BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
						StackID:         otherStack,
					})
					if err != nil {
						t.Fatalf("error creating builder config: %s", err)
					}
					h.AssertSameInstance(t, config.Repo, mockBaseImage)
					checkBuildpacks(t, config.Buildpacks)
					checkGroups(t, config.Groups)
					h.AssertEq(t, config.StackID, otherStack)
				})

				it("fails if the provided stack id does not exist", func() {
					_, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
						RepoName:        "some/image",
						BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
						NoPull:          true,
						StackID:         "some.missing.stack",
					})
					h.AssertError(t, err, "stack 'some.missing.stack' does not exist")
				})
			})

			when("--publish is passed", func() {
				it("uses a registry store and doesn't pull base image", func() {
					mockBaseImage := mocks.NewMockImage(mockController)
					mockImageFactory.EXPECT().NewRemote("default/build").Return(mockBaseImage, nil)
					mockBaseImage.EXPECT().Rename("some/image")

					config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
						RepoName:        "some/image",
						BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
						Publish:         true,
					})
					if err != nil {
						t.Fatalf("error creating builder config: %s", err)
					}
					h.AssertSameInstance(t, config.Repo, mockBaseImage)
					checkBuildpacks(t, config.Buildpacks)
					checkGroups(t, config.Groups)
					h.AssertEq(t, config.BuilderDir, "testdata")
				})
			})
		})

		when("#Create", func() {
			var mockImage *mocks.MockImage

			it.Before(func() {
				mockImage = mocks.NewMockImage(mockController)
				mockImage.EXPECT().AddLayer(gomock.Any()).AnyTimes()
			})

			when("stack is in config", func() {

				it.Before(func() {
					mockImage.EXPECT().Save()
				})

				it("stores metadata about the run images defined for the stack", func() {
					mockImage.EXPECT().SetLabel("io.buildpacks.pack.metadata", `{"runImages":["other/run","other/run2"]}`)

					err := factory.Create(pack.BuilderConfig{
						Repo:       mockImage,
						Buildpacks: []pack.Buildpack{},
						Groups:     []lifecycle.BuildpackGroup{},
						BuilderDir: "",
						StackID:    otherStack,
					})
					h.AssertNil(t, err)
				})
			})

			when("stack is not in config", func() {
				it("returns an error for the missing stack", func() {
					err := factory.Create(pack.BuilderConfig{
						Repo:       mockImage,
						Buildpacks: []pack.Buildpack{},
						Groups:     []lifecycle.BuildpackGroup{},
						BuilderDir: "",
						StackID:    "some.missing.stack",
					})

					h.AssertError(t, err, "failed to get run images: stack 'some.missing.stack' does not exist")
				})
			})
		})

		when("a buildpack location uses no scheme uris", func() {
			it("supports relative directories as well as archives", func() {
				mockImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("default/build", false).Return(mockImage, nil)
				mockImage.EXPECT().Rename("myorg/mybuilder")

				flags := pack.CreateBuilderFlags{
					RepoName:        "myorg/mybuilder",
					BuilderTomlPath: "testdata/used-to-test-various-uri-schemes/builder-with-schemeless-uris.toml",
					StackID:         defaultStack,
					Publish:         false,
					NoPull:          true,
				}

				builderConfig, err := factory.BuilderConfigFromFlags(flags)
				h.AssertNil(t, err)

				h.AssertDirContainsFileWithContents(t, builderConfig.Buildpacks[0].Dir, "bin/detect", "I come from a directory")
				h.AssertDirContainsFileWithContents(t, builderConfig.Buildpacks[1].Dir, "bin/build", "I come from an archive")
			})
			it("supports absolute directories as well as archives", func() {
				mockImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("default/build", false).Return(mockImage, nil)
				mockImage.EXPECT().Rename("myorg/mybuilder")

				absPath, err := filepath.Abs("testdata/used-to-test-various-uri-schemes/buildpack")
				h.AssertNil(t, err)

				f, err := ioutil.TempFile("", "*.toml")
				h.AssertNil(t, err)
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
					StackID:         defaultStack,
					Publish:         false,
					NoPull:          true,
				}

				builderConfig, err := factory.BuilderConfigFromFlags(flags)
				h.AssertNil(t, err)

				h.AssertDirContainsFileWithContents(t, builderConfig.Buildpacks[0].Dir, "bin/detect", "I come from a directory")
				h.AssertDirContainsFileWithContents(t, builderConfig.Buildpacks[1].Dir, "bin/build", "I come from an archive")
			})
		})
		when("a buildpack location uses file:// uris", func() {
			it("supports absolute directories as well as archives", func() {
				mockImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("default/build", false).Return(mockImage, nil)
				mockImage.EXPECT().Rename("myorg/mybuilder")

				absPath, err := filepath.Abs("testdata/used-to-test-various-uri-schemes/buildpack")
				h.AssertNil(t, err)

				f, err := ioutil.TempFile("", "*.toml")
				h.AssertNil(t, err)
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
					StackID:         defaultStack,
					Publish:         false,
					NoPull:          true,
				}

				builderConfig, err := factory.BuilderConfigFromFlags(flags)
				h.AssertNil(t, err)

				h.AssertDirContainsFileWithContents(t, builderConfig.Buildpacks[0].Dir, "bin/detect", "I come from a directory")
				h.AssertDirContainsFileWithContents(t, builderConfig.Buildpacks[1].Dir, "bin/build", "I come from an archive")
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
					t.Logf("Waiting for server to become ready on %s. Currently %v\n", server.Addr, err)
					time.Sleep(1 * time.Second)
				}
				if !serverReady {
					t.Fatal("http server does not seem to be up")
				}
			})
			it("downloads and extracts the archive", func() {
				mockImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("default/build", false).Return(mockImage, nil)
				mockImage.EXPECT().Rename("myorg/mybuilder")

				f, err := ioutil.TempFile("", "*.toml")
				h.AssertNil(t, err)
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
					StackID:         defaultStack,
					Publish:         false,
					NoPull:          true,
				}

				builderConfig, err := factory.BuilderConfigFromFlags(flags)
				h.AssertNil(t, err)

				h.AssertDirContainsFileWithContents(t, builderConfig.Buildpacks[0].Dir, "bin/build", "I come from an archive")
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
				ID:      "some/bp2",
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
			ID:     "some/bp2",
			Dir:    filepath.Join("testdata", "some-path-2"),
			Latest: false,
		},
		{
			ID:     "some/bp2",
			Dir:    filepath.Join("testdata", "some-latest-path-2"),
			Latest: true,
		},
	}); diff != "" {
		t.Fatalf("config has incorrect buildpacks, %s", diff)
	}
}
