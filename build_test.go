package pack_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/fatih/color"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/lifecycle"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

var registryPort string

func TestBuild(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())

	registryPort = h.RunRegistry(t, true)
	defer h.StopRegistry(t)
	packHome, err := ioutil.TempDir("", "build-test-pack-home")
	h.AssertNil(t, err)
	defer os.RemoveAll(packHome)
	h.ConfigurePackHome(t, packHome, registryPort)
	defer h.CleanDefaultImages(t, registryPort)

	spec.Run(t, "build", testBuild, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuild(t *testing.T, when spec.G, it spec.S) {
	var subject *pack.BuildConfig
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	var dockerCli *docker.Client
	var logger *logging.Logger

	it.Before(func() {
		var err error
		logger = logging.NewLogger(&outBuf, &errBuf, true, false)
		subject = &pack.BuildConfig{
			AppDir:      "acceptance/testdata/node_app",
			Builder:     h.DefaultBuilderImage(t, registryPort),
			RunImage:    h.DefaultRunImage(t, registryPort),
			RepoName:    "pack.build." + h.RandString(10),
			Publish:     false,
			CacheVolume: fmt.Sprintf("pack-cache-%x", uuid.New().String()),
			Logger:      logger,
			FS:          &fs.FS{},
		}
		dockerCli, err = docker.New()
		subject.Cli = dockerCli
		h.AssertNil(t, err)
	})
	it.After(func() {
		for _, volName := range []string{subject.CacheVolume, subject.CacheVolume} {
			dockerCli.VolumeRemove(context.TODO(), volName, true)
		}
	})

	when("#BuildConfigFromFlags", func() {
		var (
			factory          *pack.BuildFactory
			mockController   *gomock.Controller
			mockImageFactory *mocks.MockImageFactory
			mockDocker       *mocks.MockDocker
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImageFactory = mocks.NewMockImageFactory(mockController)
			mockDocker = mocks.NewMockDocker(mockController)

			factory = &pack.BuildFactory{
				ImageFactory: mockImageFactory,
				Config: &config.Config{
					DefaultBuilder: "some/builder",
				},
				Cli:    mockDocker,
				Logger: logger,
			}
		})

		it.After(func() {
			mockController.Finish()
		})

		it("defaults to daemon, default-builder, pulls builder and run images, selects run-image from builder", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
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
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
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
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
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
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run", "registry.com/some/run"]}`, nil)
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
				factory.Config.Builders = []config.Builder{
					{
						Image:     "some/builder",
						RunImages: []string{"registry.com/override/run"},
					},
				}

				mockBuilderImage := mocks.NewMockImage(mockController)
				mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
				mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["registry.com/default/run", "default/run"]}`, nil)
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
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
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
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
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
			h.AssertEq(t, config.AppDir, os.Getenv("PWD"))
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
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return("", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "invalid builder image 'some/builder': missing required label 'io.buildpacks.pack.metadata' -- try recreating builder")
		})

		it("returns an error when the builder metadata label is unparsable", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return("junk", nil)
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
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
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
			h.AssertEq(t, config.EnvFile, map[string]string{
				"VAR1": "value1",
				"VAR2": "value2 with spaces",
				"PATH": os.Getenv("PATH"),
			})
			h.AssertNotEq(t, os.Getenv("PATH"), "")
		})
	})

	when("#Detect", func() {
		it("copies the app in to docker and chowns it (including directories)", func() {
			h.AssertNil(t, subject.Detect())

			for _, name := range []string{"/workspace/app", "/workspace/app/app.js", "/workspace/app/mydir", "/workspace/app/mydir/myfile.txt"} {
				txt := runInImage(t, dockerCli, []string{subject.CacheVolume + ":/workspace"}, subject.Builder, "ls", "-ld", name)
				h.AssertContains(t, txt, "pack pack")
			}
		})

		when("app is not detectable", func() {
			var badappDir string
			it.Before(func() {
				var err error
				badappDir, err = ioutil.TempDir("", "pack.build.badapp.")
				h.AssertNil(t, err)
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(badappDir, "file.txt"), []byte("content"), 0644))
				subject.AppDir = badappDir
			})

			it.After(func() { os.RemoveAll(badappDir) })

			it("returns the successful group with node", func() {
				h.AssertError(t, subject.Detect(), "run detect container: failed with status code: 6")
			})
		})

		when("buildpacks are specified", func() {
			when("directory buildpack", func() {
				var bpDir string
				it.Before(func() {
					if runtime.GOOS == "windows" {
						t.Skip("directory buildpacks are not implemented on windows")
					}
					var err error
					bpDir, err = ioutil.TempDir("", "pack.build.bpdir.")
					h.AssertNil(t, err)
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "buildpack.toml"), []byte(`
					[buildpack]
					id = "com.example.mybuildpack"
					version = "1.2.3"
					name = "My Sample Buildpack"

					[[stacks]]
					id = "io.buildpacks.stacks.bionic"
					`), 0666))
					h.AssertNil(t, os.MkdirAll(filepath.Join(bpDir, "bin"), 0777))
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "bin", "detect"), []byte(`#!/usr/bin/env bash
					exit 0
					`), 0777))
				})
				it.After(func() { os.RemoveAll(bpDir) })

				it("copies directories to workspace and sets order.toml", func() {
					subject.Buildpacks = []string{
						bpDir,
					}

					h.AssertNil(t, subject.Detect())

					h.AssertContains(t, outBuf.String(), `My Sample Buildpack: pass`)
				})
			})
			when("id@version buildpack", func() {
				it("symlinks directories to workspace and sets order.toml", func() {
					subject.Buildpacks = []string{
						"io.buildpacks.samples.nodejs@latest",
					}

					h.AssertNil(t, subject.Detect())

					h.AssertContains(t, outBuf.String(), `Sample Node.js Buildpack: pass`)
				})
			})
		})

		when("EnvFile is specified", func() {
			it("sets specified env variables in /platform/env/...", func() {
				if runtime.GOOS == "windows" {
					t.Skip("directory buildpacks are not implemented on windows")
				}
				subject.EnvFile = map[string]string{
					"VAR1": "value1",
					"VAR2": "value2 with spaces",
				}
				subject.Buildpacks = []string{"acceptance/testdata/mock_buildpacks/printenv"}
				h.AssertNil(t, subject.Detect())
				h.AssertContains(t, outBuf.String(), "DETECT: VAR1 is value1;")
				h.AssertContains(t, outBuf.String(), "DETECT: VAR2 is value2 with spaces;")
			})
		})

		when("--clear-cache flag", func() {
			it.Before(func() {
				subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName

				runInImage(t, dockerCli, []string{subject.CacheVolume + ":/cache"}, subject.Builder,
					"bash", "-c", "echo foo > /cache/leftover.txt",
				)
				output := runInImage(t, dockerCli, []string{subject.CacheVolume + ":/cache"}, subject.Builder,
					"ls", "-la", "/cache",
				)
				h.AssertContains(t, output, "leftover.txt")
			})

			when("--clear-cache flag present", func() {
				it.Before(func() {
					subject.ClearCache = true
				})

				it("clears cache", func() {
					h.AssertNil(t, subject.Detect())
					output := runInImage(t, dockerCli, []string{subject.CacheVolume + ":/cache"}, subject.Builder,
						"ls", "-la", "/cache",
					)
					if strings.Contains(output, "leftover.txt") {
						t.Fatal("cache should have been cleared")
					}
					h.AssertContains(t, outBuf.String(), fmt.Sprintf("Cache volume '%s' cleared", subject.CacheVolume))
				})
			})

			when("--clear-cache not present", func() {
				it.Before(func() {
					subject.ClearCache = false
				})

				it("does not clear cache", func() {
					h.AssertNil(t, subject.Detect())
					output := runInImage(t, dockerCli, []string{subject.CacheVolume + ":/cache"}, subject.Builder,
						"ls", "-la", "/cache",
					)
					h.AssertContains(t, output, "leftover.txt")
				})
			})
		})
	})

	when("#Analyze", func() {
		it.Before(func() {
			tmpDir, err := ioutil.TempDir("", "pack.build.analyze.")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)
			h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "group.toml"), []byte(`[[buildpacks]]
			  id = "io.buildpacks.samples.nodejs"
			  version = "0.0.1"
			`), 0666))

			h.CopyWorkspaceToDocker(t, tmpDir, subject.CacheVolume)
		})

		when("no previous image exists", func() {
			when("publish", func() {
				it.Before(func() {
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true
				})

				it("succeeds and does nothing", func() {
					err := subject.Analyze()
					h.AssertNil(t, err)
				})
			})

			when("succeeds and does nothing", func() {
				it.Before(func() { subject.Publish = false })
				it("succeeds and does nothing", func() {
					err := subject.Analyze()
					h.AssertNil(t, err)
				})
			})
		})

		when("previous image exists", func() {
			var dockerFile string
			it.Before(func() {
				dockerFile = fmt.Sprintf(`
					FROM busybox
					LABEL io.buildpacks.lifecycle.metadata='{"buildpacks":[{"key":"io.buildpacks.samples.nodejs","layers":{"node_modules":{"launch": true, "sha":"sha256:99311ec03d790adf46d35cd9219ed80a7d9a4b97f761247c02c77e7158a041d5","data":{"lock_checksum":"eb04ed1b461f1812f0f4233ef997cdb5"}}}}]}'
					LABEL repo_name_for_randomisation=%s
				`, subject.RepoName)
			})

			when("publish", func() {
				it.Before(func() {
					subject.Publish = true
					subject.RepoName = h.CreateImageOnRemote(t, dockerCli, registryPort, subject.RepoName, dockerFile)
				})

				it("places files in workspace and sets owner to pack", func() {
					h.AssertNil(t, subject.Analyze())

					txt := h.ReadFromDocker(t, subject.CacheVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")

					h.AssertEq(t, txt, `build = false
launch = true
cache = false

[metadata]
  lock_checksum = "eb04ed1b461f1812f0f4233ef997cdb5"
`)
					hdr := h.StatFromDocker(t, subject.CacheVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					h.AssertEq(t, hdr.Uid, 1000)
					h.AssertEq(t, hdr.Gid, 1000)
				})
			})

			when("daemon", func() {
				it.Before(func() {
					subject.Publish = false

					h.CreateImageOnLocal(t, dockerCli, subject.RepoName, dockerFile)
				})

				it.After(func() {
					h.AssertNil(t, h.DockerRmi(dockerCli, subject.RepoName))
				})

				it("places files in workspace and sets owner to pack", func() {
					err := subject.Analyze()
					h.AssertNil(t, err)

					txt := h.ReadFromDocker(t, subject.CacheVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					h.AssertEq(t, txt, `build = false
launch = true
cache = false

[metadata]
  lock_checksum = "eb04ed1b461f1812f0f4233ef997cdb5"
`)
					hdr := h.StatFromDocker(t, subject.CacheVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					h.AssertEq(t, hdr.Uid, 1000)
					h.AssertEq(t, hdr.Gid, 1000)
				})
			})
		})
	})

	when("#Build", func() {
		when("buildpacks are specified", func() {
			when("directory buildpack", func() {
				var bpDir string
				it.Before(func() {
					var err error
					bpDir, err = ioutil.TempDir("", "pack.build.bpdir.")
					h.AssertNil(t, err)
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "buildpack.toml"), []byte(`
					[buildpack]
					id = "com.example.mybuildpack"
					version = "1.2.3"
					name = "My Sample Buildpack"

					[[stacks]]
					id = "io.buildpacks.stacks.bionic"
					`), 0666))
					h.AssertNil(t, os.MkdirAll(filepath.Join(bpDir, "bin"), 0777))
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "bin", "detect"), []byte(`#!/usr/bin/env bash
					exit 0
					`), 0777))
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "bin", "build"), []byte(`#!/usr/bin/env bash
					echo "BUILD OUTPUT FROM MY SAMPLE BUILDPACK"
					exit 0
					`), 0777))
				})
				it.After(func() {
					os.RemoveAll(bpDir)
				})

				it("runs the buildpacks bin/build", func() {
					if runtime.GOOS == "windows" {
						t.Skip("directory buildpacks are not implemented on windows")
					}
					subject.Buildpacks = []string{bpDir}

					h.AssertNil(t, subject.Detect())
					h.AssertNil(t, subject.Build())

					h.AssertContains(t, outBuf.String(), "BUILD OUTPUT FROM MY SAMPLE BUILDPACK")
				})
			})
			when("id@version buildpack", func() {
				it("runs the buildpacks bin/build", func() {
					subject.Buildpacks = []string{"io.buildpacks.samples.nodejs@latest"}

					h.AssertNil(t, subject.Detect())
					h.AssertNil(t, subject.Build())

					h.AssertContains(t, outBuf.String(), "Sample Node.js Buildpack: pass")
				})
			})
		})

		when("EnvFile is specified", func() {
			it("sets specified env variables in /platform/env/...", func() {
				if runtime.GOOS == "windows" {
					t.Skip("directory buildpacks are not implemented on windows")
				}
				subject.EnvFile = map[string]string{
					"VAR1": "value1",
					"VAR2": "value2 with spaces",
				}
				subject.Buildpacks = []string{"acceptance/testdata/mock_buildpacks/printenv"}
				h.AssertNil(t, subject.Detect())
				h.AssertNil(t, subject.Build())
				h.AssertContains(t, outBuf.String(), "BUILD: VAR1 is value1;")
				h.AssertContains(t, outBuf.String(), "BUILD: VAR2 is value2 with spaces;")
			})
		})
	})

	when("#Export", func() {
		var (
			runSHA         string
			runTopLayer    string
			setupLayersDir func()
		)
		it.Before(func() {
			tmpDir, err := ioutil.TempDir("", "pack.build.export.")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)
			setupLayersDir = func() {
				files := map[string]string{
					"group.toml":           "[[buildpacks]]\n" + `id = "io.buildpacks.samples.nodejs"` + "\n" + `version = "0.0.1"`,
					"app/file.txt":         "some text",
					"config/metadata.toml": "stuff = \"text\"",
					"io.buildpacks.samples.nodejs/mylayer.toml":     "launch = true\n[metadata]\n  key = \"myval\"",
					"io.buildpacks.samples.nodejs/mylayer/file.txt": "content",
					"io.buildpacks.samples.nodejs/other.toml":       "launch = true",
					"io.buildpacks.samples.nodejs/other/file.txt":   "something",
				}
				for name, txt := range files {
					h.AssertNil(t, os.MkdirAll(filepath.Dir(filepath.Join(tmpDir, name)), 0777))
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, name), []byte(txt), 0666))
				}
				h.CopyWorkspaceToDocker(t, tmpDir, subject.CacheVolume)
			}
			setupLayersDir()

			runSHA = imageSHA(t, dockerCli, subject.RunImage)
			runTopLayer = topLayer(t, dockerCli, subject.RunImage)
		})

		when("publish", func() {
			var oldRepoName string
			it.Before(func() {
				oldRepoName = subject.RepoName

				subject.RepoName = "localhost:" + registryPort + "/" + oldRepoName
				subject.Publish = true
			})

			it.After(func() {
				t.Log("OUTPUT:", outBuf.String())
			})

			it("creates the image on the registry", func() {
				h.AssertNil(t, subject.Export())
				images := h.HttpGet(t, "http://localhost:"+registryPort+"/v2/_catalog")
				h.AssertContains(t, images, oldRepoName)
			})

			it("puts the files on the image", func() {
				h.AssertNil(t, subject.Export())

				h.AssertNil(t, h.PullImage(dockerCli, subject.RepoName))
				defer h.DockerRmi(dockerCli, subject.RepoName)
				txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/app/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "some text")

				txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "content")
			})

			it("sets the metadata on the image", func() {
				h.AssertNil(t, subject.Export())

				h.AssertNil(t, h.PullImage(dockerCli, subject.RepoName))
				defer h.DockerRmi(dockerCli, subject.RepoName)
				var metadata lifecycle.AppImageMetadata
				metadataJSON := imageLabel(t, dockerCli, subject.RepoName, "io.buildpacks.lifecycle.metadata")
				t.Log(metadataJSON)
				h.AssertNil(t, json.Unmarshal([]byte(metadataJSON), &metadata))

				h.AssertEq(t, metadata.RunImage.SHA, runSHA)
				h.AssertEq(t, metadata.RunImage.TopLayer, runTopLayer)
				h.AssertContains(t, metadata.App.SHA, "sha256:")
				h.AssertContains(t, metadata.Config.SHA, "sha256:")
				h.AssertEq(t, len(metadata.Buildpacks), 1)
				h.AssertContains(t, metadata.Buildpacks[0].Layers["mylayer"].SHA, "sha256:")
				h.AssertEq(t, metadata.Buildpacks[0].Layers["mylayer"].Data, map[string]interface{}{"key": "myval"})
				h.AssertContains(t, metadata.Buildpacks[0].Layers["other"].SHA, "sha256:")
			})
		})

		when("daemon", func() {
			it.Before(func() { subject.Publish = false })

			it.After(func() {
				t.Log("OUTPUT:", outBuf.String())
				h.AssertNil(t, h.DockerRmi(dockerCli, subject.RepoName))
			})

			it("creates the image on the daemon", func() {
				h.AssertNil(t, subject.Export())
				images := imageList(t, dockerCli)
				h.AssertSliceContains(t, images, subject.RepoName+":latest")
			})
			it("puts the files on the image", func() {
				h.AssertNil(t, subject.Export())

				txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/app/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "some text")

				txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "content")
			})
			it("sets the metadata on the image", func() {
				h.AssertNil(t, subject.Export())

				var metadata lifecycle.AppImageMetadata
				metadataJSON := imageLabel(t, dockerCli, subject.RepoName, "io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, json.Unmarshal([]byte(metadataJSON), &metadata))

				h.AssertEq(t, metadata.RunImage.SHA, runSHA)
				h.AssertEq(t, metadata.RunImage.TopLayer, runTopLayer)
				h.AssertContains(t, metadata.App.SHA, "sha256:")
				h.AssertContains(t, metadata.Config.SHA, "sha256:")
				h.AssertEq(t, len(metadata.Buildpacks), 1)
				h.AssertContains(t, metadata.Buildpacks[0].Layers["mylayer"].SHA, "sha256:")
				h.AssertEq(t, metadata.Buildpacks[0].Layers["mylayer"].Data, map[string]interface{}{"key": "myval"})
				h.AssertContains(t, metadata.Buildpacks[0].Layers["other"].SHA, "sha256:")
			})

			when("PACK_USER_ID and PACK_GROUP_ID are set on builder", func() {
				it.Before(func() {
					subject.Builder = "packs/samples-" + h.RandString(8)
					h.CreateImageOnLocal(t, dockerCli, subject.Builder, fmt.Sprintf(`
						FROM %s
						ENV PACK_USER_ID 1234
						ENV PACK_GROUP_ID 5678
						LABEL repo_name_for_randomisation=%s
					`, h.DefaultBuilderImage(t, registryPort), subject.Builder))
				})

				it.After(func() {
					h.AssertNil(t, h.DockerRmi(dockerCli, subject.Builder))
				})

				it("sets owner of layer files to PACK_USER_ID:PACK_GROUP_ID", func() {
					h.AssertNil(t, subject.Export())
					txt := runInImage(t, dockerCli, nil, subject.RepoName, "ls", "-la", "/workspace/app/file.txt")
					h.AssertContains(t, txt, " 1234 5678 ")
				})
			})

			when("previous image exists", func() {
				it.Before(func() {
					t.Log("create image and h.Assert add new layer")
					h.AssertNil(t, subject.Export())
					setupLayersDir()
				})

				it("reuses images from previous layers", func() {
					origImageID := h.ImageID(t, subject.RepoName)
					defer func() { h.AssertNil(t, h.DockerRmi(dockerCli, origImageID)) }()

					txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
					h.AssertNil(t, err)
					h.AssertEq(t, txt, "content")

					t.Log("setup workspace to reuse layer")
					outBuf.Reset()
					runInImage(t, dockerCli,
						[]string{subject.CacheVolume + ":/workspace"},
						h.DefaultBuilderImage(t, registryPort),
						"rm", "-rf", "/workspace/io.buildpacks.samples.nodejs/mylayer",
					)

					t.Log("recreate image and h.Assert copying layer from previous image")
					h.AssertNil(t, subject.Export())
					txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
					h.AssertNil(t, err)
					h.AssertEq(t, txt, "content")
				})
			})
		})
	})
}

func imageSHA(t *testing.T, dockerCli *docker.Client, repoName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	sha := strings.Split(inspect.RepoDigests[0], "@")[1]
	return sha
}

func topLayer(t *testing.T, dockerCli *docker.Client, repoName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	layers := inspect.RootFS.Layers
	return layers[len(layers)-1]
}

func imageLabel(t *testing.T, dockerCli *docker.Client, repoName, labelName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	return inspect.Config.Labels[labelName]
}

func imageList(t *testing.T, dockerCli *docker.Client) []string {
	t.Helper()
	var out []string
	list, err := dockerCli.ImageList(context.Background(), dockertypes.ImageListOptions{})
	h.AssertNil(t, err)
	for _, s := range list {
		out = append(out, s.RepoTags...)
	}
	return out
}

func runInImage(t *testing.T, dockerCli *docker.Client, volumes []string, repoName string, args ...string) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := dockerCli.ContainerCreate(ctx, &dockercontainer.Config{
		Image: repoName,
		Cmd:   args,
		User:  "root",
	}, &dockercontainer.HostConfig{
		AutoRemove: true,
		Binds:      volumes,
	}, nil, "")
	h.AssertNil(t, err)
	okChan, errChan := dockerCli.ContainerWait(ctx, ctr.ID, container.WaitConditionRemoved)

	var buf bytes.Buffer
	err = dockerCli.RunContainer(ctx, ctr.ID, &buf, &buf)
	if err != nil {
		t.Fatalf("Expected nil: %s", errors.Wrap(err, buf.String()))
	}

	select {
	case <-okChan:
	case err = <-errChan:
		h.AssertNil(t, err)
	}
	return buf.String()
}
