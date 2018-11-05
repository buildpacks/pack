package pack_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/uuid"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestBuild(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	h.AssertNil(t, exec.Command("docker", "pull", "packs/samples").Run())
	h.AssertNil(t, exec.Command("docker", "pull", "packs/run").Run())
	defer h.StopRegistry(t)

	spec.Run(t, "build", testBuild, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuild(t *testing.T, when spec.G, it spec.S) {
	var subject *pack.BuildConfig
	var buf bytes.Buffer

	it.Before(func() {
		var err error
		subject = &pack.BuildConfig{
			AppDir:          "acceptance/testdata/node_app",
			Builder:         "packs/samples",
			RunImage:        "packs/run",
			RepoName:        "pack.build." + h.RandString(10),
			Publish:         false,
			WorkspaceVolume: fmt.Sprintf("pack-workspace-%x", uuid.New().String()),
			CacheVolume:     fmt.Sprintf("pack-cache-%x", uuid.New().String()),
			Stdout:          &buf,
			Stderr:          &buf,
			Log:             log.New(&buf, "", log.LstdFlags|log.Lshortfile),
			FS:              &fs.FS{},
			Images:          &image.Client{},
		}
		log.SetOutput(ioutil.Discard)
		subject.Cli, err = docker.New()
		h.AssertNil(t, err)
	})

	when("#BuildConfigFromFlags", func() {
		var (
			factory        *pack.BuildFactory
			mockController *gomock.Controller
			mockImages     *mocks.MockImages
			mockDocker     *mocks.MockDocker
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImages = mocks.NewMockImages(mockController)
			mockDocker = mocks.NewMockDocker(mockController)

			factory = &pack.BuildFactory{
				Images: mockImages,
				Config: &config.Config{
					DefaultBuilder: "some/builder",
					Stacks: []config.Stack{
						{
							ID:        "some.stack.id",
							RunImages: []string{"some/run", "registry.com/some/run"},
						},
					},
				},
				Cli: mockDocker,
				Log: log.New(&buf, "", log.LstdFlags|log.Lshortfile),
			}
		})

		it.After(func() {
			mockController.Finish()
		})

		it("defaults to daemon, default-builder, pulls builder and run images, selects run-image using builder's stack", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockDocker.EXPECT().PullImage("some/run")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/run").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
		})

		it("respects builder from flags", func() {
			mockDocker.EXPECT().PullImage("custom/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "custom/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockDocker.EXPECT().PullImage("some/run")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/run").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "custom/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
		})

		it("selects run images with matching registry", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockDocker.EXPECT().PullImage("registry.com/some/run")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "registry.com/some/run").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "registry.com/some/app",
				Builder:  "some/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "registry.com/some/run")
		})

		it("doesn't pull run images when --publish is passed", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockV1Image(mockController)
			mockImages.EXPECT().ReadImage("some/run", false).Return(mockRunImage, nil)
			mockRunImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{
				Config: v1.Config{
					Labels: map[string]string{
						"io.buildpacks.stack.id": "some.stack.id",
					},
				},
			}, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				Publish:  true,
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
		})

		it("allows run-image from flags if the stacks match", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockV1Image(mockController)
			mockImages.EXPECT().ReadImage("override/run", false).Return(mockRunImage, nil)
			mockRunImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{
				Config: v1.Config{
					Labels: map[string]string{
						"io.buildpacks.stack.id": "some.stack.id",
					},
				},
			}, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "override/run")
		})

		it("doesn't allows run-image from flags if the stacks are difference", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockV1Image(mockController)
			mockImages.EXPECT().ReadImage("override/run", false).Return(mockRunImage, nil)
			mockRunImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{
				Config: v1.Config{
					Labels: map[string]string{
						"io.buildpacks.stack.id": "other.stack.id",
					},
				},
			}, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
			})
			h.AssertError(t, err, `invalid stack: stack "other.stack.id" from run image "override/run" does not match stack "some.stack.id" from builder image "some/builder"`)
		})

		it("uses working dir if appDir is set to placeholder value", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockV1Image(mockController)
			mockImages.EXPECT().ReadImage("override/run", false).Return(mockRunImage, nil)
			mockRunImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{
				Config: v1.Config{
					Labels: map[string]string{
						"io.buildpacks.stack.id": "some.stack.id",
					},
				},
			}, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
				AppDir:   "current working directory",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "override/run")
			h.AssertEq(t, config.AppDir, os.Getenv("PWD"))
		})

		it("returns an errors when the builder stack label is missing", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{},
				},
			}, nil, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, `invalid builder image "some/builder": missing required label "io.buildpacks.stack.id"`)
		})
	})

	when("#Detect", func() {
		it("copies the app in to docker and chowns it (including directories)", func() {
			_, err := subject.Detect()
			h.AssertNil(t, err)

			for _, name := range []string{"/workspace/app", "/workspace/app/app.js", "/workspace/app/mydir", "/workspace/app/mydir/myfile.txt"} {
				txt, err := exec.Command("docker", "run", "-v", subject.WorkspaceVolume+":/workspace", subject.Builder, "ls", "-ld", name).Output()
				h.AssertNil(t, err)
				h.AssertContains(t, string(txt), "pack pack")
			}
		})

		when("app is detected", func() {
			it("returns the successful group with node", func() {
				group, err := subject.Detect()
				h.AssertNil(t, err)
				h.AssertEq(t, group.Buildpacks[0].ID, "io.buildpacks.samples.nodejs")
			})
		})

		when("app is not detectable", func() {
			var badappDir string
			it.Before(func() {
				var err error
				badappDir, err = ioutil.TempDir("/tmp", "pack.build.badapp.")
				h.AssertNil(t, err)
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(badappDir, "file.txt"), []byte("content"), 0644))
				subject.AppDir = badappDir
			})
			it.After(func() { os.RemoveAll(badappDir) })
			it("returns the successful group with node", func() {
				_, err := subject.Detect()

				h.AssertNotNil(t, err)
				h.AssertEq(t, err.Error(), "run detect container: failed with status code: 6")
			})
		})

		when("buildpacks are specified", func() {
			when("directory buildpack", func() {
				var bpDir string
				it.Before(func() {
					var err error
					bpDir, err = ioutil.TempDir("/tmp", "pack.build.bpdir.")
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

					_, err := subject.Detect()
					h.AssertNil(t, err)

					h.AssertMatch(t, buf.String(), regexp.MustCompile(`DETECTING WITH MANUALLY-PROVIDED GROUP:\n[0-9\s:\/]* Group: My Sample Buildpack: pass\n`))
				})
			})
			when("id@version buildpack", func() {
				it("symlinks directories to workspace and sets order.toml", func() {
					subject.Buildpacks = []string{
						"io.buildpacks.samples.nodejs@latest",
					}

					_, err := subject.Detect()
					h.AssertNil(t, err)

					h.AssertMatch(t, buf.String(), regexp.MustCompile(`DETECTING WITH MANUALLY-PROVIDED GROUP:\n[0-9\s:\/]* Group: Sample Node.js Buildpack: pass\n`))
				})
			})
		})
	})

	when("#Analyze", func() {
		it.Before(func() {
			tmpDir, err := ioutil.TempDir("/tmp", "pack.build.analyze.")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)
			h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "group.toml"), []byte(`[[buildpacks]]
			  id = "io.buildpacks.samples.nodejs"
				version = "0.0.1"
			`), 0666))

			h.CopyWorkspaceToDocker(t, tmpDir, subject.WorkspaceVolume)
		})
		when("no previous image exists", func() {
			when("publish", func() {
				var registryPort string
				it.Before(func() {
					registryPort = h.RunRegistry(t)
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true
				})

				it("informs the user", func() {
					err := subject.Analyze()
					h.AssertNil(t, err)
					h.AssertContains(t, buf.String(), "WARNING: skipping analyze, image not found or requires authentication to access")
				})
			})
			when("daemon", func() {
				it.Before(func() { subject.Publish = false })
				it("informs the user", func() {
					err := subject.Analyze()
					h.AssertNil(t, err)
					h.AssertContains(t, buf.String(), "WARNING: skipping analyze, image not found\n")
				})
			})
		})

		when("previous image exists", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", subject.RepoName, "-")
				cmd.Stdin = strings.NewReader("FROM scratch\n" + `LABEL io.buildpacks.lifecycle.metadata='{"buildpacks":[{"key":"io.buildpacks.samples.nodejs","layers":{"node_modules":{"sha":"sha256:99311ec03d790adf46d35cd9219ed80a7d9a4b97f761247c02c77e7158a041d5","data":{"lock_checksum":"eb04ed1b461f1812f0f4233ef997cdb5"}}}}]}'` + "\n")
				h.AssertNil(t, cmd.Run())
			})
			it.After(func() {
				h.RemoveImage(subject.RepoName)
			})

			when("publish", func() {
				var registryPort string
				it.Before(func() {
					oldRepoName := subject.RepoName
					registryPort = h.RunRegistry(t)

					subject.RepoName = "localhost:" + registryPort + "/" + oldRepoName
					subject.Publish = true

					h.Run(t, exec.Command("docker", "tag", oldRepoName, subject.RepoName))
					h.Run(t, exec.Command("docker", "push", subject.RepoName))
					h.RemoveImage(oldRepoName, subject.RepoName)
				})

				it("tells the user nothing", func() {
					h.AssertNil(t, subject.Analyze())

					txt := string(bytes.Trim(buf.Bytes(), "\x00"))
					h.AssertEq(t, txt, "")
				})

				it("places files in workspace", func() {
					h.AssertNil(t, subject.Analyze())

					txt := h.ReadFromDocker(t, subject.WorkspaceVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")

					h.AssertEq(t, txt, "lock_checksum = \"eb04ed1b461f1812f0f4233ef997cdb5\"\n")
				})
			})

			when("daemon", func() {
				it.Before(func() { subject.Publish = false })

				it("tells the user nothing", func() {
					h.AssertNil(t, subject.Analyze())

					txt := string(bytes.Trim(buf.Bytes(), "\x00"))
					h.AssertEq(t, txt, "")
				})

				it("places files in workspace", func() {
					h.AssertNil(t, subject.Analyze())

					txt := h.ReadFromDocker(t, subject.WorkspaceVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					h.AssertEq(t, txt, "lock_checksum = \"eb04ed1b461f1812f0f4233ef997cdb5\"\n")
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
					bpDir, err = ioutil.TempDir("/tmp", "pack.build.bpdir.")
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
				it.After(func() { os.RemoveAll(bpDir) })

				it("runs the buildpacks bin/build", func() {
					subject.Buildpacks = []string{bpDir}
					_, err := subject.Detect()
					h.AssertNil(t, err)

					err = subject.Build()
					h.AssertNil(t, err)

					h.AssertContains(t, buf.String(), "BUILD OUTPUT FROM MY SAMPLE BUILDPACK")
				})
			})
			when("id@version buildpack", func() {
				it("runs the buildpacks bin/build", func() {
					subject.Buildpacks = []string{"io.buildpacks.samples.nodejs@latest"}
					_, err := subject.Detect()
					h.AssertNil(t, err)

					err = subject.Build()
					h.AssertNil(t, err)

					h.AssertContains(t, buf.String(), "npm notice created a lockfile as package-lock.json. You should commit this file.")
				})
			})
		})
	})

	when("#Export", func() {
		var (
			group       *lifecycle.BuildpackGroup
			runSHA      string
			runTopLayer string
		)
		it.Before(func() {
			tmpDir, err := ioutil.TempDir("/tmp", "pack.build.export.")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)
			files := map[string]string{
				"group.toml":           "[[buildpacks]]\n" + `id = "io.buildpacks.samples.nodejs"` + "\n" + `version = "0.0.1"`,
				"app/file.txt":         "some text",
				"config/metadata.toml": "stuff = \"text\"",
				"io.buildpacks.samples.nodejs/mylayer.toml":     `key = "myval"`,
				"io.buildpacks.samples.nodejs/mylayer/file.txt": "content",
				"io.buildpacks.samples.nodejs/other.toml":       "",
				"io.buildpacks.samples.nodejs/other/file.txt":   "something",
			}
			for name, txt := range files {
				h.AssertNil(t, os.MkdirAll(filepath.Dir(filepath.Join(tmpDir, name)), 0777))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, name), []byte(txt), 0666))
			}
			h.CopyWorkspaceToDocker(t, tmpDir, subject.WorkspaceVolume)

			group = &lifecycle.BuildpackGroup{
				Buildpacks: []*lifecycle.Buildpack{
					{ID: "io.buildpacks.samples.nodejs", Version: "0.0.1"},
				},
			}
			h.AssertNil(t, exec.Command("docker", "pull", "packs/run").Run())
			runSHA = imageSHA(t, subject.RunImage)
			runTopLayer = topLayer(t, subject.RunImage)
		})
		it.After(func() { h.RemoveImage(subject.RepoName) })

		when("no previous image exists", func() {
			when("publish", func() {
				var oldRepoName, registryPort string
				it.Before(func() {
					oldRepoName = subject.RepoName
					registryPort = h.RunRegistry(t)

					subject.RepoName = "localhost:" + registryPort + "/" + oldRepoName
					subject.Publish = true
				})
				it("creates the image on the registry", func() {
					h.AssertNil(t, subject.Export(group))
					images := h.HttpGet(t, "http://localhost:"+registryPort+"/v2/_catalog")
					h.AssertContains(t, images, oldRepoName)
				})
				it("puts the files on the image", func() {
					h.AssertNil(t, subject.Export(group))

					h.Run(t, exec.Command("docker", "pull", subject.RepoName))
					txt := h.Run(t, exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/app/file.txt"))
					h.AssertEq(t, string(txt), "some text")

					txt = h.Run(t, exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/io.buildpacks.samples.nodejs/mylayer/file.txt"))
					h.AssertEq(t, string(txt), "content")
				})
				it("sets the metadata on the image", func() {
					h.AssertNil(t, subject.Export(group))

					h.Run(t, exec.Command("docker", "pull", subject.RepoName))
					var metadata lifecycle.AppImageMetadata
					metadataJSON := h.Run(t, exec.Command("docker", "inspect", subject.RepoName, "--format", `{{index .Config.Labels "io.buildpacks.lifecycle.metadata"}}`))
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
					if subject.Builder != "" {
						h.RemoveImage(subject.Builder)
					}
				})

				it("creates the image on the daemon", func() {
					h.AssertNil(t, subject.Export(group))
					images := h.Run(t, exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}"))
					h.AssertContains(t, string(images), subject.RepoName)
				})
				it("puts the files on the image", func() {
					h.AssertNil(t, subject.Export(group))

					txt := h.Run(t, exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/app/file.txt"))
					h.AssertEq(t, string(txt), "some text")

					txt = h.Run(t, exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/io.buildpacks.samples.nodejs/mylayer/file.txt"))
					h.AssertEq(t, string(txt), "content")
				})
				it("sets the metadata on the image", func() {
					h.AssertNil(t, subject.Export(group))

					var metadata lifecycle.AppImageMetadata
					metadataJSON := h.Run(t, exec.Command("docker", "inspect", subject.RepoName, "--format", `{{index .Config.Labels "io.buildpacks.lifecycle.metadata"}}`))
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

				it("sets owner of layer files to PACK_USER_ID:PACK_GROUP_ID", func() {
					subject.Builder = "packs/samples-" + h.RandString(8)
					cmd := exec.Command("docker", "build", "-t", subject.Builder, "-")
					cmd.Stdin = strings.NewReader(`
						FROM packs/samples
						ENV PACK_USER_ID 1234
						ENV PACK_GROUP_ID 5678
					`)
					h.Run(t, cmd)

					h.AssertNil(t, subject.Export(group))
					txt := h.Run(t, exec.Command("docker", "run", subject.RepoName, "ls", "-la", "/workspace/app/file.txt"))
					h.AssertContains(t, string(txt), " 1234 5678 ")
				})

				it("errors if run image is missing PACK_USER_ID", func() {
					subject.Builder = "packs/samples-" + h.RandString(8)
					cmd := exec.Command("docker", "build", "-t", subject.Builder, "-")
					cmd.Stdin = strings.NewReader(`
						FROM packs/samples
						ENV PACK_USER_ID ''
						ENV PACK_GROUP_ID 5678
					`)
					h.Run(t, cmd)

					err := subject.Export(group)
					h.AssertError(t, err, "export: not found pack uid & gid")
				})
			})
		})

		when("previous image exists", func() {
			it("reuses images from previous layers", func() {
				addLayer := "ADD --chown=1000:1000 /workspace/io.buildpacks.samples.nodejs/mylayer /workspace/io.buildpacks.samples.nodejs/mylayer"
				copyLayer := "COPY --from=prev --chown=1000:1000 /workspace/io.buildpacks.samples.nodejs/mylayer /workspace/io.buildpacks.samples.nodejs/mylayer"

				t.Log("create image and h.Assert add new layer")
				h.AssertNil(t, subject.Export(group))
				h.AssertContains(t, buf.String(), addLayer)

				t.Log("setup workspace to reuse layer")
				buf.Reset()
				h.Run(t, exec.Command("docker", "run", "--user=root", "-v", subject.WorkspaceVolume+":/workspace", "packs/samples", "rm", "-rf", "/workspace/io.buildpacks.samples.nodejs/mylayer"))

				t.Log("recreate image and h.Assert copying layer from previous image")
				h.AssertNil(t, subject.Export(group))
				h.AssertContains(t, buf.String(), copyLayer)
			})
		})
	})
}

func imageSHA(t *testing.T, repoName string) string {
	digests := make([]string, 1, 1)
	data := h.Run(t, exec.Command("docker", "inspect", repoName, "--format", `{{json .RepoDigests}}`))
	json.Unmarshal([]byte(data), &digests)
	sha := strings.Split(digests[0], "@")[1]
	return sha
}

func topLayer(t *testing.T, repoName string) string {
	layers := make([]string, 1, 1)
	layerData := h.Run(t, exec.Command("docker", "inspect", repoName, "--format", `{{json .RootFS.Layers}}`))
	json.Unmarshal([]byte(layerData), &layers)
	return strings.TrimSpace(layers[len(layers)-1])
}
