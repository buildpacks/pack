package buildmodule_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/buildmodule"
	"github.com/buildpacks/pack/pkg/dist"
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
		builderBPs   []dist.ModuleInfo
		expectedType buildmodule.LocatorType
		expectedErr  string
	}

	var localPath = func(path string) string {
		return filepath.Join("testdata", path)
	}

	for _, tc := range []testCase{
		{
			locator:      "from=builder",
			expectedType: buildmodule.FromBuilderLocator,
		},
		{
			locator:      "from=builder:some-bp",
			builderBPs:   []dist.ModuleInfo{{ID: "some-bp", Version: "some-version"}},
			expectedType: buildmodule.IDLocator,
		},
		{
			locator:     "from=builder:some-bp",
			expectedErr: "'from=builder:some-bp' is not a valid identifier",
		},
		{
			locator:     "from=builder:some-bp@some-other-version",
			builderBPs:  []dist.ModuleInfo{{ID: "some-bp", Version: "some-version"}},
			expectedErr: "'from=builder:some-bp@some-other-version' is not a valid identifier",
		},
		{
			locator:      "urn:cnb:builder:some-bp",
			builderBPs:   []dist.ModuleInfo{{ID: "some-bp", Version: "some-version"}},
			expectedType: buildmodule.IDLocator,
		},
		{
			locator:     "urn:cnb:builder:some-bp",
			expectedErr: "'urn:cnb:builder:some-bp' is not a valid identifier",
		},
		{
			locator:     "urn:cnb:builder:some-bp@some-other-version",
			builderBPs:  []dist.ModuleInfo{{ID: "some-bp", Version: "some-version"}},
			expectedErr: "'urn:cnb:builder:some-bp@some-other-version' is not a valid identifier",
		},
		{
			locator:      "some-bp",
			builderBPs:   []dist.ModuleInfo{{ID: "some-bp", Version: "any-version"}},
			expectedType: buildmodule.IDLocator,
		},
		{
			locator:      localPath("buildpack"),
			builderBPs:   []dist.ModuleInfo{{ID: "bp.one", Version: "1.2.3"}},
			expectedType: buildmodule.URILocator,
		},
		{
			locator:      "https://example.com/buildpack.tgz",
			expectedType: buildmodule.URILocator,
		},
		{
			locator:      "localhost:1234/example/package-cnb",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "cnbs/some-bp:latest",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "docker://cnbs/some-bp",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "docker://cnbs/some-bp@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "docker://cnbs/some-bp:some-tag",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "docker://cnbs/some-bp:some-tag@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "docker://registry.com/cnbs/some-bp",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "docker://registry.com/cnbs/some-bp@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "docker://registry.com/cnbs/some-bp:some-tag",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "docker://registry.com/cnbs/some-bp:some-tag@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "cnbs/some-bp@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "cnbs/some-bp:some-tag@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "registry.com/cnbs/some-bp",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "registry.com/cnbs/some-bp@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "registry.com/cnbs/some-bp:some-tag",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "registry.com/cnbs/some-bp:some-tag@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedType: buildmodule.PackageLocator,
		},
		{
			locator:      "urn:cnb:registry:example/foo@1.0.0",
			expectedType: buildmodule.RegistryLocator,
		},
		{
			locator:      "example/foo@1.0.0",
			expectedType: buildmodule.RegistryLocator,
		},
		{
			locator:      "example/registry-cnb",
			expectedType: buildmodule.RegistryLocator,
		},
		{
			locator:      "cnbs/sample-package@hello-universe",
			expectedType: buildmodule.InvalidLocator,
		},
		{
			locator:      "dev.local/http-go-fn:latest",
			expectedType: buildmodule.PackageLocator,
		},
	} {
		tc := tc

		desc := fmt.Sprintf("locator is %s", tc.locator)
		if len(tc.builderBPs) > 0 {
			var names []string
			for _, bp := range tc.builderBPs {
				names = append(names, bp.FullName())
			}
			desc += fmt.Sprintf(" and builder has buildpacks %s", names)
		}

		when(desc, func() {
			it(fmt.Sprintf("should return %s", tc.expectedType), func() {
				actualType, actualErr := buildmodule.GetLocatorType(tc.locator, "", tc.builderBPs)

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
