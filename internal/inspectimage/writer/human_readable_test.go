package writer_test

import (
	"bytes"
	"github.com/buildpacks/pack/internal/config"
	"testing"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/inspectimage/writer"
	ilogging "github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestHumanReadable(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Human Readable Writer", testHumanReadable, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testHumanReadable(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = h.NewAssertionManager(t)
		outBuf bytes.Buffer

		remoteInfo *pack.ImageInfo
		localInfo  *pack.ImageInfo

		expectedRemoteOutput = `REMOTE:

Stack: test.stack.id.remote

Base Image:
  Reference: some-remote-run-image-reference
  Top Layer: some-remote-top-layer

Run Images:
  user-configured-mirror-for-remote        (user-configured)
  some-remote-run-image
  some-remote-mirror
  other-remote-mirror

Buildpacks:
  ID                        VERSION
  test.bp.one.remote        1.0.0
  test.bp.two.remote        2.0.0

Processes:
  TYPE                              SHELL        COMMAND                      ARGS
  some-remote-type (default)        bash         /some/remote command         some remote args
  other-remote-type                              /other/remote/command        other remote args`

		expectedLocalOutput = `LOCAL:

Stack: test.stack.id.local

Base Image:
  Reference: some-local-run-image-reference
  Top Layer: some-local-top-layer

Run Images:
  user-configured-mirror-for-local        (user-configured)
  some-local-run-image
  some-local-mirror
  other-local-mirror

Buildpacks:
  ID                       VERSION
  test.bp.one.local        1.0.0
  test.bp.two.local        2.0.0

Processes:
  TYPE                             SHELL        COMMAND                     ARGS
  some-local-type (default)        bash         /some/local command         some local args
  other-local-type                              /other/local/command        other local args`
	)

	when("Print", func() {
		it.Before(func() {
			type someData struct {
				String string
				Bool   bool
				Int    int
				Nested struct {
					String string
				}
			}

			remoteInfo = &pack.ImageInfo{
				StackID: "test.stack.id.remote",
				Buildpacks: []lifecycle.Buildpack{
					{ID: "test.bp.one.remote", Version: "1.0.0"},
					{ID: "test.bp.two.remote", Version: "2.0.0"},
				},
				Base: lifecycle.RunImageMetadata{
					TopLayer:  "some-remote-top-layer",
					Reference: "some-remote-run-image-reference",
				},
				Stack: lifecycle.StackMetadata{
					RunImage: lifecycle.StackRunImageMetadata{
						Image:   "some-remote-run-image",
						Mirrors: []string{"some-remote-mirror", "other-remote-mirror"},
					},
				},
				BOM: []lifecycle.BOMEntry{{
					Require: lifecycle.Require{
						Name:    "name-1",
						Version: "version-1",
						Metadata: map[string]interface{}{
							"RemoteData": someData{
								String: "aString",
								Bool:   true,
								Int:    123,
								Nested: struct {
									String string
								}{
									String: "anotherString",
								},
							},
						},
					},
					Buildpack: lifecycle.Buildpack{ID: "test.bp.one.remote", Version: "1.0.0"},
				}},
				Processes: pack.ProcessDetails{
					DefaultProcess: &launch.Process{
						Type:    "some-remote-type",
						Command: "/some/remote command",
						Args:    []string{"some", "remote", "args"},
						Direct:  false,
					},
					OtherProcesses: []launch.Process{
						{
							Type:    "other-remote-type",
							Command: "/other/remote/command",
							Args:    []string{"other", "remote", "args"},
							Direct:  true,
						},
					},
				},
			}

			localInfo = &pack.ImageInfo{
				StackID: "test.stack.id.local",
				Buildpacks: []lifecycle.Buildpack{
					{ID: "test.bp.one.local", Version: "1.0.0"},
					{ID: "test.bp.two.local", Version: "2.0.0"},
				},
				Base: lifecycle.RunImageMetadata{
					TopLayer:  "some-local-top-layer",
					Reference: "some-local-run-image-reference",
				},
				Stack: lifecycle.StackMetadata{
					RunImage: lifecycle.StackRunImageMetadata{
						Image:   "some-local-run-image",
						Mirrors: []string{"some-local-mirror", "other-local-mirror"},
					},
				},
				BOM: []lifecycle.BOMEntry{{
					Require: lifecycle.Require{
						Name:    "name-1",
						Version: "version-1",
						Metadata: map[string]interface{}{
							"LocalData": someData{
								Bool: false,
								Int:  456,
							},
						},
					},
					Buildpack: lifecycle.Buildpack{ID: "test.bp.one.remote", Version: "1.0.0"},
				}},
				Processes: pack.ProcessDetails{
					DefaultProcess: &launch.Process{
						Type:    "some-local-type",
						Command: "/some/local command",
						Args:    []string{"some", "local", "args"},
						Direct:  false,
					},
					OtherProcesses: []launch.Process{
						{
							Type:    "other-local-type",
							Command: "/other/local/command",
							Args:    []string{"other", "local", "args"},
							Direct:  true,
						},
					},
				},
			}

			outBuf = bytes.Buffer{}
		})

		when("local and remote image exits", func() {
			it("prints both local and remote image info in a human readable format", func() {
				runImageMirrors := []config.RunImage{
					{
						Image:   "un-used-run-image",
						Mirrors: []string{"un-used"},
					},
					{
						Image:   "some-local-run-image",
						Mirrors: []string{"user-configured-mirror-for-local"},
					},
					{
						Image:   "some-remote-run-image",
						Mirrors: []string{"user-configured-mirror-for-remote"},
					},
				}
				sharedImageInfo := &writer.SharedImageInfo{
					Name:            "test-image",
					RunImageMirrors: runImageMirrors,
				}
				humanReadableWriter := writer.NewHumanReadable()

				logger := ilogging.NewLogWithWriters(&outBuf, &outBuf)
				err := humanReadableWriter.Print(logger, sharedImageInfo, localInfo, remoteInfo, nil, nil)
				assert.Nil(err)

				assert.Contains(outBuf.String(), expectedLocalOutput)
				assert.Contains(outBuf.String(), expectedRemoteOutput)
			})
		})

		when("only local image exists", func() {
			it("prints local image info in a human readable format", func() {

				runImageMirrors := []config.RunImage{
					{
						Image:   "un-used-run-image",
						Mirrors: []string{"un-used"},
					},
					{
						Image:   "some-local-run-image",
						Mirrors: []string{"user-configured-mirror-for-local"},
					},
					{
						Image:   "some-remote-run-image",
						Mirrors: []string{"user-configured-mirror-for-remote"},
					},
				}
				sharedImageInfo := &writer.SharedImageInfo{
					Name:            "test-image",
					RunImageMirrors: runImageMirrors,
				}
				humanReadableWriter := writer.NewHumanReadable()

				logger := ilogging.NewLogWithWriters(&outBuf, &outBuf)
				err := humanReadableWriter.Print(logger, sharedImageInfo, localInfo, nil, nil, nil)
				assert.Nil(err)

				assert.Contains(outBuf.String(), expectedLocalOutput)
				assert.NotContains(outBuf.String(), expectedRemoteOutput)
			})
		})

		when("only remote image exists", func() {
			it("prints remote image info in a human readable format", func() {

				runImageMirrors := []config.RunImage{
					{
						Image:   "un-used-run-image",
						Mirrors: []string{"un-used"},
					},
					{
						Image:   "some-local-run-image",
						Mirrors: []string{"user-configured-mirror-for-local"},
					},
					{
						Image:   "some-remote-run-image",
						Mirrors: []string{"user-configured-mirror-for-remote"},
					},
				}
				sharedImageInfo := &writer.SharedImageInfo{
					Name:            "test-image",
					RunImageMirrors: runImageMirrors,
				}
				humanReadableWriter := writer.NewHumanReadable()

				logger := ilogging.NewLogWithWriters(&outBuf, &outBuf)
				err := humanReadableWriter.Print(logger, sharedImageInfo, nil, remoteInfo, nil, nil)
				assert.Nil(err)

				assert.NotContains(outBuf.String(), expectedLocalOutput)
				assert.Contains(outBuf.String(), expectedRemoteOutput)
			})
		})
	})
}
