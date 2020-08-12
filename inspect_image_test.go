package pack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	"github.com/buildpacks/pack/internal/logging"

	"github.com/buildpacks/pack/config"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/image"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
)

func TestInspectImage(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "InspectImage", testInspectImage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testInspectImage(t *testing.T, when spec.G, it spec.S) {
	var (
		subject          *Client
		mockImageFetcher *testmocks.MockImageFetcher
		mockDockerClient *testmocks.MockCommonAPIClient
		mockController   *gomock.Controller
		fakeImage        *fakes.Image
		out              bytes.Buffer
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)
		mockDockerClient = testmocks.NewMockCommonAPIClient(mockController)

		var err error
		subject, err = NewClient(WithLogger(logging.NewLogWithWriters(&out, &out)), WithFetcher(mockImageFetcher), WithDockerClient(mockDockerClient))
		h.AssertNil(t, err)

		fakeImage = fakes.NewImage("some/image", "", nil)
		h.AssertNil(t, fakeImage.SetLabel("io.buildpacks.stack.id", "test.stack.id"))
		h.AssertNil(t, fakeImage.SetLabel(
			"io.buildpacks.lifecycle.metadata",
			`{
  "stack": {
    "runImage": {
      "image": "some-run-image",
      "mirrors": [
        "some-mirror",
        "other-mirror"
      ]
    }
  },
  "runImage": {
    "topLayer": "some-top-layer",
    "reference": "some-run-image-reference"
  }
}`,
		))
		h.AssertNil(t, fakeImage.SetLabel(
			"io.buildpacks.build.metadata",
			`{
  "bom": [
    {
      "name": "some-bom-element"
    }
  ],
  "buildpacks": [
    {
      "id": "some-buildpack",
      "version": "some-version"
    },
    {
      "id": "other-buildpack",
      "version": "other-version"
    }
  ],
  "processes": [
    {
      "type": "other-process",
      "command": "/other/process",
      "args": ["opt", "1"],
      "direct": true
    },
    {
      "type": "web",
      "command": "/start/web-process",
      "args": ["-p", "1234"],
      "direct": false
    }
  ],
  "launcher": {
    "version": "0.5.0"
  }
}`,
		))
	})

	it.After(func() {
		mockController.Finish()
	})

	when("the image exists", func() {
		for _, useDaemon := range []bool{true, false} {
			useDaemon := useDaemon
			when(fmt.Sprintf("daemon is %t", useDaemon), func() {
				it.Before(func() {
					if useDaemon {
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/image", true, config.PullNever).Return(fakeImage, nil)
					} else {
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/image", false, config.PullNever).Return(fakeImage, nil)
					}
				})

				it("returns the stack ID", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)
					h.AssertEq(t, info.StackID, "test.stack.id")
				})

				it("returns the stack", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)
					h.AssertEq(t, info.Stack,
						lifecycle.StackMetadata{
							RunImage: lifecycle.StackRunImageMetadata{
								Image: "some-run-image",
								Mirrors: []string{
									"some-mirror",
									"other-mirror",
								},
							},
						},
					)
				})

				it("returns the base image", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)
					h.AssertEq(t, info.Base,
						lifecycle.RunImageMetadata{
							TopLayer:  "some-top-layer",
							Reference: "some-run-image-reference",
						},
					)
				})

				it("returns the BOM", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)

					rawBOM, err := json.Marshal(info.BOM)
					h.AssertNil(t, err)
					h.AssertContains(t, string(rawBOM), `[{"name":"some-bom-element"`)
				})

				it("returns the buildpacks", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)

					h.AssertEq(t, len(info.Buildpacks), 2)
					h.AssertEq(t, info.Buildpacks[0].ID, "some-buildpack")
					h.AssertEq(t, info.Buildpacks[0].Version, "some-version")
					h.AssertEq(t, info.Buildpacks[1].ID, "other-buildpack")
					h.AssertEq(t, info.Buildpacks[1].Version, "other-version")
				})

				it("returns the processes setting the web process as default", func() {
					info, err := subject.InspectImage("some/image", useDaemon)
					h.AssertNil(t, err)

					h.AssertEq(t, info.Processes,
						ProcessDetails{
							DefaultProcess: &launch.Process{
								Type:    "web",
								Command: "/start/web-process",
								Args:    []string{"-p", "1234"},
								Direct:  false,
							},
							OtherProcesses: []launch.Process{
								{
									Type:    "other-process",
									Command: "/other/process",
									Args:    []string{"opt", "1"},
									Direct:  true,
								},
							},
						},
					)
				})

				when("Platform API < 0.4", func() {
					when("CNB_PROCESS_TYPE is set", func() {
						it.Before(func() {
							h.AssertNil(t, fakeImage.SetEnv("CNB_PROCESS_TYPE", "other-process"))
						})

						it("returns processes setting the correct default process", func() {
							info, err := subject.InspectImage("some/image", useDaemon)
							h.AssertNil(t, err)

							h.AssertEq(t, info.Processes,
								ProcessDetails{
									DefaultProcess: &launch.Process{
										Type:    "other-process",
										Command: "/other/process",
										Args:    []string{"opt", "1"},
										Direct:  true,
									},
									OtherProcesses: []launch.Process{
										{
											Type:    "web",
											Command: "/start/web-process",
											Args:    []string{"-p", "1234"},
											Direct:  false,
										},
									},
								},
							)
						})
					})

					when("CNB_PROCESS_TYPE is set, but doesn't match an existing process", func() {
						it.Before(func() {
							h.AssertNil(t, fakeImage.SetEnv("CNB_PROCESS_TYPE", "missing-process"))
						})

						it("returns a nil default process", func() {
							info, err := subject.InspectImage("some/image", useDaemon)
							h.AssertNil(t, err)

							h.AssertEq(t, info.Processes,
								ProcessDetails{
									DefaultProcess: nil,
									OtherProcesses: []launch.Process{
										{
											Type:    "other-process",
											Command: "/other/process",
											Args:    []string{"opt", "1"},
											Direct:  true,
										},
										{
											Type:    "web",
											Command: "/start/web-process",
											Args:    []string{"-p", "1234"},
											Direct:  false,
										},
									},
								},
							)
						})
					})

					it("returns a nil default process when CNB_PROCESS_TYPE is not set and there is no web process", func() {
						h.AssertNil(t, fakeImage.SetLabel(
							"io.buildpacks.build.metadata",
							`{
  "processes": [
    {
      "type": "other-process",
      "command": "/other/process",
      "args": ["opt", "1"],
      "direct": true
    }
  ]
}`,
						))

						info, err := subject.InspectImage("some/image", useDaemon)
						h.AssertNil(t, err)

						h.AssertEq(t, info.Processes,
							ProcessDetails{
								DefaultProcess: nil,
								OtherProcesses: []launch.Process{
									{
										Type:    "other-process",
										Command: "/other/process",
										Args:    []string{"opt", "1"},
										Direct:  true,
									},
								},
							},
						)
					})
				})

				when("Platform API >= 0.4", func() {
					it.Before(func() {
						h.AssertNil(t, fakeImage.SetEnv("CNB_PLATFORM_API", "0.4"))
					})

					when("CNB_PLATFORM_API set to bad value", func() {
						it("errors", func() {
							h.AssertNil(t, fakeImage.SetEnv("CNB_PLATFORM_API", "not-semver"))
							_, err := subject.InspectImage("some/image", useDaemon)
							h.AssertError(t, err, "parsing platform api version")
						})
					})

					when("docker can't inspect image", func() {
						it("errors", func() {
							mockDockerClient.EXPECT().ImageInspectWithRaw(gomock.Any(), fakeImage.Name()).
								Return(types.ImageInspect{}, nil, errors.New("some-error"))

							_, err := subject.InspectImage("some/image", useDaemon)
							h.AssertError(t, err, "reading image")
						})
					})

					when("ENTRYPOINT is empty", func() {
						it("sets nil default process", func() {
							mockDockerClient.EXPECT().ImageInspectWithRaw(gomock.Any(), fakeImage.Name()).
								Return(types.ImageInspect{Config: &container.Config{}}, nil, nil)

							info, err := subject.InspectImage("some/image", useDaemon)
							h.AssertNil(t, err)

							h.AssertEq(t, info.Processes,
								ProcessDetails{
									DefaultProcess: nil,
									OtherProcesses: []launch.Process{
										{
											Type:    "other-process",
											Command: "/other/process",
											Args:    []string{"opt", "1"},
											Direct:  true,
										},
										{
											Type:    "web",
											Command: "/start/web-process",
											Args:    []string{"-p", "1234"},
											Direct:  false,
										},
									},
								},
							)
						})
					})

					when("CNB_PROCESS_TYPE is set", func() {
						it.Before(func() {
							h.AssertNil(t, fakeImage.SetEnv("CNB_PROCESS_TYPE", "other-process"))

							mockDockerClient.EXPECT().ImageInspectWithRaw(gomock.Any(), fakeImage.Name()).
								Return(types.ImageInspect{Config: &container.Config{Entrypoint: []string{"/cnb/process/web"}}}, nil, nil)
						})

						it("ignores it and sets the correct default process", func() {
							info, err := subject.InspectImage("some/image", useDaemon)
							h.AssertNil(t, err)

							h.AssertEq(t, info.Processes,
								ProcessDetails{
									DefaultProcess: &launch.Process{
										Type:    "web",
										Command: "/start/web-process",
										Args:    []string{"-p", "1234"},
										Direct:  false,
									},
									OtherProcesses: []launch.Process{
										{
											Type:    "other-process",
											Command: "/other/process",
											Args:    []string{"opt", "1"},
											Direct:  true,
										},
									},
								},
							)
						})
					})

					when("ENTRYPOINT is set, but doesn't match an existing process", func() {
						it.Before(func() {
							mockDockerClient.EXPECT().ImageInspectWithRaw(gomock.Any(), fakeImage.Name()).
								Return(types.ImageInspect{Config: &container.Config{Entrypoint: []string{"/cnb/process/unknown-process"}}}, nil, nil)
						})

						it("returns nil default default process", func() {
							info, err := subject.InspectImage("some/image", useDaemon)
							h.AssertNil(t, err)

							h.AssertEq(t, info.Processes,
								ProcessDetails{
									DefaultProcess: nil,
									OtherProcesses: []launch.Process{
										{
											Type:    "other-process",
											Command: "/other/process",
											Args:    []string{"opt", "1"},
											Direct:  true,
										},
										{
											Type:    "web",
											Command: "/start/web-process",
											Args:    []string{"-p", "1234"},
											Direct:  false,
										},
									},
								},
							)
						})
					})

					when("ENTRYPOINT set to /cnb/lifecycle/launcher", func() {
						it("returns a nil default process", func() {
							mockDockerClient.EXPECT().ImageInspectWithRaw(gomock.Any(), fakeImage.Name()).
								Return(types.ImageInspect{Config: &container.Config{Entrypoint: []string{"/cnb/lifecycle/launcher"}}}, nil, nil)

							h.AssertNil(t, fakeImage.SetLabel(
								"io.buildpacks.build.metadata",
								`{
					 "processes": [
					   {
					     "type": "other-process",
					     "command": "/other/process",
					     "args": ["opt", "1"],
					     "direct": true
					   }
					 ]
					}`,
							))

							info, err := subject.InspectImage("some/image", useDaemon)
							h.AssertNil(t, err)

							h.AssertEq(t, info.Processes,
								ProcessDetails{
									DefaultProcess: nil,
									OtherProcesses: []launch.Process{
										{
											Type:    "other-process",
											Command: "/other/process",
											Args:    []string{"opt", "1"},
											Direct:  true,
										},
									},
								},
							)
						})
					})

					when("Inspecting Windows images", func() {
						when(`ENTRYPOINT set to c:\cnb\lifecycle\launcher.exe`, func() {
							it("sets default process to nil", func() {
								mockDockerClient.EXPECT().ImageInspectWithRaw(gomock.Any(), fakeImage.Name()).
									Return(types.ImageInspect{Config: &container.Config{Entrypoint: []string{`c:\cnb\lifecycle\launcher.exe`}}}, nil, nil)

								info, err := subject.InspectImage("some/image", useDaemon)
								h.AssertNil(t, err)

								h.AssertEq(t, info.Processes,
									ProcessDetails{
										DefaultProcess: nil,
										OtherProcesses: []launch.Process{
											{
												Type:    "other-process",
												Command: "/other/process",
												Args:    []string{"opt", "1"},
												Direct:  true,
											},
											{
												Type:    "web",
												Command: "/start/web-process",
												Args:    []string{"-p", "1234"},
												Direct:  false,
											},
										},
									},
								)
							})
						})

						when("ENTRYPOINT is set, but doesn't match an existing process", func() {
							it("sets default process to nil", func() {
								mockDockerClient.EXPECT().ImageInspectWithRaw(gomock.Any(), fakeImage.Name()).
									Return(types.ImageInspect{Config: &container.Config{Entrypoint: []string{`c:\cnb\process\unknown-process.exe`}}}, nil, nil)

								info, err := subject.InspectImage("some/image", useDaemon)
								h.AssertNil(t, err)

								h.AssertEq(t, info.Processes,
									ProcessDetails{
										DefaultProcess: nil,
										OtherProcesses: []launch.Process{
											{
												Type:    "other-process",
												Command: "/other/process",
												Args:    []string{"opt", "1"},
												Direct:  true,
											},
											{
												Type:    "web",
												Command: "/start/web-process",
												Args:    []string{"-p", "1234"},
												Direct:  false,
											},
										},
									},
								)
							})
						})

						when("ENTRYPOINT is set, and matches an existing process", func() {
							it("sets default process to defined process", func() {
								mockDockerClient.EXPECT().ImageInspectWithRaw(gomock.Any(), fakeImage.Name()).
									Return(types.ImageInspect{Config: &container.Config{Entrypoint: []string{`c:\cnb\process\other-process.exe`}}}, nil, nil)

								info, err := subject.InspectImage("some/image", useDaemon)
								h.AssertNil(t, err)

								h.AssertEq(t, info.Processes,
									ProcessDetails{
										DefaultProcess: &launch.Process{
											Type:    "other-process",
											Command: "/other/process",
											Args:    []string{"opt", "1"},
											Direct:  true,
										},
										OtherProcesses: []launch.Process{
											{
												Type:    "web",
												Command: "/start/web-process",
												Args:    []string{"-p", "1234"},
												Direct:  false,
											},
										},
									},
								)
							})
						})
					})
				})
			})
		}
	})

	when("the image doesn't exist", func() {
		it("returns nil", func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "not/some-image", true, config.PullNever).Return(nil, image.ErrNotFound)

			info, err := subject.InspectImage("not/some-image", true)
			h.AssertNil(t, err)
			h.AssertNil(t, info)
		})
	})

	when("there is an error fetching the image", func() {
		it("returns the error", func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "not/some-image", true, config.PullNever).Return(nil, errors.New("some-error"))

			_, err := subject.InspectImage("not/some-image", true)
			h.AssertError(t, err, "some-error")
		})
	})

	when("the image is missing labels", func() {
		it("returns empty data", func() {
			mockImageFetcher.EXPECT().
				Fetch(gomock.Any(), "missing/labels", true, config.PullNever).
				Return(fakes.NewImage("missing/labels", "", nil), nil)
			info, err := subject.InspectImage("missing/labels", true)
			h.AssertNil(t, err)
			h.AssertEq(t, info, &ImageInfo{})
		})
	})

	when("the image has malformed labels", func() {
		var badImage *fakes.Image

		it.Before(func() {
			badImage = fakes.NewImage("bad/image", "", nil)
			mockImageFetcher.EXPECT().
				Fetch(gomock.Any(), "bad/image", true, config.PullNever).
				Return(badImage, nil)
		})

		it("returns an error when layers md cannot parse", func() {
			h.AssertNil(t, badImage.SetLabel("io.buildpacks.lifecycle.metadata", "not   ----  json"))
			_, err := subject.InspectImage("bad/image", true)
			h.AssertError(t, err, "unmarshalling label 'io.buildpacks.lifecycle.metadata'")
		})

		it("returns an error when build md cannot parse", func() {
			h.AssertNil(t, badImage.SetLabel("io.buildpacks.build.metadata", "not   ----  json"))
			_, err := subject.InspectImage("bad/image", true)
			h.AssertError(t, err, "unmarshalling label 'io.buildpacks.build.metadata'")
		})
	})

	when("lifecycle version is 0.4.x or earlier", func() {
		it("includes an empty base image reference", func() {
			oldImage := fakes.NewImage("old/image", "", nil)
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "old/image", true, config.PullNever).Return(oldImage, nil)

			h.AssertNil(t, oldImage.SetLabel(
				"io.buildpacks.lifecycle.metadata",
				`{
  "runImage": {
    "topLayer": "some-top-layer",
    "reference": "some-run-image-reference"
  }
}`,
			))
			h.AssertNil(t, oldImage.SetLabel(
				"io.buildpacks.build.metadata",
				`{
  "launcher": {
    "version": "0.4.0"
  }
}`,
			))

			info, err := subject.InspectImage("old/image", true)
			h.AssertNil(t, err)
			h.AssertEq(t, info.Base,
				lifecycle.RunImageMetadata{
					TopLayer:  "some-top-layer",
					Reference: "",
				},
			)
		})
	})
}
