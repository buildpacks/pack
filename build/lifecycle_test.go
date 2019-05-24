package build_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/buildpack/imgutil"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/internal/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

var (
	repoName  string
	dockerCli *client.Client
)

func TestLifecycle(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	color.NoColor = true

	h.RequireDocker(t)

	var err error
	dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)

	repoName = "lifecycle.test." + h.RandString(10)
	CreateFakeLifecycleImage(t, dockerCli, repoName)
	defer h.DockerRmi(dockerCli, repoName)

	spec.Run(t, "lifecycle", testLifecycle, spec.Report(report.Terminal{}), spec.Parallel())
}

func testLifecycle(t *testing.T, when spec.G, it spec.S) {
	var (
		subject        *build.Lifecycle
		outBuf, errBuf bytes.Buffer
		docker         *client.Client
	)

	it.Before(func() {
		logger := mocks.NewMockLogger(&outBuf)

		var err error
		docker, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
		h.AssertNil(t, err)
		subject = build.NewLifecycle(docker, logger)
		builderImage, err := imgutil.NewLocalImage(repoName, docker)
		h.AssertNil(t, err)
		bldr, err := builder.GetBuilder(builderImage)
		h.AssertNil(t, err)
		subject.Setup(build.LifecycleOptions{
			AppDir:     filepath.Join("testdata", "fake-app"),
			Builder:    bldr,
			HTTPProxy:  "some-http-proxy",
			HTTPSProxy: "some-https-proxy",
			NoProxy:    "some-no-proxy",
		})
	})

	it.After(func() {
		h.AssertNil(t, subject.Cleanup())
	})

	when("Phase", func() {
		when("#Run", func() {
			it("runs the subject phase on the builder image", func() {
				phase, err := subject.NewPhase("phase")
				h.AssertNil(t, err)
				assertRunSucceeds(t, phase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "running some-lifecycle-phase")
			})

			it("prefixes the output with the phase name", func() {
				phase, err := subject.NewPhase("phase")
				h.AssertNil(t, err)
				assertRunSucceeds(t, phase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "[phase] running some-lifecycle-phase")
			})

			it("attaches the same layers volume to each phase", func() {
				writePhase, err := subject.NewPhase("phase", build.WithArgs("write", "/layers/test.txt", "test-layers"))
				h.AssertNil(t, err)
				assertRunSucceeds(t, writePhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "[phase] write test")
				readPhase, err := subject.NewPhase("phase", build.WithArgs("read", "/layers/test.txt"))
				h.AssertNil(t, err)
				assertRunSucceeds(t, readPhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "[phase] file contents: test-layers")
			})

			it("attaches the same app volume to each phase", func() {
				writePhase, err := subject.NewPhase("phase", build.WithArgs("write", "/workspace/test.txt", "test-app"))
				h.AssertNil(t, err)
				assertRunSucceeds(t, writePhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "[phase] write test")
				readPhase, err := subject.NewPhase("phase", build.WithArgs("read", "/workspace/test.txt"))
				h.AssertNil(t, err)
				assertRunSucceeds(t, readPhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "[phase] file contents: test-app")
			})

			it("copies the app into the app volume before the first phase", func() {
				readPhase, err := subject.NewPhase("phase", build.WithArgs("read", "/workspace/fake-app-file"))
				h.AssertNil(t, err)
				assertRunSucceeds(t, readPhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "[phase] file contents: fake-app-contents")
				h.AssertContains(t, outBuf.String(), "[phase] file uid/gid 111/222")
				deletePhase, err := subject.NewPhase("phase", build.WithArgs("delete", "/workspace/fake-app-file"))
				h.AssertNil(t, err)
				assertRunSucceeds(t, deletePhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "[phase] delete test")
				readPhase2, err := subject.NewPhase("phase", build.WithArgs("read", "/workspace/fake-app-file"))
				h.AssertNil(t, err)
				err = readPhase2.Run(context.TODO())
				readPhase2.Cleanup()
				h.AssertNotNil(t, err)
				h.AssertContains(t, outBuf.String(), "failed to read file")
			})

			it("sets the proxy vars in the container", func() {
				phase, err := subject.NewPhase(
					"phase",
					build.WithArgs("proxy"),
				)
				h.AssertNil(t, err)
				assertRunSucceeds(t, phase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "HTTP_PROXY=some-http-proxy")
				h.AssertContains(t, outBuf.String(), "HTTPS_PROXY=some-https-proxy")
				h.AssertContains(t, outBuf.String(), "NO_PROXY=some-no-proxy")
				h.AssertContains(t, outBuf.String(), "http_proxy=some-http-proxy")
				h.AssertContains(t, outBuf.String(), "https_proxy=some-https-proxy")
				h.AssertContains(t, outBuf.String(), "no_proxy=some-no-proxy")
			})

			when("#WithArgs", func() {
				it("runs the subject phase with args", func() {
					phase, err := subject.NewPhase("phase", build.WithArgs("some", "args"))
					h.AssertNil(t, err)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), `received args [/lifecycle/phase some args]`)
				})
			})

			when("#WithDaemonAccess", func() {
				it("allows daemon access inside the container", func() {
					phase, err := subject.NewPhase(
						"phase",
						build.WithArgs("daemon"),
						build.WithDaemonAccess(),
					)
					h.AssertNil(t, err)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "[phase] daemon test")
				})
			})

			when("#WithBinds", func() {
				it.After(func() {
					docker.VolumeRemove(context.TODO(), "some-volume", true)
				})

				it("mounts volumes inside container", func() {
					phase, err := subject.NewPhase(
						"phase",
						build.WithArgs("binds"),
						build.WithBinds("some-volume:/mounted"),
					)
					h.AssertNil(t, err)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "[phase] binds test")
					body, err := docker.VolumeList(context.TODO(), filters.NewArgs(filters.KeyValuePair{
						Key:   "name",
						Value: "some-volume",
					}))
					h.AssertNil(t, err)
					h.AssertEq(t, len(body.Volumes), 1)
				})
			})

			when("#WithRegistryAccess", func() {
				var registry *h.TestRegistryConfig

				it.Before(func() {
					registry = h.RunRegistry(t, true)
				})

				it.After(func() {
					registry.StopRegistry(t)
				})

				it("provides auth for registry in the container", func() {
					phase, err := subject.NewPhase(
						"phase",
						build.WithArgs("registry", registry.RepoName("packs/build:v3alpha2")),
						build.WithRegistryAccess(),
					)
					h.AssertNil(t, err)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "[phase] registry test")
				})
			})
		})
	})

	when("#Cleanup", func() {
		it.Before(func() {
			phase, err := subject.NewPhase("phase")
			h.AssertNil(t, err)
			assertRunSucceeds(t, phase, &outBuf, &errBuf)
			h.AssertContains(t, outBuf.String(), "running some-lifecycle-phase")

			h.AssertNil(t, subject.Cleanup())
		})

		it("should delete the layers volume", func() {
			body, err := docker.VolumeList(context.TODO(),
				filters.NewArgs(filters.KeyValuePair{
					Key:   "name",
					Value: subject.LayersVolume,
				}))
			h.AssertNil(t, err)
			h.AssertEq(t, len(body.Volumes), 0)
		})

		it("should delete the app volume", func() {
			body, err := docker.VolumeList(context.TODO(),
				filters.NewArgs(filters.KeyValuePair{
					Key:   "name",
					Value: subject.AppVolume,
				}))
			h.AssertNil(t, err)
			h.AssertEq(t, len(body.Volumes), 0)
		})
	})
}

func assertRunSucceeds(t *testing.T, phase *build.Phase, outBuf *bytes.Buffer, errBuf *bytes.Buffer) {
	t.Helper()
	if err := phase.Run(context.TODO()); err != nil {
		phase.Cleanup()
		t.Fatalf("Failed to run phase '%s' \n stdout: '%s' \n stderr '%s'", err, outBuf.String(), errBuf.String())
	}
	phase.Cleanup()
}

func CreateFakeLifecycleImage(t *testing.T, dockerCli *client.Client, repoName string) {
	ctx := context.Background()

	wd, err := os.Getwd()
	h.AssertNil(t, err)
	buildContext, _ := archive.CreateTarReader(filepath.Join(wd, "testdata", "fake-lifecycle"), "/", 0, 0, -1)

	res, err := dockerCli.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:        []string{repoName},
		Remove:      true,
		ForceRemove: true,
	})
	h.AssertNil(t, err)

	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()
}
