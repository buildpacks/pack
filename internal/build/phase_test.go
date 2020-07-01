package build_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/builder"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

const phaseName = "phase"

var (
	repoName  string
	ctrClient client.CommonAPIClient
)

func TestPhase(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	color.Disable(true)
	defer color.Disable(false)

	h.RequireDocker(t)

	var err error
	ctrClient, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)

	info, err := ctrClient.Info(context.TODO())
	h.AssertNil(t, err)
	h.SkipIf(t, info.OSType == "windows", "These tests are not yet compatible with Windows-based containers")

	repoName = "phase.test.lc-" + h.RandString(10)
	wd, err := os.Getwd()
	h.AssertNil(t, err)
	h.CreateImageFromDir(t, ctrClient, repoName, filepath.Join(wd, "testdata", "fake-lifecycle"))
	defer h.DockerRmi(ctrClient, repoName)

	spec.Run(t, "phase", testPhase, spec.Report(report.Terminal{}), spec.Sequential())
}

func testPhase(t *testing.T, when spec.G, it spec.S) {
	var (
		lifecycle      *build.Lifecycle
		phaseFactory   *build.DefaultPhaseFactory
		outBuf, errBuf bytes.Buffer
		docker         client.CommonAPIClient
		logger         logging.Logger
	)

	it.Before(func() {
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

		var err error
		docker, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
		h.AssertNil(t, err)
		lifecycle, err = CreateFakeLifecycle(docker, logger, filepath.Join("testdata", "fake-app"), repoName)
		h.AssertNil(t, err)
		phaseFactory = build.NewDefaultPhaseFactory(lifecycle)
	})

	it.After(func() {
		h.AssertNil(t, lifecycle.Cleanup())
	})

	when("Phase", func() {
		when("#Run", func() {
			it("runs the subject phase on the builder image", func() {
				configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle)
				phase := phaseFactory.New(configProvider)
				assertRunSucceeds(t, phase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "running some-lifecycle-phase")
			})

			it("prefixes the output with the phase name", func() {
				configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithLogPrefix("phase"))
				phase := phaseFactory.New(configProvider)
				assertRunSucceeds(t, phase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "[phase] running some-lifecycle-phase")
			})

			it("attaches the same layers volume to each phase", func() {
				configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("write", "/layers/test.txt", "test-layers"))
				writePhase := phaseFactory.New(configProvider)

				assertRunSucceeds(t, writePhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "write test")

				configProvider = build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("read", "/layers/test.txt"))
				readPhase := phaseFactory.New(configProvider)
				assertRunSucceeds(t, readPhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "file contents: test-layers")
			})

			it("attaches the same app volume to each phase", func() {
				configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("write", "/workspace/test.txt", "test-app"))
				writePhase := phaseFactory.New(configProvider)
				assertRunSucceeds(t, writePhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "write test")

				configProvider = build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("read", "/workspace/test.txt"))
				readPhase := phaseFactory.New(configProvider)
				assertRunSucceeds(t, readPhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "file contents: test-app")
			})

			it("copies the app into the app volume", func() {
				configProvider := build.NewPhaseConfigProvider(
					phaseName,
					lifecycle,
					build.WithArgs("read", "/workspace/fake-app-file"),
					build.WithContainerOperations(
						build.CopyDir(
							lifecycle.AppPath(),
							"/workspace",
							lifecycle.Builder().UID(),
							lifecycle.Builder().GID(),
							nil,
						),
					),
				)
				readPhase := phaseFactory.New(configProvider)
				assertRunSucceeds(t, readPhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "file contents: fake-app-contents")
				h.AssertContains(t, outBuf.String(), "file uid/gid: 111/222")

				configProvider = build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("delete", "/workspace/fake-app-file"))
				deletePhase := phaseFactory.New(configProvider)
				assertRunSucceeds(t, deletePhase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "delete test")

				configProvider = build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("read", "/workspace/fake-app-file"))
				readPhase2 := phaseFactory.New(configProvider)
				err := readPhase2.Run(context.TODO())
				readPhase2.Cleanup()
				h.AssertNotNil(t, err)
				h.AssertContains(t, outBuf.String(), "failed to read file")
			})

			when("app is a dir", func() {
				it("preserves original mod times", func() {
					assertAppModTimePreserved(t, lifecycle, phaseFactory, &outBuf, &errBuf)
				})
			})

			when("app is a zip", func() {
				it("preserves original mod times", func() {
					var err error
					lifecycle, err = CreateFakeLifecycle(docker, logger, filepath.Join("testdata", "fake-app.zip"), repoName)
					h.AssertNil(t, err)
					phaseFactory = build.NewDefaultPhaseFactory(lifecycle)

					assertAppModTimePreserved(t, lifecycle, phaseFactory, &outBuf, &errBuf)
				})
			})

			when("is posix", func() {
				it.Before(func() {
					h.SkipIf(t, runtime.GOOS == "windows", "Skipping on windows")
				})

				when("restricted directory is present", func() {
					var (
						err              error
						tmpFakeAppDir    string
						dirWithoutAccess string
					)

					it.Before(func() {
						h.SkipIf(t, os.Getuid() == 0, "Skipping b/c current user is root")

						tmpFakeAppDir, err = ioutil.TempDir("", "fake-app")
						h.AssertNil(t, err)
						dirWithoutAccess = filepath.Join(tmpFakeAppDir, "bad-dir")
						err := os.MkdirAll(dirWithoutAccess, 0222)
						h.AssertNil(t, err)
					})

					it.After(func() {
						h.AssertNil(t, os.RemoveAll(tmpFakeAppDir))
					})

					it("returns an error", func() {
						logger := ilogging.NewLogWithWriters(&outBuf, &outBuf)
						lifecycle, err = CreateFakeLifecycle(docker, logger, tmpFakeAppDir, repoName)
						h.AssertNil(t, err)
						phaseFactory = build.NewDefaultPhaseFactory(lifecycle)
						readPhase := phaseFactory.New(build.NewPhaseConfigProvider(
							phaseName,
							lifecycle,
							build.WithArgs("read", "/workspace/fake-app-file"),
							build.WithContainerOperations(
								build.CopyDir(lifecycle.AppPath(), "/workspace", 0, 0, nil),
							),
						))
						h.AssertNil(t, err)
						err = readPhase.Run(context.TODO())
						defer readPhase.Cleanup()

						h.AssertNotNil(t, err)
						h.AssertContains(t,
							err.Error(),
							fmt.Sprintf("open %s: permission denied", dirWithoutAccess),
						)
					})
				})
			})

			it("sets the proxy vars in the container", func() {
				configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("proxy"))
				phase := phaseFactory.New(configProvider)
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
					configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("some", "args"))
					phase := phaseFactory.New(configProvider)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), `received args [/cnb/lifecycle/phase some args]`)
				})
			})

			when("#WithDaemonAccess", func() {
				it("allows daemon access inside the container", func() {
					configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("daemon"), build.WithDaemonAccess())
					phase := phaseFactory.New(configProvider)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "daemon test")
				})
			})

			when("#WithRoot", func() {
				it("sets the containers user to root", func() {
					configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("user"), build.WithRoot())
					phase := phaseFactory.New(configProvider)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "current user is root")
				})
			})

			when("#WithBinds", func() {
				it.After(func() {
					docker.VolumeRemove(context.TODO(), "some-volume", true)
				})

				it("mounts volumes inside container", func() {
					configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("binds"), build.WithBinds("some-volume:/mounted"))
					phase := phaseFactory.New(configProvider)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "binds test")
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
					registry = h.RunRegistry(t)
					h.AssertNil(t, os.Setenv("DOCKER_CONFIG", registry.DockerConfigDir))
				})

				it.After(func() {
					if registry != nil {
						registry.StopRegistry(t)
					}
					h.AssertNil(t, os.Unsetenv("DOCKER_CONFIG"))
				})

				it("provides auth for registry in the container", func() {
					repoName := h.CreateImageOnRemote(t, ctrClient, registry, "packs/build:v3alpha2", "FROM busybox")

					authConfig, err := auth.BuildEnvVar(authn.DefaultKeychain, repoName)
					h.AssertNil(t, err)

					configProvider := build.NewPhaseConfigProvider(
						phaseName,
						lifecycle,
						build.WithArgs("registry", repoName),
						build.WithRegistryAccess(authConfig),
						build.WithNetwork("host"),
					)
					phase := phaseFactory.New(configProvider)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "registry test")
				})
			})

			when("#WithNetwork", func() {
				it("specifies a network for the container", func() {
					configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle, build.WithArgs("network"), build.WithNetwork("none"))
					phase := phaseFactory.New(configProvider)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertNotContainsMatch(t, outBuf.String(), `interface: eth\d+`)
					h.AssertContains(t, outBuf.String(), `error connecting to internet:`)
				})
			})
		})
	})

	when("#Cleanup", func() {
		it.Before(func() {
			configProvider := build.NewPhaseConfigProvider(phaseName, lifecycle)
			phase := phaseFactory.New(configProvider)
			assertRunSucceeds(t, phase, &outBuf, &errBuf)
			h.AssertContains(t, outBuf.String(), "running some-lifecycle-phase")

			h.AssertNil(t, lifecycle.Cleanup())
		})

		it("should delete the layers volume", func() {
			body, err := docker.VolumeList(context.TODO(),
				filters.NewArgs(filters.KeyValuePair{
					Key:   "name",
					Value: lifecycle.LayersVolume(),
				}))
			h.AssertNil(t, err)
			h.AssertEq(t, len(body.Volumes), 0)
		})

		it("should delete the app volume", func() {
			body, err := docker.VolumeList(context.TODO(),
				filters.NewArgs(filters.KeyValuePair{
					Key:   "name",
					Value: lifecycle.AppVolume(),
				}))
			h.AssertNil(t, err)
			h.AssertEq(t, len(body.Volumes), 0)
		})
	})
}

