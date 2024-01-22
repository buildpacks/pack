package image_test

import (
	"bytes"
	"testing"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestChecker(t *testing.T) {
	spec.Run(t, "Checker", testChecker, spec.Report(report.Terminal{}))
}

func testChecker(t *testing.T, when spec.G, it spec.S) {
	when("#Check", func() {
		it("fails when checking dummy image", func() {
			buf := &bytes.Buffer{}

			keychain, err := auth.DefaultKeychain("pack.test/dummy")
			h.AssertNil(t, err)

			ic := image.NewAccessChecker(logging.NewSimpleLogger(buf), keychain)

			h.AssertFalse(t, ic.Check("pack.test/dummy"))
			h.AssertContains(t, buf.String(), "DEBUG:  CheckReadAccess failed for the run image pack.test/dummy")
		})
	})
}
