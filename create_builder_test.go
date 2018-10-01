package pack_test

import (
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/mocks"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"io/ioutil"
	"log"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCreateBuilder(t *testing.T) {
	spec.Run(t, "create-builder", testCreateBuilder, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testCreateBuilder(t *testing.T, when spec.G, it spec.S) {
	when("#BuilderConfigFromFlags", func() {
		var (
			mockController *gomock.Controller
			mockDocker     *mocks.MockDocker
			factory        pack.BuilderFactory
		)
		it.Before(func() {
			mockController = gomock.NewController(t)
			mockDocker = mocks.NewMockDocker(mockController)
			factory = pack.BuilderFactory{
				Docker: mockDocker,
				Log:    log.New(ioutil.Discard, "", log.LstdFlags),
			}

			output, err := exec.Command("docker", "pull", "packs/build").CombinedOutput()
			if err != nil {
				t.Fatalf("Failed to pull the base image in test setup: %s: %s", output, err)
			}
		})

		it.After(func() {
			mockController.Finish()
		})

		it("uses stack build image as base image", func() {
			mockDocker.EXPECT().PullImage("packs/build")

			config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
				RepoName:        "some/image",
				BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
			})
			if err != nil {
				t.Fatalf("error creating builder config: %s", err)
			}
			assertEq(t, reflect.TypeOf(config.BaseImage).String(), "*daemon.image")
			assertEq(t, reflect.TypeOf(config.Repo).String(), "*img.daemonStore")
			checkBuildpacks(t, config.Buildpacks)
			checkGroups(t, config.Groups)
		})

		when("the daemon has the base image", func() {
			it("doesn't pull base a new image when --no-pull flag is provided", func() {
				config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
					NoPull:          true,
				})
				if err != nil {
					t.Fatalf("error creating builder config: %s", err)
				}
				if config.Repo == nil {
					t.Fatalf("failed to set repository: %s", err)
				}
				assertEq(t, reflect.TypeOf(config.BaseImage).String(), "*daemon.image")
				assertEq(t, reflect.TypeOf(config.Repo).String(), "*img.daemonStore")
				checkBuildpacks(t, config.Buildpacks)
				checkGroups(t, config.Groups)
			})
		})

		when("the daemon does not have the base image", func() {
			it.Before(func() {
				exec.Command("docker", "rmi", "packs/build").CombinedOutput()
			})

			it("fails if --no-pull is provided", func() {
				_, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
					RepoName:        "some/image",
					BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
					NoPull:          true,
				})
				if err == nil {
					t.Fatalf("Expected error when base image is missing from daemon")
				}
			})
		})

		it("doesn't pull base image when --publish flag is provided", func() {
			config, err := factory.BuilderConfigFromFlags(pack.CreateBuilderFlags{
				RepoName:        "some/image",
				BuilderTomlPath: filepath.Join("testdata", "builder.toml"),
				Publish:         true,
			})
			if err != nil {
				t.Fatalf("error creating builder config: %s", err)
			}
			assertEq(t, reflect.TypeOf(config.BaseImage).String(), "*remote.mountableImage")
			assertEq(t, reflect.TypeOf(config.Repo).String(), "*img.registryStore")
			checkBuildpacks(t, config.Buildpacks)
			checkGroups(t, config.Groups)
		})
	})
}

func checkGroups(t *testing.T, groups []lifecycle.BuildpackGroup) {
	t.Helper()
	if diff := cmp.Diff(groups, []lifecycle.BuildpackGroup{
		{Buildpacks: []*lifecycle.Buildpack{
			{
				ID:      "com.example.sample.bp1",
				Version: "1.2.3",
			},
			{
				ID:      "com.example.sample.bp2",
				Version: "1.2.4",
			},
		}},
		{Buildpacks: []*lifecycle.Buildpack{
			{
				ID:      "com.example.sample.bp1",
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
			ID:  "com.example.sample.bp1",
			URI: "file://testdata/buildpacks/sample_bp1",
		},
		{
			ID:  "com.example.sample.bp2",
			URI: "file://testdata/buildpacks/sample_bp2",
		},
	}); diff != "" {
		t.Fatalf("config has incorrect buildpacks, %s", diff)
	}
}