func assertAppModTimePreserved(t *testing.T, lifecycle *build.Lifecycle, phaseFactory *build.DefaultPhaseFactory, outBuf *bytes.Buffer, errBuf *bytes.Buffer) {
	t.Helper()
	readPhase := phaseFactory.New(build.NewPhaseConfigProvider(
		phaseName,
		lifecycle,
		build.WithArgs("read", "/workspace/fake-app-file"),
		build.WithContainerOperations(
			build.CopyDir(lifecycle.AppPath(), "/workspace", 0, 0, nil),
		),
	))
	assertRunSucceeds(t, readPhase, outBuf, errBuf)

	matches := regexp.MustCompile(regexp.QuoteMeta("file mod time (unix): ") + "(.*)").FindStringSubmatch(outBuf.String())
	h.AssertEq(t, len(matches), 2)
	h.AssertFalse(t, matches[1] == strconv.FormatInt(archive.NormalizedDateTime.Unix(), 10))
}

func assertRunSucceeds(t *testing.T, phase build.RunnerCleaner, outBuf *bytes.Buffer, errBuf *bytes.Buffer) {
	t.Helper()
	if err := phase.Run(context.TODO()); err != nil {
		phase.Cleanup()
		t.Fatalf("Failed to run phase: %s\nstdout:\n%s\nstderr:\n%s\n", err, outBuf.String(), errBuf.String())
	}
	phase.Cleanup()
}

func CreateFakeLifecycle(docker client.CommonAPIClient, logger logging.Logger, appDir string, repoName string) (*build.Lifecycle, error) {
	subject := build.NewLifecycle(docker, logger)
	builderImage, err := local.NewImage(repoName, docker, local.FromBaseImage(repoName))
	if err != nil {
		return nil, err
	}

	bldr, err := builder.FromImage(builderImage)
	if err != nil {
		return nil, err
	}

	subject.Setup(build.LifecycleOptions{
		AppPath:    appDir,
		Builder:    bldr,
		HTTPProxy:  "some-http-proxy",
		HTTPSProxy: "some-https-proxy",
		NoProxy:    "some-no-proxy",
	})
	return subject, nil
}
