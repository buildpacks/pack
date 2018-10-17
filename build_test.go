package pack_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/lifecycle"
	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/uuid"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/mocks"
)

func TestBuild(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	assertNil(t, exec.Command("docker", "pull", "registry:2").Run())
	assertNil(t, exec.Command("docker", "pull", "packs/samples").Run())
	assertNil(t, exec.Command("docker", "pull", "packs/run").Run())
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
			RepoName:        "pack.build." + randString(10),
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
		assertNil(t, err)
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

		it("defaults to daemon, pulls builder and run images, selects run-image using builder's stack", func() {
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
				Builder:  "some/builder",
			})
			assertNil(t, err)
			assertEq(t, config.RunImage, "some/run")
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
			assertNil(t, err)
			assertEq(t, config.RunImage, "registry.com/some/run")
		})

		it("doesn't pull run images when --publish is passed", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockImage(mockController)
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
			assertNil(t, err)
			assertEq(t, config.RunImage, "some/run")
		})

		it("allows run-image from flags if the stacks match", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockImage(mockController)
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
			assertNil(t, err)
			assertEq(t, config.RunImage, "override/run")
		})

		it("doesn't allows run-image from flags if the stacks are difference", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockImage(mockController)
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
			assertError(t, err, `invalid stack: stack "other.stack.id" from run image "override/run" does not match stack "some.stack.id" from builder image "some/builder"`)
		})

		it("uses working dir if appDir is set to placeholder value", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockImage(mockController)
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
			assertNil(t, err)
			assertEq(t, config.RunImage, "override/run")
			assertEq(t, config.AppDir, os.Getenv("PWD"))
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
			assertError(t, err, `invalid builder image "some/builder": missing required label "io.buildpacks.stack.id"`)
		})
	})

	when("#Detect", func() {
		it("copies the app in to docker and chowns it (including directories)", func() {
			_, err := subject.Detect()
			assertNil(t, err)

			for _, name := range []string{"/workspace/app", "/workspace/app/app.js", "/workspace/app/mydir", "/workspace/app/mydir/myfile.txt"} {
				txt, err := exec.Command("docker", "run", "-v", subject.WorkspaceVolume+":/workspace", subject.Builder, "ls", "-ld", name).Output()
				assertNil(t, err)
				assertContains(t, string(txt), "pack pack")
			}
		})

		when("app is detected", func() {
			it("returns the successful group with node", func() {
				group, err := subject.Detect()
				assertNil(t, err)
				assertEq(t, group.Buildpacks[0].ID, "io.buildpacks.samples.nodejs")
			})
		})

		when("app is not detectable", func() {
			var badappDir string
			it.Before(func() {
				var err error
				badappDir, err = ioutil.TempDir("/tmp", "pack.build.badapp.")
				assertNil(t, err)
				assertNil(t, ioutil.WriteFile(filepath.Join(badappDir, "file.txt"), []byte("content"), 0644))
				subject.AppDir = badappDir
			})
			it.After(func() { os.RemoveAll(badappDir) })
			it("returns the successful group with node", func() {
				_, err := subject.Detect()

				assertNotNil(t, err)
				assertEq(t, err.Error(), "run detect container: failed with status code: 6")
			})
		})
	})

	when("#Analyze", func() {
		it.Before(func() {
			tmpDir, err := ioutil.TempDir("/tmp", "pack.build.analyze.")
			assertNil(t, err)
			defer os.RemoveAll(tmpDir)
			assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "group.toml"), []byte(`[[buildpacks]]
			  id = "io.buildpacks.samples.nodejs"
				version = "0.0.1"
			`), 0666))

			copyWorkspaceToDocker(t, tmpDir, subject.WorkspaceVolume)
		})
		when("no previous image exists", func() {
			when("publish", func() {
				var registryContainerName, registryPort string
				it.Before(func() {
					registryContainerName, registryPort = runRegistry(t)
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true
				})
				it.After(func() { assertNil(t, exec.Command("docker", "kill", registryContainerName).Run()) })

				it("informs the user", func() {
					err := subject.Analyze()
					assertNil(t, err)
					assertContains(t, buf.String(), "WARNING: skipping analyze, image not found or requires authentication to access")
				})
			})
			when("daemon", func() {
				it.Before(func() { subject.Publish = false })
				it("informs the user", func() {
					err := subject.Analyze()
					assertNil(t, err)
					assertContains(t, buf.String(), "WARNING: skipping analyze, image not found\n")
				})
			})
		})

		when("previous image exists", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", subject.RepoName, "-")
				cmd.Stdin = strings.NewReader("FROM scratch\n" + `LABEL io.buildpacks.lifecycle.metadata='{"buildpacks":[{"key":"io.buildpacks.samples.nodejs","layers":{"node_modules":{"sha":"sha256:99311ec03d790adf46d35cd9219ed80a7d9a4b97f761247c02c77e7158a041d5","data":{"lock_checksum":"eb04ed1b461f1812f0f4233ef997cdb5"}}}}]}'` + "\n")
				assertNil(t, cmd.Run())
			})
			it.After(func() {
				exec.Command("docker", "rmi", subject.RepoName).Run()
			})

			when("publish", func() {
				var registryContainerName, registryPort string
				it.Before(func() {
					oldRepoName := subject.RepoName
					registryContainerName, registryPort = runRegistry(t)
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true

					assertNil(t, exec.Command("docker", "tag", oldRepoName, subject.RepoName).Run())
					assertNil(t, exec.Command("docker", "push", subject.RepoName).Run())
					assertNil(t, exec.Command("docker", "rmi", oldRepoName, subject.RepoName).Run())
				})
				it.After(func() {
					assertNil(t, exec.Command("docker", "kill", registryContainerName).Run())
				})

				it("tells the user nothing", func() {
					assertNil(t, subject.Analyze())

					txt := string(bytes.Trim(buf.Bytes(), "\x00"))
					assertEq(t, txt, "")
				})

				it("places files in workspace", func() {
					assertNil(t, subject.Analyze())

					txt := readFromDocker(t, subject.WorkspaceVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")

					assertEq(t, txt, "lock_checksum = \"eb04ed1b461f1812f0f4233ef997cdb5\"\n")
				})
			})

			when("daemon", func() {
				it.Before(func() { subject.Publish = false })

				it("tells the user nothing", func() {
					assertNil(t, subject.Analyze())

					txt := string(bytes.Trim(buf.Bytes(), "\x00"))
					assertEq(t, txt, "")
				})

				it("places files in workspace", func() {
					assertNil(t, subject.Analyze())

					txt := readFromDocker(t, subject.WorkspaceVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					assertEq(t, txt, "lock_checksum = \"eb04ed1b461f1812f0f4233ef997cdb5\"\n")
				})
			})
		})
	})

	when("#Export", func() {
		var group *lifecycle.BuildpackGroup
		it.Before(func() {
			tmpDir, err := ioutil.TempDir("/tmp", "pack.build.export.")
			assertNil(t, err)
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
				assertNil(t, os.MkdirAll(filepath.Dir(filepath.Join(tmpDir, name)), 0777))
				assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, name), []byte(txt), 0666))
			}
			copyWorkspaceToDocker(t, tmpDir, subject.WorkspaceVolume)

			group = &lifecycle.BuildpackGroup{
				Buildpacks: []*lifecycle.Buildpack{
					{ID: "io.buildpacks.samples.nodejs", Version: "0.0.1"},
				},
			}
		})
		it.After(func() { exec.Command("docker", "rmi", subject.RepoName).Run() })

		when("no previous image exists", func() {
			when("publish", func() {
				var oldRepoName, registryContainerName, registryPort string
				it.Before(func() {
					oldRepoName = subject.RepoName
					registryContainerName, registryPort = runRegistry(t)
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true
				})
				it.After(func() {
					assertNil(t, exec.Command("docker", "kill", registryContainerName).Run())
				})
				it("creates the image on the registry", func() {
					assertNil(t, subject.Export(group))
					images := httpGet(t, "http://localhost:"+registryPort+"/v2/_catalog")
					assertContains(t, images, oldRepoName)
				})
				it("puts the files on the image", func() {
					assertNil(t, subject.Export(group))

					assertNil(t, exec.Command("docker", "pull", subject.RepoName).Run())
					txt, err := exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/app/file.txt").Output()
					assertNil(t, err)
					assertEq(t, string(txt), "some text")

					txt, err = exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/io.buildpacks.samples.nodejs/mylayer/file.txt").Output()
					assertNil(t, err)
					assertEq(t, string(txt), "content")
				})
				it("sets the metadata on the image", func() {
					assertNil(t, subject.Export(group))

					assertNil(t, exec.Command("docker", "pull", subject.RepoName).Run())
					var metadata lifecycle.AppImageMetadata
					metadataJSON, err := exec.Command("docker", "inspect", subject.RepoName, "--format", `{{index .Config.Labels "io.buildpacks.lifecycle.metadata"}}`).Output()
					assertNil(t, err)
					assertNil(t, json.Unmarshal(metadataJSON, &metadata))

					assertContains(t, metadata.App.SHA, "sha256:")
					assertContains(t, metadata.Config.SHA, "sha256:")
					assertEq(t, len(metadata.Buildpacks), 1)
					assertContains(t, metadata.Buildpacks[0].Layers["mylayer"].SHA, "sha256:")
					assertEq(t, metadata.Buildpacks[0].Layers["mylayer"].Data, map[string]interface{}{"key": "myval"})
					assertContains(t, metadata.Buildpacks[0].Layers["other"].SHA, "sha256:")
				})
			})

			when("daemon", func() {
				it.Before(func() { subject.Publish = false })
				it("creates the image on the daemon", func() {
					assertNil(t, subject.Export(group))
					images, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}").Output()
					assertNil(t, err)
					assertContains(t, string(images), subject.RepoName)
				})
				it("puts the files on the image", func() {
					assertNil(t, subject.Export(group))

					txt, err := exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/app/file.txt").Output()
					assertNil(t, err)
					assertEq(t, string(txt), "some text")

					txt, err = exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/io.buildpacks.samples.nodejs/mylayer/file.txt").Output()
					assertNil(t, err)
					assertEq(t, string(txt), "content")
				})
				it("sets the metadata on the image", func() {
					assertNil(t, subject.Export(group))

					var metadata lifecycle.AppImageMetadata
					metadataJSON, err := exec.Command("docker", "inspect", subject.RepoName, "--format", `{{index .Config.Labels "io.buildpacks.lifecycle.metadata"}}`).Output()
					assertNil(t, err)
					assertNil(t, json.Unmarshal(metadataJSON, &metadata))

					assertEq(t, metadata.RunImage.Name, "packs/run")
					assertContains(t, metadata.App.SHA, "sha256:")
					assertContains(t, metadata.Config.SHA, "sha256:")
					assertEq(t, len(metadata.Buildpacks), 1)
					assertContains(t, metadata.Buildpacks[0].Layers["mylayer"].SHA, "sha256:")
					assertEq(t, metadata.Buildpacks[0].Layers["mylayer"].Data, map[string]interface{}{"key": "myval"})
					assertContains(t, metadata.Buildpacks[0].Layers["other"].SHA, "sha256:")
				})

				it("sets owner of layer files to PACK_USER_ID:PACK_USER_GID", func() {
					subject.RunImage = "packs/run-" + randString(8)
					defer exec.Command("docker", "rmi", subject.RunImage)
					cmd := exec.Command("docker", "build", "-t", subject.RunImage, "-")
					cmd.Stdin = strings.NewReader(`
						FROM packs/run
						ENV PACK_USER_ID 1234
						ENV PACK_USER_GID 5678
					`)
					assertNil(t, cmd.Run())

					assertNil(t, subject.Export(group))
					txt, err := exec.Command("docker", "run", subject.RepoName, "ls", "-la", "/workspace/app/file.txt").Output()
					assertNil(t, err)
					assertContains(t, string(txt), " 1234 5678 ")
				})

				it("errors if run image is missing PACK_USER_ID", func() {
					subject.RunImage = "packs/run-" + randString(8)
					defer exec.Command("docker", "rmi", subject.RunImage)
					cmd := exec.Command("docker", "build", "-t", subject.RunImage, "-")
					cmd.Stdin = strings.NewReader(`
						FROM packs/run
						ENV PACK_USER_ID ''
						ENV PACK_USER_GID 5678
					`)
					assertNil(t, cmd.Run())

					err := subject.Export(group)
					assertError(t, err, "export uid/gid: could not find PACK_USER_ID && PACK_USER_GID from run image")
				})
			})
		})

		when("previous image exists", func() {
			it("reuses images from previous layers", func() {
				addLayer := "ADD --chown=pack:pack /workspace/io.buildpacks.samples.nodejs/mylayer /workspace/io.buildpacks.samples.nodejs/mylayer"
				copyLayer := "COPY --from=prev --chown=pack:pack /workspace/io.buildpacks.samples.nodejs/mylayer /workspace/io.buildpacks.samples.nodejs/mylayer"

				t.Log("create image and assert add new layer")
				assertNil(t, subject.Export(group))
				assertContains(t, buf.String(), addLayer)

				t.Log("setup workspace to reuse layer")
				buf.Reset()
				assertNil(t, exec.Command("docker", "run", "--user=root", "-v", subject.WorkspaceVolume+":/workspace", "packs/samples", "rm", "-rf", "/workspace/io.buildpacks.samples.nodejs/mylayer").Run())

				t.Log("recreate image and assert copying layer from previous image")
				assertNil(t, subject.Export(group))
				assertContains(t, buf.String(), copyLayer)
			})
		})
	})

}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

