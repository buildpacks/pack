package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	lifecyclepkg "github.com/buildpack/lifecycle"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

var registryConfig *h.TestRegistryConfig

func TestLifecycle(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())

	registryConfig = h.RunRegistry(t, true)
	defer registryConfig.StopRegistry(t)
	packHome, err := ioutil.TempDir("", "build-test-pack-home")
	h.AssertNil(t, err)
	defer os.RemoveAll(packHome)
	h.ConfigurePackHome(t, packHome, registryConfig.RunRegistryPort)
	defer h.CleanDefaultImages(t, registryConfig.RunRegistryPort)

	spec.Run(t, "pack", testLifecycle, spec.Report(report.Terminal{}))
}

func testLifecycle(t *testing.T, when spec.G, it spec.S) {
	var (
		subject            *pack.BuildConfig
		outBuf             bytes.Buffer
		errBuf             bytes.Buffer
		dockerCli          *docker.Client
		logger             *logging.Logger
		defaultBuilderName string
		ctx                context.Context
		buildCache         *cache.Cache
	)

	it.Before(func() {
		var err error

		err = os.Setenv("DOCKER_CONFIG", registryConfig.DockerConfigDir)
		h.AssertNil(t, err)

		ctx = context.TODO()
		logger = logging.NewLogger(&outBuf, &errBuf, true, false)
		dockerCli, err = docker.New()
		h.AssertNil(t, err)
		repoName := "pack.build." + h.RandString(10)
		buildCache, err = cache.New(repoName, dockerCli)
		defaultBuilderName = h.DefaultBuilderImage(t, registryConfig.RunRegistryPort)
		subject = &pack.BuildConfig{
			Builder:  defaultBuilderName,
			RunImage: h.DefaultRunImage(t, registryConfig.RunRegistryPort),
			RepoName: repoName,
			Publish:  false,
			Cache:    buildCache,
			Logger:   logger,
			FS:       &fs.FS{},
			Cli:      dockerCli,
			LifecycleConfig: build.LifecycleConfig{
				BuilderImage: defaultBuilderName,
				Logger:       logger,
				AppDir:       "../acceptance/testdata/node_app",
			},
		}
	})

	it.After(func() {
		err := os.Unsetenv("DOCKER_CONFIG")
		h.AssertNil(t, err)
	})

	when("#Detect", func() {
		it("copies the app in to docker and chowns it (including directories)", func() {
			lifecycle, err := build.NewLifecycle(subject.LifecycleConfig)
			h.AssertNil(t, err)
			defer lifecycle.Cleanup(ctx)

			h.AssertNil(t, subject.Detect(ctx, lifecycle))

			for _, name := range []string{"/workspace/app", "/workspace/app/app.js", "/workspace/app/mydir", "/workspace/app/mydir/myfile.txt"} {
				txt := h.RunInImage(t, dockerCli, []string{lifecycle.WorkspaceVolume + ":/workspace"}, subject.Builder, "ls", "-ld", name)
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
				subject.LifecycleConfig.AppDir = badappDir
			})

			it.After(func() {
				os.RemoveAll(badappDir)
			})

			it("returns the successful group with node", func() {
				lifecycle, err := build.NewLifecycle(subject.LifecycleConfig)
				h.AssertNil(t, err)
				defer lifecycle.Cleanup(ctx)

				h.AssertError(t, subject.Detect(ctx, lifecycle), "run detect container: failed with status code: 6")
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
					subject.LifecycleConfig.Buildpacks = []string{
						bpDir,
					}
					lifecycle, err := build.NewLifecycle(subject.LifecycleConfig)
					h.AssertNil(t, err)
					defer lifecycle.Cleanup(ctx)

					h.AssertNil(t, subject.Detect(ctx, lifecycle))

					h.AssertNil(t, subject.Detect(ctx, lifecycle))
					h.AssertContains(t, outBuf.String(), `My Sample Buildpack: pass`)
				})
			})
			when("id@version buildpack", func() {
				it("symlinks directories to workspace and sets order.toml", func() {
					subject.LifecycleConfig.Buildpacks = []string{
						"io.buildpacks.samples.nodejs@latest",
					}

					lifecycle, err := build.NewLifecycle(subject.LifecycleConfig)
					h.AssertNil(t, err)
					defer lifecycle.Cleanup(ctx)

					h.AssertNil(t, subject.Detect(ctx, lifecycle))

					h.AssertContains(t, outBuf.String(), `Sample Node.js Buildpack: pass`)
				})
			})
		})

		when("EnvFile is specified", func() {
			it("sets specified env variables in /platform/env/...", func() {
				if runtime.GOOS == "windows" {
					t.Skip("directory buildpacks are not implemented on windows")
				}
				subject.LifecycleConfig.EnvFile = map[string]string{
					"VAR1": "value1",
					"VAR2": "value2 with spaces",
				}
				subject.LifecycleConfig.Buildpacks = []string{"../acceptance/testdata/mock_buildpacks/printenv"}
				lifecycle, err := build.NewLifecycle(subject.LifecycleConfig)
				h.AssertNil(t, err)
				defer lifecycle.Cleanup(ctx)

				h.AssertNil(t, subject.Detect(ctx, lifecycle))
				h.AssertContains(t, outBuf.String(), "DETECT: VAR1 is value1;")
				h.AssertContains(t, outBuf.String(), "DETECT: VAR2 is value2 with spaces;")
			})
		})
	})
	when("#Analyze", func() {
		var lifecycle *build.Lifecycle

		it.Before(func() {
			var err error

			logger = logging.NewLogger(&outBuf, &errBuf, true, false)
			dockerCli, err = docker.New()
			h.AssertNil(t, err)
			repoName := "pack.build." + h.RandString(10)
			buildCache, err := cache.New(repoName, dockerCli)
			defaultBuilderName = h.DefaultBuilderImage(t, registryConfig.RunRegistryPort)
			subject = &pack.BuildConfig{
				Builder:  defaultBuilderName,
				RunImage: h.DefaultRunImage(t, registryConfig.RunRegistryPort),
				RepoName: repoName,
				Publish:  false,
				Cache:    buildCache,
				Logger:   logger,
				FS:       &fs.FS{},
				Cli:      dockerCli,
				LifecycleConfig: build.LifecycleConfig{
					BuilderImage: defaultBuilderName,
					Logger:       logger,
					AppDir:       "../acceptance/testdata/node_app",
				},
			}

			tmpDir, err := ioutil.TempDir("", "pack.build.analyze.")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)
			h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "group.toml"), []byte(`[[buildpacks]]
			  id = "io.buildpacks.samples.nodejs"
			  version = "0.0.1"
			`), 0666))

			lifecycle, err = build.NewLifecycle(subject.LifecycleConfig)
			h.AssertNil(t, err)
			h.CopyWorkspaceToDocker(t, tmpDir, lifecycle.WorkspaceVolume)
		})

		it.After(func() {
			h.AssertNil(t, lifecycle.Cleanup(ctx))
		})

		when("no previous image exists", func() {
			when("publish", func() {
				it.Before(func() {
					subject.RepoName = registryConfig.RepoName(subject.RepoName)
					subject.Publish = true
				})

				it("succeeds and does nothing", func() {
					h.AssertNil(t, subject.Analyze(ctx, lifecycle))
				})
			})

			when("succeeds and does nothing", func() {
				it.Before(func() { subject.Publish = false })
				it("succeeds and does nothing", func() {
					err := subject.Analyze(ctx, lifecycle)
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
					subject.RepoName = h.CreateImageOnRemote(t, dockerCli, registryConfig, subject.RepoName, dockerFile)
				})

				it("places files in workspace and sets owner to pack", func() {
					h.AssertNil(t, subject.Analyze(ctx, lifecycle))

					txt := h.ReadFromDocker(t, lifecycle.WorkspaceVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")

					h.AssertEq(t, txt, `build = false
launch = true
cache = false

[metadata]
  lock_checksum = "eb04ed1b461f1812f0f4233ef997cdb5"
`)
					hdr := h.StatFromDocker(t, lifecycle.WorkspaceVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
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
					err := subject.Analyze(ctx, lifecycle)
					h.AssertNil(t, err)

					txt := h.ReadFromDocker(t, lifecycle.WorkspaceVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					h.AssertEq(t, txt, `build = false
launch = true
cache = false

[metadata]
  lock_checksum = "eb04ed1b461f1812f0f4233ef997cdb5"
`)
					hdr := h.StatFromDocker(t, lifecycle.WorkspaceVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					h.AssertEq(t, hdr.Uid, 1000)
					h.AssertEq(t, hdr.Gid, 1000)
				})
			})
		})
	}, spec.Sequential())

	when("#Build", func() {
		it.Before(func() {
			var err error

			logger = logging.NewLogger(&outBuf, &errBuf, true, false)
			dockerCli, err = docker.New()
			h.AssertNil(t, err)
			repoName := "pack.build." + h.RandString(10)
			buildCache, err := cache.New(repoName, dockerCli)
			defaultBuilderName = h.DefaultBuilderImage(t, registryConfig.RunRegistryPort)
			subject = &pack.BuildConfig{
				Builder:  defaultBuilderName,
				RunImage: h.DefaultRunImage(t, registryConfig.RunRegistryPort),
				RepoName: repoName,
				Publish:  false,
				Cache:    buildCache,
				Logger:   logger,
				FS:       &fs.FS{},
				Cli:      dockerCli,
				LifecycleConfig: build.LifecycleConfig{
					BuilderImage: defaultBuilderName,
					Logger:       logger,
					AppDir:       "../acceptance/testdata/node_app",
				},
			}
		})

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
					subject.LifecycleConfig.Buildpacks = []string{bpDir}
					lifecycle, err := build.NewLifecycle(subject.LifecycleConfig)
					h.AssertNil(t, err)
					defer lifecycle.Cleanup(ctx)

					h.AssertNil(t, subject.Detect(ctx, lifecycle))
					h.AssertNil(t, subject.Build(ctx, lifecycle))

					h.AssertContains(t, outBuf.String(), "BUILD OUTPUT FROM MY SAMPLE BUILDPACK")
				})
			})
			when("id@version buildpack", func() {
				it("runs the buildpacks bin/build", func() {
					subject.LifecycleConfig.Buildpacks = []string{"io.buildpacks.samples.nodejs@latest"}
					lifecycle, err := build.NewLifecycle(subject.LifecycleConfig)
					h.AssertNil(t, err)
					defer lifecycle.Cleanup(ctx)

					h.AssertNil(t, subject.Detect(ctx, lifecycle))
					h.AssertNil(t, subject.Build(ctx, lifecycle))

					h.AssertContains(t, outBuf.String(), "Sample Node.js Buildpack: pass")
				})
			})
		})

		when("EnvFile is specified", func() {
			it("sets specified env variables in /platform/env/...", func() {
				if runtime.GOOS == "windows" {
					t.Skip("directory buildpacks are not implemented on windows")
				}
				subject.LifecycleConfig.EnvFile = map[string]string{
					"VAR1": "value1",
					"VAR2": "value2 with spaces",
				}
				subject.LifecycleConfig.Buildpacks = []string{"../acceptance/testdata/mock_buildpacks/printenv"}
				lifecycle, err := build.NewLifecycle(subject.LifecycleConfig)
				h.AssertNil(t, err)
				defer lifecycle.Cleanup(ctx)

				h.AssertNil(t, subject.Detect(ctx, lifecycle))
				h.AssertNil(t, subject.Build(ctx, lifecycle))
				h.AssertContains(t, outBuf.String(), "BUILD: VAR1 is value1;")
				h.AssertContains(t, outBuf.String(), "BUILD: VAR2 is value2 with spaces;")
			})
		})
	}, spec.Sequential())

	when("#Export", func() {
		var (
			runSHA         string
			runTopLayer    string
			setupLayersDir func()
			lifecycle      *build.Lifecycle
		)

		it.Before(func() {
			var err error

			logger = logging.NewLogger(&outBuf, &errBuf, true, false)
			dockerCli, err = docker.New()
			h.AssertNil(t, err)
			repoName := "pack.build." + h.RandString(10)
			buildCache, err := cache.New(repoName, dockerCli)
			defaultBuilderName = h.DefaultBuilderImage(t, registryConfig.RunRegistryPort)
			subject = &pack.BuildConfig{
				Builder:                   defaultBuilderName,
				RunImage:                  h.DefaultRunImage(t, registryConfig.RunRegistryPort),
				LocallyConfiguredRunImage: false,
				RepoName:                  repoName,
				Publish:                   false,
				Cache:                     buildCache,
				Logger:                    logger,
				FS:                        &fs.FS{},
				Cli:                       dockerCli,
				LifecycleConfig: build.LifecycleConfig{
					BuilderImage: defaultBuilderName,
					Logger:       logger,
					AppDir:       "../acceptance/testdata/node_app",
				},
			}

			lifecycle, err = build.NewLifecycle(subject.LifecycleConfig)
			h.AssertNil(t, err)

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
				h.CopyWorkspaceToDocker(t, tmpDir, lifecycle.WorkspaceVolume)
			}
			setupLayersDir()

			runSHA = imageSHA(t, dockerCli, subject.RunImage)
			runTopLayer = topLayer(t, dockerCli, subject.RunImage)
		})

		it.After(func() {
			h.AssertNil(t, lifecycle.Cleanup(ctx))
		})

		when("publish", func() {
			var oldRepoName string

			it.Before(func() {
				oldRepoName = subject.RepoName

				subject.RepoName = registryConfig.RepoName(subject.RepoName)
				subject.Publish = true
			})

			it.After(func() {
				if t.Failed() {
					t.Log("OUTPUT:", outBuf.String())
				}
			})

			it("creates the image on the registry", func() {
				h.AssertNil(t, subject.Export(ctx, lifecycle))
				images, err := registryConfig.RegistryCatalog()
				h.AssertNil(t, err)
				h.AssertContains(t, images, oldRepoName)
			})

			it("puts the files on the image", func() {
				h.AssertNil(t, subject.Export(ctx, lifecycle))

				h.AssertNil(t, h.PullImageWithAuth(dockerCli, subject.RepoName, registryConfig.RegistryAuth()))
				defer h.DockerRmi(dockerCli, subject.RepoName)
				txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/app/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "some text")

				txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "content")
			})

			when("the run image is the default image", func() {
				it("sets the sets the run image label on the metadata of the image", func() {
					subject.LocallyConfiguredRunImage = false

					h.AssertNil(t, subject.Export(ctx, lifecycle))

					h.AssertNil(t, h.PullImageWithAuth(dockerCli, subject.RepoName, registryConfig.RegistryAuth()))
					defer h.DockerRmi(dockerCli, subject.RepoName)
					var metadata lifecyclepkg.AppImageMetadata
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

					runImageLabel := imageLabel(t, dockerCli, subject.RepoName, "io.buildpacks.run-image")
					h.AssertEq(t, runImageLabel, h.DefaultRunImage(t, registryConfig.RunRegistryPort))
				})
			})

			// todo when
			when("the run image is the configured locally", func() {
				it("does not set the run image label", func() {
					subject.LocallyConfiguredRunImage = true

					h.AssertNil(t, subject.Export(ctx, lifecycle))

					h.AssertNil(t, h.PullImageWithAuth(dockerCli, subject.RepoName, registryConfig.RegistryAuth()))
					defer h.DockerRmi(dockerCli, subject.RepoName)
					var metadata lifecyclepkg.AppImageMetadata
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

					assertImageLabelAbsent(t, dockerCli, subject.RepoName, "io.buildpacks.run-image")
				})
			})
		})

		when("daemon", func() {
			it.Before(func() { subject.Publish = false })

			it.After(func() {
				if t.Failed() {
					t.Log("Stdout:", outBuf.String())
					t.Log("Stderr:", errBuf.String())
				}
				h.AssertNil(t, h.DockerRmi(dockerCli, subject.RepoName))
			})

			it("creates the image on the daemon", func() {
				h.AssertNil(t, subject.Export(ctx, lifecycle))
				images := imageList(t, dockerCli)
				h.AssertSliceContains(t, images, subject.RepoName+":latest")
			})

			it("puts the files on the image", func() {
				h.AssertNil(t, subject.Export(ctx, lifecycle))

				txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/app/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "some text")

				txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "content")
			})

			when("the run image is the configured locally", func() {
				it("does not set the run image metadata on the image", func() {
					subject.LocallyConfiguredRunImage = true

					h.AssertNil(t, subject.Export(ctx, lifecycle))

					var metadata lifecyclepkg.AppImageMetadata
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

					assertImageLabelAbsent(t, dockerCli, subject.RepoName, "io.buildpacks.run-image")
				})
			})

			when("the run image is the default image", func() {
				it("sets the run image metadata", func() {
					subject.LocallyConfiguredRunImage = false

					h.AssertNil(t, subject.Export(ctx, lifecycle))

					var metadata lifecyclepkg.AppImageMetadata
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

					runImageLabel := imageLabel(t, dockerCli, subject.RepoName, "io.buildpacks.run-image")
					h.AssertEq(t, runImageLabel, h.DefaultRunImage(t, registryConfig.RunRegistryPort))
				})
			})

			when("PACK_USER_ID and PACK_GROUP_ID are set on builder", func() {
				var lifecycle2 *build.Lifecycle

				it.Before(func() {
					subject.LifecycleConfig.BuilderImage = "packs/samples-" + h.RandString(8)
					h.CreateImageOnLocal(t, dockerCli, subject.LifecycleConfig.BuilderImage, fmt.Sprintf(`
						FROM %s
						ENV PACK_USER_ID 1234
						ENV PACK_GROUP_ID 5678
						LABEL repo_name_for_randomisation=%s
					`, h.DefaultBuilderImage(t, registryConfig.RunRegistryPort), subject.LifecycleConfig.BuilderImage))
					var err error
					lifecycle2, err = build.NewLifecycle(subject.LifecycleConfig)
					h.AssertNil(t, err)

					// We need to carry over the workspace volume for our original lifecycle that is
					// setup in the #Export before each block. This is because export needs files
					// created by previous phases to exist.
					lifecycle2.WorkspaceVolume = lifecycle.WorkspaceVolume
				})

				it.After(func() {
					h.AssertNil(t, lifecycle2.Cleanup(ctx))
				})

				it("sets owner of layer files to PACK_USER_ID:PACK_GROUP_ID", func() {
					h.AssertNil(t, subject.Export(ctx, lifecycle2))
					txt := h.RunInImage(t, dockerCli, nil, subject.RepoName, "ls", "-la", "/workspace/app/file.txt")
					h.AssertContains(t, txt, " 1234 5678 ")
				})
			})

			when("previous image exists", func() {
				it.Before(func() {
					t.Log("create image and h.Assert add new layer")
					h.AssertNil(t, subject.Export(ctx, lifecycle))
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
					h.RunInImage(t, dockerCli,
						[]string{lifecycle.WorkspaceVolume + ":/workspace"},
						h.DefaultBuilderImage(t, registryConfig.RunRegistryPort),
						"rm", "-rf", "/workspace/io.buildpacks.samples.nodejs/mylayer",
					)

					t.Log("recreate image and h.Assert copying layer from previous image")
					h.AssertNil(t, subject.Export(ctx, lifecycle))
					txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
					h.AssertNil(t, err)
					h.AssertEq(t, txt, "content")
				})
			})
		})
	}, spec.Sequential())
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
	label, ok := inspect.Config.Labels[labelName]
	if !ok {
		t.Errorf("expected label %s to exist", labelName)
	}
	return label
}
func assertImageLabelAbsent(t *testing.T, dockerCli *docker.Client, repoName, labelName string) {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	val, ok := inspect.Config.Labels[labelName]
	if ok {
		t.Errorf("expected label %s not to exist but was %s", labelName, val)
	}
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
