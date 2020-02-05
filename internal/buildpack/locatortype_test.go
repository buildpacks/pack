package buildpack_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/buildpack"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestGetLocatorType(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "testGetLocatorType", testGetLocatorType, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testGetLocatorType(t *testing.T, when spec.G, it spec.S) {
	type testCase struct {
		locator      string
		builderIDs   []string
		expectedType buildpack.LocatorType
		expectedErr  string
		localPath    string
	}

	var localPath = func(path string) string {
		return filepath.Join("testdata", path)
	}

	for _, tc := range []testCase{
		{
			locator:      "from=builder",
			expectedType: buildpack.FromBuilderLocator,
		},
		{
			locator:      "from=builder:some-bp",
			builderIDs:   []string{"some-bp@some-version"},
			expectedType: buildpack.IDLocator,
		},
		{
			locator:     "from=builder:some-bp",
			builderIDs:  nil,
			expectedErr: "'from=builder:some-bp' is not a valid identifier",
		},
		{
			locator:     "from=builder:some-bp@some-other-version",
			builderIDs:  []string{"some-bp@some-version"},
			expectedErr: "'from=builder:some-bp@some-other-version' is not a valid identifier",
		},
		{
			locator:      "some-bp",
			builderIDs:   []string{"some-bp"},
			expectedType: buildpack.IDLocator,
		},
		{
			locator:      localPath("some-bp"),
			builderIDs:   []string{localPath("some-bp") + "@some-version"},
			localPath:    localPath("some-bp"),
			expectedType: buildpack.URILocator,
		},
		{
			locator:      "https://example.com/buildpack.tgz",
			expectedType: buildpack.URILocator,
		},
		{
			locator:      "cnbs/some-bp",
			builderIDs:   nil,
			localPath:    "",
			expectedType: buildpack.PackageLocator,
		},
		{
			locator:      "cnbs/some-bp@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildpack.PackageLocator,
		},
		{
			locator:      "cnbs/some-bp:some-tag",
			expectedType: buildpack.PackageLocator,
		},
		{
			locator:      "cnbs/some-bp:some-tag@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildpack.PackageLocator,
		},
		{
			locator:      "registry.com/cnbs/some-bp",
			expectedType: buildpack.PackageLocator,
		},
		{
			locator:      "registry.com/cnbs/some-bp@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildpack.PackageLocator,
		},
		{
			locator:      "registry.com/cnbs/some-bp:some-tag",
			expectedType: buildpack.PackageLocator,
		},
		{
			locator:      "registry.com/cnbs/some-bp:some-tag@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildpack.PackageLocator,
		},
	} {
		tc := tc

		desc := fmt.Sprintf("locator is %s", tc.locator)
		if len(tc.builderIDs) > 0 {
			desc += fmt.Sprintf(" and builder has IDs %s", tc.builderIDs)
		}
		if tc.localPath != "" {
			desc += fmt.Sprintf(" and a local path exists at '%s'", tc.localPath)
		}

		when(desc, func() {
			it(fmt.Sprintf("should return '%s'", tc.expectedType), func() {
				actualType, actualErr := buildpack.GetLocatorType(tc.locator, tc.builderIDs)

				if tc.expectedErr == "" {
					h.AssertNil(t, actualErr)
				} else {
					h.AssertError(t, actualErr, tc.expectedErr)
				}

				h.AssertEq(t, actualType, tc.expectedType)
			})
		})
	}
}
