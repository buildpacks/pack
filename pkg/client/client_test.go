package client

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/buildpacks/lifecycle/api"

	"github.com/golang/mock/gomock"
	dockerClient "github.com/moby/moby/client"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestClient(t *testing.T) {
	spec.Run(t, "Client", testClient, spec.Report(report.Terminal{}))
}

func testClient(t *testing.T, when spec.G, it spec.S) {
	when("#NewClient", func() {
		it("default works", func() {
			_, err := NewClient()
			h.AssertNil(t, err)
		})

		when("docker env is messed up", func() {
			var dockerHost string
			var dockerHostKey = "DOCKER_HOST"
			it.Before(func() {
				dockerHost = os.Getenv(dockerHostKey)
				h.AssertNil(t, os.Setenv(dockerHostKey, "fake-value"))
			})

			it.After(func() {
				h.AssertNil(t, os.Setenv(dockerHostKey, dockerHost))
			})

			it("returns errors", func() {
				_, err := NewClient()
				h.AssertError(t, err, "docker client")
			})
		})
	})

	when("#WithLogger", func() {
		it("uses logger provided", func() {
			var w bytes.Buffer
			logger := logging.NewSimpleLogger(&w)
			cl, err := NewClient(WithLogger(logger))
			h.AssertNil(t, err)
			h.AssertSameInstance(t, cl.logger, logger)
		})
	})

	when("#WithImageFactory", func() {
		it("uses image factory provided", func() {
			mockController := gomock.NewController(t)
			mockImageFactory := testmocks.NewMockImageFactory(mockController)
			cl, err := NewClient(WithImageFactory(mockImageFactory))
			h.AssertNil(t, err)
			h.AssertSameInstance(t, cl.imageFactory, mockImageFactory)
		})
	})

	when("#WithFetcher", func() {
		it("uses image factory provided", func() {
			mockController := gomock.NewController(t)
			mockFetcher := testmocks.NewMockImageFetcher(mockController)
			cl, err := NewClient(WithFetcher(mockFetcher))
			h.AssertNil(t, err)
			h.AssertSameInstance(t, cl.imageFetcher, mockFetcher)
		})
	})

	when("#WithDownloader", func() {
		it("uses image factory provided", func() {
			mockController := gomock.NewController(t)
			mockDownloader := testmocks.NewMockBlobDownloader(mockController)
			cl, err := NewClient(WithDownloader(mockDownloader))
			h.AssertNil(t, err)
			h.AssertSameInstance(t, cl.downloader, mockDownloader)
		})
	})

	when("#WithDockerClient", func() {
		it("uses docker client provided", func() {
			docker, err := dockerClient.NewClientWithOpts(
				dockerClient.FromEnv,
			)
			h.AssertNil(t, err)
			cl, err := NewClient(WithDockerClient(docker))
			h.AssertNil(t, err)
			h.AssertSameInstance(t, cl.docker, docker)
		})
	})

	when("#WithExperimental", func() {
		it("sets experimental = true", func() {
			cl, err := NewClient(WithExperimental(true))
			h.AssertNil(t, err)
			h.AssertEq(t, cl.experimental, true)
		})

		it("sets experimental = false", func() {
			cl, err := NewClient(WithExperimental(true))
			h.AssertNil(t, err)
			h.AssertEq(t, cl.experimental, true)
		})
	})

	when("#WithRegistryMirror", func() {
		it("uses registry mirrors provided", func() {
			registryMirrors := map[string]string{
				"index.docker.io": "10.0.0.1",
			}

			cl, err := NewClient(WithRegistryMirrors(registryMirrors))
			h.AssertNil(t, err)
			h.AssertEq(t, cl.registryMirrors, registryMirrors)
		})
	})

	when("#processSystem", func() {
		var (
			subject          *Client
			mockController   *gomock.Controller
			availableBPs     []buildpack.BuildModule
			systemBuildpacks dist.System
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			subject = &Client{}

			// Create mock buildpack modules
			availableBPs = []buildpack.BuildModule{
				&mockBuildModule{id: "example/pre-bp", version: "1.0.0"},
				&mockBuildModule{id: "example/post-bp", version: "2.0.0"},
				&mockBuildModule{id: "example/optional-bp", version: "3.0.0"},
			}
		})

		it.After(func() {
			mockController.Finish()
		})

		when("disableSystem is true", func() {
			it("returns empty system", func() {
				systemBuildpacks = dist.System{
					Pre: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "example/pre-bp", Version: "1.0.0"}, Optional: false},
						},
					},
				}

				result, err := subject.processSystem(systemBuildpacks, availableBPs, true)
				h.AssertNil(t, err)
				h.AssertEq(t, len(result.Pre.Buildpacks), 0)
				h.AssertEq(t, len(result.Post.Buildpacks), 0)
			})
		})

		when("no buildpacks are available", func() {
			it("returns the original system", func() {
				systemBuildpacks = dist.System{
					Pre: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "example/pre-bp", Version: "1.0.0"}, Optional: false},
						},
					},
				}

				result, err := subject.processSystem(systemBuildpacks, []buildpack.BuildModule{}, false)
				h.AssertNil(t, err)
				h.AssertEq(t, result, systemBuildpacks)
			})
		})

		when("all required system buildpacks are available", func() {
			it("returns resolved system with all buildpacks", func() {
				systemBuildpacks = dist.System{
					Pre: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "example/pre-bp", Version: "1.0.0"}, Optional: false},
						},
					},
					Post: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "example/post-bp", Version: "2.0.0"}, Optional: false},
						},
					},
				}

				result, err := subject.processSystem(systemBuildpacks, availableBPs, false)
				h.AssertNil(t, err)
				h.AssertEq(t, len(result.Pre.Buildpacks), 1)
				h.AssertEq(t, result.Pre.Buildpacks[0].ID, "example/pre-bp")
				h.AssertEq(t, len(result.Post.Buildpacks), 1)
				h.AssertEq(t, result.Post.Buildpacks[0].ID, "example/post-bp")
			})
		})

		when("required system buildpack is missing", func() {
			it("returns an error for missing pre-buildpack", func() {
				systemBuildpacks = dist.System{
					Pre: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "missing/pre-bp", Version: "1.0.0"}, Optional: false},
						},
					},
				}

				_, err := subject.processSystem(systemBuildpacks, availableBPs, false)
				h.AssertError(t, err, "required system buildpack missing/pre-bp@1.0.0 is not available")
			})

			it("returns an error for missing post-buildpack", func() {
				systemBuildpacks = dist.System{
					Post: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "missing/post-bp", Version: "1.0.0"}, Optional: false},
						},
					},
				}

				_, err := subject.processSystem(systemBuildpacks, availableBPs, false)
				h.AssertError(t, err, "required system buildpack missing/post-bp@1.0.0 is not available")
			})
		})

		when("optional system buildpack is missing", func() {
			it("ignores missing optional pre-buildpack", func() {
				systemBuildpacks = dist.System{
					Pre: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "example/pre-bp", Version: "1.0.0"}, Optional: false},
							{ModuleInfo: dist.ModuleInfo{ID: "missing/optional-bp", Version: "1.0.0"}, Optional: true},
						},
					},
				}

				result, err := subject.processSystem(systemBuildpacks, availableBPs, false)
				h.AssertNil(t, err)
				h.AssertEq(t, len(result.Pre.Buildpacks), 1)
				h.AssertEq(t, result.Pre.Buildpacks[0].ID, "example/pre-bp")
			})

			it("ignores missing optional post-buildpack", func() {
				systemBuildpacks = dist.System{
					Post: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "example/post-bp", Version: "2.0.0"}, Optional: false},
							{ModuleInfo: dist.ModuleInfo{ID: "missing/optional-bp", Version: "1.0.0"}, Optional: true},
						},
					},
				}

				result, err := subject.processSystem(systemBuildpacks, availableBPs, false)
				h.AssertNil(t, err)
				h.AssertEq(t, len(result.Post.Buildpacks), 1)
				h.AssertEq(t, result.Post.Buildpacks[0].ID, "example/post-bp")
			})
		})

		when("mix of available and missing buildpacks", func() {
			it("includes available buildpacks and reports error for required missing ones", func() {
				systemBuildpacks = dist.System{
					Pre: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "example/pre-bp", Version: "1.0.0"}, Optional: false},
							{ModuleInfo: dist.ModuleInfo{ID: "missing/required-bp", Version: "1.0.0"}, Optional: false},
						},
					},
					Post: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "example/post-bp", Version: "2.0.0"}, Optional: true},
							{ModuleInfo: dist.ModuleInfo{ID: "missing/optional-bp", Version: "1.0.0"}, Optional: true},
						},
					},
				}

				_, err := subject.processSystem(systemBuildpacks, availableBPs, false)
				h.AssertError(t, err, "required system buildpack missing/required-bp@1.0.0 is not available")
			})
		})

		when("buildpack version mismatch", func() {
			it("requires exact version match", func() {
				systemBuildpacks = dist.System{
					Pre: dist.SystemBuildpacks{
						Buildpacks: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "example/pre-bp", Version: "2.0.0"}, Optional: false}, // wrong version
						},
					},
				}

				_, err := subject.processSystem(systemBuildpacks, availableBPs, false)
				h.AssertError(t, err, "required system buildpack example/pre-bp@2.0.0 is not available")
			})
		})
	})
}

