package pack

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/buildpacks/imgutil/fakes"

	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestRegisterBuildpack(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "rebase_factory", testRegisterBuildpack, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRegisterBuildpack(t *testing.T, when spec.G, it spec.S) {
	when("#RegisterBuildpack", func() {
		var (
			fakeImageFetcher *ifakes.FakeImageFetcher
			fakeAppImage     *fakes.Image
			subject          *Client
			out              bytes.Buffer
		)

		it.Before(func() {
			fakeImageFetcher = ifakes.NewFakeImageFetcher()
			fakeAppImage = fakes.NewImage("buildpack/image", "", &fakeIdentifier{name: "buildpack-image"})

			h.AssertNil(t, fakeAppImage.SetLabel("io.buildpacks.buildpackage.metadata",
				`{"id":"heroku/java-function","version":"1.1.1","stacks":[{"id":"heroku-18"},{"id":"io.buildpacks.stacks.bionic"},{"id":"org.cloudfoundry.stacks.cflinuxfs3"}]}`))
			fakeImageFetcher.RemoteImages["buildpack/image"] = fakeAppImage

			fakeLogger := logging.NewLogWithWriters(&out, &out)
			subject = &Client{
				logger:       fakeLogger,
				imageFetcher: fakeImageFetcher,
			}
		})

		it.After(func() {
			_ = fakeAppImage.Cleanup()
		})

		it("should return error for an invalid image", func() {
			fakeAppImage = fakes.NewImage("invalid/image", "", &fakeIdentifier{name: "buildpack-image"})
			h.AssertNil(t, fakeAppImage.SetLabel("io.buildpacks.buildpackage.metadata", `{}`))

			h.AssertNotNil(t, subject.RegisterBuildpack(context.TODO(),
				RegisterBuildpackOptions{
					ImageName: "invalid/image",
					Type:      "github",
					URL:       "https://github.com/jkutner/buildpack-registry",
				}))
		})

		it("should return error for missing image label", func() {
			fakeAppImage = fakes.NewImage("misslinglabel/image", "", &fakeIdentifier{name: "buildpack-image"})
			h.AssertNil(t, fakeAppImage.SetLabel("io.buildpacks.buildpackage.metadata", `{}`))
			fakeImageFetcher.RemoteImages["missinglabel/image"] = fakeAppImage

			h.AssertNotNil(t, subject.RegisterBuildpack(context.TODO(),
				RegisterBuildpackOptions{
					ImageName: "missinglabel/image",
					Type:      "github",
					URL:       "https://github.com/jkutner/buildpack-registry",
				}))
		})

		when("registry type is github", func() {
			it("should open a github issue in the browser", func() {
				const expectedURL = `https://github.com/jkutner/buildpack-registry/issues/new?body=%0A%23%23%23+Data%0A%0A%60%60%60toml%0Aid+%3D+%22heroku%2Fjava-function%22%0Aversion+%3D+%221.1.1%22%0Aaddr+%3D+%22buildpack-image%22%0A%60%60%60&title=ADD+heroku%2Fjava-function%401.1.1`

				switch runtime.GOOS {
				case "linux":
					execCommand = getFakeExecCommand(t, "xdg-open", expectedURL)
				case "windows":
					execCommand = getFakeExecCommand(t, "rundll32", "url.dll,FileProtocolHandler", expectedURL)
				case "darwin":
					execCommand = getFakeExecCommand(t, "open", expectedURL)
				default:
					// do nothing
				}

				h.AssertNil(t, subject.RegisterBuildpack(context.TODO(),
					RegisterBuildpackOptions{
						ImageName: "buildpack/image",
						Type:      "github",
						URL:       "https://github.com/jkutner/buildpack-registry",
					}))
			})

			it("should throw error if missing URL", func() {
				h.AssertError(t, subject.RegisterBuildpack(context.TODO(),
					RegisterBuildpackOptions{
						ImageName: "buildpack/image",
						Type:      "github",
						URL:       "",
					}), "missing github URL")
			})
		})
	})
}

func getFakeExecCommand(t *testing.T, expectedCmd string, expectedArgs ...string) func(command string, args ...string) *exec.Cmd {
	return func(command string, args ...string) *exec.Cmd {
		h.AssertEq(t, command, expectedCmd)
		for i, arg := range args {
			h.AssertEq(t, arg, expectedArgs[i])
		}

		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}

		return cmd
	}
}
