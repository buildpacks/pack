package writer_test

import (
"bytes"
"github.com/buildpacks/lifecycle"
"github.com/buildpacks/lifecycle/launch"
"github.com/buildpacks/pack"
"github.com/buildpacks/pack/internal/config"
"github.com/buildpacks/pack/internal/inspectimage"
"github.com/buildpacks/pack/internal/inspectimage/writer"
ilogging "github.com/buildpacks/pack/internal/logging"
h "github.com/buildpacks/pack/testhelpers"
"github.com/heroku/color"
"github.com/sclevine/spec"
"github.com/sclevine/spec/report"
"testing"
)

func TestYAML(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "YAML Writer", testYAML, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testYAML(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = h.NewAssertionManager(t)
		outBuf bytes.Buffer

		remoteInfo *pack.ImageInfo
		localInfo  *pack.ImageInfo

		expectedLocalOutput = `---
local_info:
  stack: test.stack.id.local
  base_image:
    topLayer: some-local-top-layer
    reference: some-local-run-image-reference
  run_images:
  - name: user-configured-mirror-for-local
    user_configured: true
  - name: some-local-run-image
  - name: some-local-mirror
  - name: other-local-mirror
  buildpacks:
  - id: test.bp.one.local
    version: 1.0.0
  - id: test.bp.two.local
    version: 2.0.0
  processes:
  - type: some-local-type
    shell: bash
    command: "/some/local command"
    default: true
    args:
    - some
    - local
    - args
  - type: other-local-type
    shell: ''
    command: "/other/local/command"
    default: false
    args:
    - other
    - local
    - args
`
		expectedRemoteOutput = `---
remote_info:
  stack: test.stack.id.remote
  base_image:
    topLayer: some-remote-top-layer
    reference: some-remote-run-image-reference
  run_images:
  - name: user-configured-mirror-for-remote
    user_configured: true
  - name: some-remote-run-image
  - name: some-remote-mirror
  - name: other-remote-mirror
  buildpacks:
  - id: test.bp.one.remote
    version: 1.0.0
  - id: test.bp.two.remote
    version: 2.0.0
  processes:
  - type: some-remote-type
    shell: bash
    command: "/some/remote command"
    default: true
    args:
    - some
    - remote
    - args
  - type: other-remote-type
    shell: ''
    command: "/other/remote/command"
    default: false
    args:
    - other
    - remote
    - args
`
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
			it("prints both local and remote image info in a YAML format", func() {
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
				sharedImageInfo := inspectimage.GeneralInfo{
					Name:            "test-image",
					RunImageMirrors: runImageMirrors,
				}
				yamlWriter := writer.NewYAML()

				logger := ilogging.NewLogWithWriters(&outBuf, &outBuf)
				err := yamlWriter.Print(logger, sharedImageInfo, localInfo, remoteInfo, nil, nil)
				assert.Nil(err)

				assert.ContainsYAML(outBuf.String(), `"image_name": "test-image"`)
				assert.ContainsYAML(outBuf.String(), expectedLocalOutput)
				assert.ContainsYAML(outBuf.String(), expectedRemoteOutput)
			})
		})

		when("only local image exists", func() {
			it("prints local image info in YAML format", func() {
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
				sharedImageInfo := inspectimage.GeneralInfo{
					Name:            "test-image",
					RunImageMirrors: runImageMirrors,
				}
				yamlWriter := writer.NewYAML()

				logger := ilogging.NewLogWithWriters(&outBuf, &outBuf)
				err := yamlWriter.Print(logger, sharedImageInfo, localInfo, nil, nil, nil)
				assert.Nil(err)

				assert.ContainsYAML(outBuf.String(), `"image_name": "test-image"`)
				assert.ContainsYAML(outBuf.String(), expectedLocalOutput)

				// How do we handle non-existance??
				// TODO: lets develop a better method for this comparision
				assert.NotContains(outBuf.String(), "test.stack.id.remote")
				assert.ContainsYAML(outBuf.String(), expectedLocalOutput)
			})
		})

		when("only remote image exists", func() {
			it("prints remote image info in YAML format", func() {
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
				sharedImageInfo := inspectimage.GeneralInfo{
					Name:            "test-image",
					RunImageMirrors: runImageMirrors,
				}
				yamlWriter := writer.NewYAML()

				logger := ilogging.NewLogWithWriters(&outBuf, &outBuf)
				err := yamlWriter.Print(logger, sharedImageInfo, nil, remoteInfo, nil, nil)
				assert.Nil(err)

				assert.ContainsYAML(outBuf.String(), `"image_name": "test-image"`)
				assert.NotContains(outBuf.String(), "test.stack.id.local")
				assert.ContainsYAML(outBuf.String(), expectedRemoteOutput)
			})
		})
	})


}
