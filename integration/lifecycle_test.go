package integration_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

var registryPort string

func TestLifecycle(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())

	registryPort = h.RunRegistry(t, true)
	defer h.StopRegistry(t)
	packHome, err := ioutil.TempDir("", "build-test-pack-home")
	h.AssertNil(t, err)
	defer os.RemoveAll(packHome)
	h.ConfigurePackHome(t, packHome, registryPort)
	defer h.CleanDefaultImages(t, registryPort)

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
	)

	it.Before(func() {
		var err error

		ctx = context.TODO()
		logger = logging.NewLogger(&outBuf, &errBuf, true, false)
		dockerCli, err = docker.New()
		h.AssertNil(t, err)
		repoName := "pack.build." + h.RandString(10)
		buildCache, err := cache.New(repoName, dockerCli)
		defaultBuilderName = h.DefaultBuilderImage(t, registryPort)
		subject = &pack.BuildConfig{
			AppDir:   "../acceptance/testdata/node_app",
			Builder:  defaultBuilderName,
			RunImage: h.DefaultRunImage(t, registryPort),
			RepoName: repoName,
			Publish:  false,
			Cache:    buildCache,
			Logger:   logger,
			FS:       &fs.FS{},
			Cli:      dockerCli,
		}
	})
	it.After(func() {
		for _, volName := range []string{subject.Cache.Volume(), subject.Cache.Volume()} {
			dockerCli.VolumeRemove(context.TODO(), volName, true)
		}
	})

	when.Pend("#Detect", func() {
		it("copies the app in to docker and chowns it (including directories)", func() {
			h.AssertNil(t, subject.Detect(ctx))

			for _, name := range []string{"/workspace/app", "/workspace/app/app.js", "/workspace/app/mydir", "/workspace/app/mydir/myfile.txt"} {
				txt := h.RunInImage(t, dockerCli, []string{subject.Cache.Volume() + ":/workspace"}, subject.Builder, "ls", "-ld", name)
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
				h.AssertError(t, subject.Detect(ctx), "run detect container: failed with status code: 6")
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

					h.AssertNil(t, subject.Detect(ctx))

					h.AssertContains(t, outBuf.String(), `My Sample Buildpack: pass`)
				})
			})
			when("id@version buildpack", func() {
				it("symlinks directories to workspace and sets order.toml", func() {
					subject.Buildpacks = []string{
						"io.buildpacks.samples.nodejs@latest",
					}

					h.AssertNil(t, subject.Detect(ctx))

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
				subject.Buildpacks = []string{"../acceptance/testdata/mock_buildpacks/printenv"}
				h.AssertNil(t, subject.Detect(ctx))
				h.AssertContains(t, outBuf.String(), "DETECT: VAR1 is value1;")
				h.AssertContains(t, outBuf.String(), "DETECT: VAR2 is value2 with spaces;")
			})
		})
	})
}
