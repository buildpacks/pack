package pack_test

import (
	"bytes"
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
			outBuf           bytes.Buffer
			errBuf           bytes.Buffer
			logger           *logging.Logger
			mockController   *gomock.Controller
			factory          *pack.BuildFactory
			mockImageFactory *mocks.MockImageFactory
			mockCache        *mocks.MockCache
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImageFactory = mocks.NewMockImageFactory(mockController)
			mockCache = mocks.NewMockCache(mockController)
			logger = logging.NewLogger(&outBuf, &errBuf, true, false)

			factory = &pack.BuildFactory{
				ImageFactory: mockImageFactory,
				Config: &config.Config{
					DefaultBuilder: "some/builder",
				},
				Logger: logger,
				Cache:  mockCache,
			}

			mockCache.EXPECT().Volume().AnyTimes()
		})

		it.After(func() {
			mockController.Finish()
		})

		it("defaults to daemon, default-builder, pulls builder and run images, selects run-image from builder", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", true).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.Builder, "some/builder")
		})

		it("respects builder from flags", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil)
			mockImageFactory.EXPECT().NewLocal("custom/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", true).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "custom/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.Builder, "custom/builder")
		})

		it("doesn't pull builder or run images when --no-pull is passed", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil)
			mockImageFactory.EXPECT().NewLocal("custom/builder", false).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", false).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				NoPull:   true,
				RepoName: "some/app",
				Builder:  "custom/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.Builder, "custom/builder")
		})

		it("selects run images with matching registry", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").
				Return(`{"runImage": {"image": "some/run", "mirrors": ["registry.com/some/run"]}}`, nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("registry.com/some/run", true).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "registry.com/some/app",
				Builder:  "some/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "registry.com/some/run")
			h.AssertEq(t, config.Builder, "some/builder")
		})

		when("both builder and local override run images have a matching registry", func() {
			it.Before(func() {
				factory.Config.RunImages = []config.RunImage{
					{
						Image:   "default/run",
						Mirrors: []string{"registry.com/override/run"},
					},
				}

				mockBuilderImage := mocks.NewMockImage(mockController)
				mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
				mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").
					Return(`{"runImage": {"image": "default/run", "mirrors": ["registry.com/default/run"]}}`, nil)
				mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

				mockRunImage := mocks.NewMockImage(mockController)
				mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
				mockRunImage.EXPECT().Found().Return(true, nil)
				mockImageFactory.EXPECT().NewLocal("registry.com/override/run", true).Return(mockRunImage, nil)
			})

			it("selects from local override run images first", func() {
				config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
					RepoName: "registry.com/some/app",
					Builder:  "some/builder",
				})
				h.AssertNil(t, err)
				h.AssertEq(t, config.RunImage, "registry.com/override/run")
				h.AssertEq(t, config.Builder, "some/builder")
			})
		})

		it("uses a remote run image when --publish is passed", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewRemote("some/run").Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				Publish:  true,
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.Builder, "some/builder")
		})

		it("allows run-image from flags if the stacks match", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewRemote("override/run").Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "override/run")
			h.AssertEq(t, config.Builder, "some/builder")
		})

		it("doesn't allow run-image from flags if the stacks are different", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("other.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewRemote("override/run").Return(mockRunImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
			})
			h.AssertError(t, err, "invalid stack: stack 'other.stack.id' from run image 'override/run' does not match stack 'some.stack.id' from builder image 'some/builder'")
		})

		it("uses working dir if appDir is set to placeholder value", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewRemote("some/run").Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				Publish:  true,
				AppDir:   "",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.Builder, "some/builder")
			h.AssertEq(t, config.LifecycleConfig.AppDir, os.Getenv("PWD"))
		})

		it("returns an error when the builder stack label is missing", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "invalid builder image 'some/builder': missing required label 'io.buildpacks.stack.id'")
		})

		it("returns an error when the builder stack label is empty", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "invalid builder image 'some/builder': missing required label 'io.buildpacks.stack.id'")
		})

		it("returns an error when the builder metadata label is missing", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "invalid builder image 'some/builder': missing required label 'io.buildpacks.builder.metadata' -- try recreating builder")
		})

		it("returns an error when the builder metadata label is unparsable", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("junk", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "invalid builder image metadata: invalid character 'j' looking for beginning of value")
		})

		it("returns an error if remote run image doesn't exist in remote on published builds", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(false, nil)
			mockImageFactory.EXPECT().NewRemote("some/run").Return(mockRunImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "some/run",
				Publish:  true,
			})
			h.AssertError(t, err, "remote run image 'some/run' does not exist")
		})

		it("returns an error if local run image doesn't exist locally on local builds", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(false, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", gomock.Any()).Return(mockRunImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "some/run",
				Publish:  false,
			})
			h.AssertError(t, err, "local run image 'some/run' does not exist")
		})

		it("sets EnvFile", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/run"}}`, nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", true).Return(mockRunImage, nil)

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

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				EnvFile:  envFile.Name(),
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.LifecycleConfig.EnvFile, map[string]string{
				"VAR1": "value1",
				"VAR2": "value2 with spaces",
				"PATH": os.Getenv("PATH"),
			})
			h.AssertNotEq(t, os.Getenv("PATH"), "")
		})
	}, spec.Parallel())
}
