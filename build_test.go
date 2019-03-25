package pack_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

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

var registryConfig *h.TestRegistryConfig

func TestBuildFactory(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "build_factory", testBuildFactory, spec.Report(report.Terminal{}))
}

func testBuildFactory(t *testing.T, when spec.G, it spec.S) {
	when("#BuildConfigFromFlags", func() {
		var (
			outBuf         bytes.Buffer
			errBuf         bytes.Buffer
			logger         *logging.Logger
			mockController *gomock.Controller
			factory        *pack.BuildFactory
			mockCache      *mocks.MockCache
			mockFetcher    *mocks.MockFetcher
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockCache = mocks.NewMockCache(mockController)
			mockFetcher = mocks.NewMockFetcher(mockController)
			logger = logging.NewLogger(&outBuf, &errBuf, true, false)

			factory = &pack.BuildFactory{
				Fetcher: mockFetcher,
				Config: &config.Config{
					DefaultBuilder: "some/builder",
				},
				Logger: logger,
				Cache:  mockCache,
			}

			mockCache.EXPECT().Image().AnyTimes()
		})

		it.After(func() {
			mockController.Finish()
		})

		it("defaults to daemon, default-builder, pulls builder and run images, selects run-image from builder", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil).AnyTimes()
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/run", gomock.Any()).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.LocallyConfiguredRunImage, false)
			h.AssertEq(t, config.Builder, "some/builder")
		})

		it("respects builder from flags", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil).AnyTimes()
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "custom/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/run", gomock.Any()).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "custom/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.LocallyConfiguredRunImage, false)
			h.AssertEq(t, config.Builder, "custom/builder")
		})

		it("doesn't pull builder or run images when --no-pull is passed", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil).AnyTimes()
			mockFetcher.EXPECT().FetchLocalImage("custom/builder").Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockFetcher.EXPECT().FetchLocalImage("some/run").Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				NoPull:   true,
				RepoName: "some/app",
				Builder:  "custom/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.LocallyConfiguredRunImage, false)
			h.AssertEq(t, config.Builder, "custom/builder")
		})

		it("selects run images with matching registry", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").
				Return(`{"runImage": {"image": "some/run", "mirrors": ["registry.com/some/run"]}}`, nil).AnyTimes()
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "registry.com/some/run", gomock.Any()).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "registry.com/some/app",
				Builder:  "some/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "registry.com/some/run")
			h.AssertEq(t, config.LocallyConfiguredRunImage, false)
			h.AssertEq(t, config.Builder, "some/builder")
		})

		when("both builder and local override run images have a matching registry", func() {
			var mockRunImage *mocks.MockImage

			it.Before(func() {
				factory.Config.RunImages = []config.RunImage{
					{
						Image:   "default/run",
						Mirrors: []string{"registry.com/override/run"},
					},
				}

				mockBuilderImage := mocks.NewMockImage(mockController)
				mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").
					Return(`{"runImage": {"image": "default/run", "mirrors": ["registry.com/default/run"]}}`, nil).AnyTimes()
				mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

				mockRunImage = mocks.NewMockImage(mockController)
				mockRunImage.EXPECT().Found().Return(true, nil)
			})

			it("selects from local override run images first", func() {
				mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "registry.com/override/run", gomock.Any()).Return(mockRunImage, nil)

				config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
					RepoName: "registry.com/some/app",
					Builder:  "some/builder",
				})
				h.AssertNil(t, err)
				h.AssertEq(t, config.RunImage, "registry.com/override/run")
				h.AssertEq(t, config.LocallyConfiguredRunImage, true)
				h.AssertEq(t, config.Builder, "some/builder")
			})

			it("selects the first local override if no run image matches the registry", func() {
				mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "registry.com/override/run", gomock.Any()).Return(mockRunImage, nil)

				config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
					RepoName: "some-other-registry.com/some/app",
					Builder:  "some/builder",
				})
				h.AssertNil(t, err)
				h.AssertEq(t, config.RunImage, "registry.com/override/run")
				h.AssertEq(t, config.LocallyConfiguredRunImage, true)
				h.AssertEq(t, config.Builder, "some/builder")
			})
		})

		it("uses a remote run image when --publish is passed", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil).AnyTimes()
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockFetcher.EXPECT().FetchRemoteImage("some/run").Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				Publish:  true,
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.LocallyConfiguredRunImage, false)
			h.AssertEq(t, config.Builder, "some/builder")
		})

		it("allows run-image from flags if the stacks match", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockFetcher.EXPECT().FetchRemoteImage("override/run").Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "override/run")
			h.AssertEq(t, config.LocallyConfiguredRunImage, true)
			h.AssertEq(t, config.Builder, "some/builder")
		})

		it("uses working dir if appDir is set to placeholder value", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil).AnyTimes()
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockFetcher.EXPECT().FetchRemoteImage("some/run").Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				Publish:  true,
				AppDir:   "",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.LocallyConfiguredRunImage, false)
			h.AssertEq(t, config.Builder, "some/builder")
			h.AssertEq(t, config.LifecycleConfig.AppDir, os.Getenv("PWD"))
		})

		it("returns an error when the builder metadata label is missing", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Name().Return("some/builder")
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("", nil)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "builder 'some/builder' missing label 'io.buildpacks.builder.metadata' -- try recreating builder")
		})

		it("returns an error when the builder metadata label is unparsable", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Name().Return("some/builder")
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("junk", nil)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "failed to parse metadata for builder 'some/builder': invalid character 'j' looking for beginning of value")
		})

		it("returns an error if remote run image doesn't exist in remote on published builds", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(false, nil)
			mockFetcher.EXPECT().FetchRemoteImage("some/run").Return(mockRunImage, nil)

			_, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "some/run",
				Publish:  true,
			})
			h.AssertError(t, err, "remote run image 'some/run' does not exist")
		})

		it("returns an error if local run image doesn't exist locally on local builds", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(false, nil)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/run", gomock.Any()).Return(mockRunImage, nil)

			_, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "some/run",
				Publish:  false,
			})
			h.AssertError(t, err, "local run image 'some/run' does not exist")
		})

		it("sets Env", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil).AnyTimes()
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/run", gomock.Any()).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				Env: []string{
					"VAR1=value1",
					"VAR2=value2 with spaces",
					"PATH",
				},
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.LifecycleConfig.Env, map[string]string{
				"VAR1": "value1",
				"VAR2": "value2 with spaces",
				"PATH": os.Getenv("PATH"),
			})
			h.AssertNotEq(t, os.Getenv("PATH"), "")
		})

		it("sets EnvFile", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil).AnyTimes()
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/run", gomock.Any()).Return(mockRunImage, nil)

			envFile, err := ioutil.TempFile("", "pack.build.envfile")
			h.AssertNil(t, err)
			defer os.Remove(envFile.Name())

			_, err = envFile.Write([]byte(`
VAR1=value1
VAR2=value2 with spaces
PATH
				`))
			h.AssertNil(t, err)
			envFile.Close()

			config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				EnvFile:  envFile.Name(),
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.LifecycleConfig.Env, map[string]string{
				"VAR1": "value1",
				"VAR2": "value2 with spaces",
				"PATH": os.Getenv("PATH"),
			})
			h.AssertNotEq(t, os.Getenv("PATH"), "")
		})

		it("sets EnvFile with Env overrides", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil).AnyTimes()
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/builder", gomock.Any()).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockFetcher.EXPECT().FetchUpdatedLocalImage(gomock.Any(), "some/run", gomock.Any()).Return(mockRunImage, nil)

			envFile, err := ioutil.TempFile("", "pack.build.envfile")
			h.AssertNil(t, err)
			defer os.Remove(envFile.Name())

			_, err = envFile.Write([]byte(`
VAR1=value1
VAR2=value2 with spaces
PATH
				`))
			h.AssertNil(t, err)
			envFile.Close()

			config, err := factory.BuildConfigFromFlags(context.TODO(), &pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				EnvFile:  envFile.Name(),
				Env:      []string{"VAR1=override1"},
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.LifecycleConfig.Env, map[string]string{
				"VAR1": "override1",
				"VAR2": "value2 with spaces",
				"PATH": os.Getenv("PATH"),
			})
			h.AssertNotEq(t, os.Getenv("PATH"), "")
		})
	}, spec.Parallel())
}
