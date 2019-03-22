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
			it("uses stack build image as base image", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("some/build", true).Return(mockBaseImage, nil)
				mockBaseImage.EXPECT().Rename("some/image")

				cfg, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
				})
				if err != nil {
					t.Fatalf("error creating builder config: %s", err)
				}
				h.AssertSameInstance(t, cfg.Repo, mockBaseImage)
				checkBuildpacks(t, cfg.Buildpacks)
				checkGroups(t, cfg.Groups)
				h.AssertEq(t, cfg.BuilderDir, "testdata")
				h.AssertEq(t, cfg.RunImage, "some/run")
				h.AssertEq(t, cfg.RunImageMirrors, []string{"gcr.io/some/run2"})
			})

			it("doesn't pull a new base image when --no-pull flag is provided", func() {
				mockBaseImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("some/build", false).Return(mockBaseImage, nil)
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
				mockImageFactory.EXPECT().NewLocal("some/build", true).Return(nil, fmt.Errorf("read image failed"))

				_, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
				})
				if err == nil {
					t.Fatalf("Expected error when base image is missing from daemon")
				}
			})

			when("--publish is passed", func() {
				it("uses a registry store and doesn't pull base image", func() {
					mockBaseImage := mocks.NewMockImage(mockController)
					mockImageFactory.EXPECT().NewRemote("some/build").Return(mockBaseImage, nil)
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
				mockImage.EXPECT().Save()
			})

			it("stores metadata about the run image defined in builder TOML", func() {
				mockImage.EXPECT().SetLabel("io.buildpacks.builder.metadata", `{"runImage":{"image":"myorg/run","mirrors":["gcr.io/myorg/run"]}}`)

				err := factory.Create(pack.BuilderConfig{
					Repo:            mockImage,
					Buildpacks:      []pack.Buildpack{},
					Groups:          []lifecycle.BuildpackGroup{},
					BuilderDir:      "",
					RunImage:        "myorg/run",
					RunImageMirrors: []string{"gcr.io/myorg/run"},
				})
				h.AssertNil(t, err)
			})
		})

		when("a buildpack location uses no scheme uris", func() {
			it("supports relative directories as well as archives", func() {
				mockImage := mocks.NewMockImage(mockController)
				mockImageFactory.EXPECT().NewLocal("some/build", false).Return(mockImage, nil)
				mockImage.EXPECT().Rename("myorg/mybuilder")

				flags := pack.CreateBuilderFlags{
					RepoName:        "myorg/mybuilder",
					BuilderTomlPath: "testdata/used-to-test-various-uri-schemes/builder-with-schemeless-uris.toml",
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
				mockImageFactory.EXPECT().NewLocal("some/build", false).Return(mockImage, nil)
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
]

[stack]
id = "com.example.stack"
build-image = "some/build"
run-image = "some/run"
`, absPath, absPath)), 0644)
				f.Name()

				flags := pack.CreateBuilderFlags{
					RepoName:        "myorg/mybuilder",
					BuilderTomlPath: f.Name(),
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
				mockImageFactory.EXPECT().NewLocal("some/build", false).Return(mockImage, nil)
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
]

[stack]
id = "com.example.stack"
build-image = "some/build"
run-image = "some/run"
`, absPath, absPath)), 0644)
				f.Name()

				flags := pack.CreateBuilderFlags{
					RepoName:        "myorg/mybuilder",
					BuilderTomlPath: f.Name(),
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
				mockImageFactory.EXPECT().NewLocal("some/build", false).Return(mockImage, nil)
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
]

[stack]
id = "com.example.stack"
build-image = "some/build"
run-image = "some/run"
`, server.Addr)), 0644)
				f.Name()

				flags := pack.CreateBuilderFlags{
					RepoName:        "myorg/mybuilder",
					BuilderTomlPath: f.Name(),
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

		it("fails when a stack.build-image is not provided", func() {
			f, err := ioutil.TempFile("", "*.toml")
			h.AssertNil(t, err)
			ioutil.WriteFile(f.Name(), []byte(`
[stack]
id = "com.example.stack"
run-image = "some/run"
`), 0644)
			flags := pack.CreateBuilderFlags{
				RepoName:        "myorg/mybuilder",
				BuilderTomlPath: f.Name(),
				Publish:         false,
				NoPull:          true,
			}

			_, err = factory.BuilderConfigFromFlags(flags)
			h.AssertNotNil(t, err)
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