// Assert deep equality (and provide useful difference as a test failure)
func assertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}

// Assert the simplistic pointer (or literal value) equality
func assertSameInstance(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if actual != expected {
		t.Fatalf("Expected %s and %s to be pointers to the variable", actual, expected)
	}
}

func assertError(t *testing.T, actual error, expected string) {
	t.Helper()
	if actual == nil {
		t.Fatalf("Expected an error but got nil")
	}
	if actual.Error() != expected {
		t.Fatalf(`Expected error to equal "%s", got "%s"`, expected, actual.Error())
	}
}

func assertContains(t *testing.T, actual, expected string) {
	t.Helper()
	if !strings.Contains(actual, expected) {
		t.Fatalf("Expected: '%s' inside '%s'", expected, actual)
	}
}

func assertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func assertNotNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual == nil {
		t.Fatal("Expected not nil")
	}
}

func contains(arr []string, val string) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}

func runRegistry(t *testing.T) (string, string) {
	t.Helper()
	name := "test-registry-" + randString(10)
	assertNil(t, exec.Command("docker", "run", "-d", "--rm", "-p", ":5000", "--name", name, "registry:2").Run())
	port, err := exec.Command("docker", "inspect", name, "-f", `{{index (index (index .NetworkSettings.Ports "5000/tcp") 0) "HostPort"}}`).Output()
	assertNil(t, err)
	return name, strings.TrimSpace(string(port))
}

func httpGet(t *testing.T, url string) string {
	resp, err := http.Get(url)
	assertNil(t, err)
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("HTTP Status was bad: %s => %d", url, resp.StatusCode)
	}
	b, err := ioutil.ReadAll(resp.Body)
	assertNil(t, err)
	return string(b)
}

func copyWorkspaceToDocker(t *testing.T, srcPath, destVolume string) {
	t.Helper()
	ctrName := uuid.New().String()
	defer exec.Command("docker", "rm", ctrName).Run()
	assertNil(t, exec.Command("docker", "create", "--name", ctrName, "-v", destVolume+":/workspace", "packs/samples", "true").Run())
	assertNil(t, exec.Command("docker", "cp", srcPath+"/.", ctrName+":/workspace/").Run())
}

func readFromDocker(t *testing.T, volume, path string) string {
	t.Helper()

	var buf bytes.Buffer
	cmd := exec.Command("docker", "run", "-v", volume+":/workspace", "packs/samples", "cat", path)
	cmd.Stdout = &buf
	assertNil(t, cmd.Run())
	return buf.String()
}