// Mock implementations for testing purpuse

// mockDescriptor is a mock implementation of buildpack.Descriptor
type mockDescriptor struct {
	info dist.ModuleInfo
}

func (m *mockDescriptor) API() *api.Version {
	return nil
}

func (m *mockDescriptor) EnsureStackSupport(stackID string, providedMixins []string, validateRunStageMixins bool) error {
	return nil
}

func (m *mockDescriptor) EnsureTargetSupport(os, arch, distroName, distroVersion string) error {
	return nil
}

func (m *mockDescriptor) EscapedID() string {
	return m.info.ID
}

func (m *mockDescriptor) Info() dist.ModuleInfo {
	return m.info
}

func (m *mockDescriptor) Kind() string {
	return buildpack.KindBuildpack
}

func (m *mockDescriptor) Order() dist.Order {
	return nil
}

func (m *mockDescriptor) Stacks() []dist.Stack {
	return nil
}

func (m *mockDescriptor) Targets() []dist.Target {
	return nil
}

// mockBuildModule is a mock implementation of buildpack.BuildModule for testing
type mockBuildModule struct {
	id      string
	version string
}

func (m *mockBuildModule) Descriptor() buildpack.Descriptor {
	return &mockDescriptor{
		info: dist.ModuleInfo{
			ID:      m.id,
			Version: m.version,
		},
	}
}

func (m *mockBuildModule) Open() (io.ReadCloser, error) {
	return nil, nil
}
