//go:build linux

package client

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/pack/testhelpers"
)

// processVolumes is only built for linux and windows (see process_volumes.go);
// other unix hosts use the docker/cli compose loader in process_volumes_unix.go.
// The parser is picked from the image OS and the host OS together, so these
// tests are pinned to linux to keep that selection deterministic.

func TestProcessVolumes(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "process_volumes", testProcessVolumes, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testProcessVolumes(t *testing.T, when spec.G, it spec.S) {
	when("#processVolumes", func() {
		when("the volume specification is valid", func() {
			it("keeps an explicit read-only mode", func() {
				processed, warnings, err := processVolumes("linux", []string{"/host:/container:ro"})
				h.AssertNil(t, err)
				h.AssertEq(t, processed, []string{"/host:/container:ro"})
				h.AssertEq(t, len(warnings), 0)
			})

			it("keeps an explicit read-write mode", func() {
				processed, warnings, err := processVolumes("linux", []string{"/host:/container:rw"})
				h.AssertNil(t, err)
				h.AssertEq(t, processed, []string{"/host:/container:rw"})
				h.AssertEq(t, len(warnings), 0)
			})

			// A spec with no mode parses as read-write, but pack deliberately
			// mounts it read-only; processMode turns the empty mode into "ro".
			it("defaults an unspecified mode to read-only", func() {
				processed, _, err := processVolumes("linux", []string{"/host:/container"})
				h.AssertNil(t, err)
				h.AssertEq(t, processed, []string{"/host:/container:ro"})
			})

			it("preserves propagation and relabel options", func() {
				processed, _, err := processVolumes("linux", []string{"/host:/container:ro,z,shared"})
				h.AssertNil(t, err)
				h.AssertEq(t, processed, []string{"/host:/container:ro,z,shared"})
			})

			it("processes several volumes in order", func() {
				processed, _, err := processVolumes("linux", []string{"/a:/one:ro", "/b:/two:rw"})
				h.AssertNil(t, err)
				h.AssertEq(t, processed, []string{"/a:/one:ro", "/b:/two:rw"})
			})

			it("accepts a named volume", func() {
				processed, _, err := processVolumes("linux", []string{"myvolume:/container:ro"})
				h.AssertNil(t, err)
				h.AssertEq(t, processed, []string{"myvolume:/container:ro"})
			})

			it("returns nothing for no volumes", func() {
				processed, warnings, err := processVolumes("linux", nil)
				h.AssertNil(t, err)
				h.AssertEq(t, len(processed), 0)
				h.AssertEq(t, len(warnings), 0)
			})
		})

		when("the target is a sensitive directory", func() {
			for _, dir := range []string{"/cnb", "/layers", "/workspace"} {
				it("warns when mounting to "+dir, func() {
					processed, warnings, err := processVolumes("linux", []string{"/host:" + dir + ":ro"})
					h.AssertNil(t, err)
					h.AssertEq(t, processed, []string{"/host:" + dir + ":ro"})
					h.AssertEq(t, len(warnings), 1)
					h.AssertContains(t, warnings[0], "Mounting to a sensitive directory")
					h.AssertContains(t, warnings[0], dir)
				})
			}

			it("warns for a path nested under a sensitive directory", func() {
				_, warnings, err := processVolumes("linux", []string{"/host:/cnb/nested/path:ro"})
				h.AssertNil(t, err)
				h.AssertEq(t, len(warnings), 1)
				h.AssertContains(t, warnings[0], "/cnb/nested/path")
			})

			it("does not warn for an unrelated target", func() {
				_, warnings, err := processVolumes("linux", []string{"/host:/somewhere:ro"})
				h.AssertNil(t, err)
				h.AssertEq(t, len(warnings), 0)
			})
		})

		when("the volume specification is invalid", func() {
			it("wraps the parser error with the offending spec", func() {
				_, _, err := processVolumes("linux", []string{"/host:/container:sw"})
				h.AssertNotNil(t, err)
				h.AssertContains(t, err.Error(), `platform volume "/host:/container:sw" has invalid format`)
				h.AssertContains(t, err.Error(), "invalid mode")
			})

			it("rejects a relative mount path", func() {
				_, _, err := processVolumes("linux", []string{"/host:container"})
				h.AssertNotNil(t, err)
				h.AssertContains(t, err.Error(), "mount path must be absolute")
			})

			it("rejects an empty specification", func() {
				_, _, err := processVolumes("linux", []string{""})
				h.AssertNotNil(t, err)
				h.AssertContains(t, err.Error(), "invalid volume specification")
			})

			// The first spec parses fine; the second does not. Nothing from the
			// successful one should leak out alongside the error.
			it("returns no results alongside the error", func() {
				processed, warnings, err := processVolumes("linux", []string{"/a:/one:ro", "::"})
				h.AssertNotNil(t, err)
				h.AssertEq(t, len(processed), 0)
				h.AssertEq(t, len(warnings), 0)
			})
		})
	})

	when("#processMode", func() {
		it("defaults an empty mode to read-only", func() {
			h.AssertEq(t, processMode(""), "ro")
		})

		it("passes any other mode through unchanged", func() {
			h.AssertEq(t, processMode("rw"), "rw")
			h.AssertEq(t, processMode("ro,z"), "ro,z")
		})
	})
}
