package cache

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/pack/testhelpers"
)

type CacheOptTestCase struct {
	name       string
	input      string
	output     string
	shouldFail bool
}

func TestMetadata(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Metadata", testCacheOpts, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testCacheOpts(t *testing.T, when spec.G, it spec.S) {
	when("cache options are passed", func() {
		it("image cache format with complete options", func() {
			testcases := []CacheOptTestCase{
				{
					name:   "Build cache as Image",
					input:  "type=build;format=image;name=io.test.io/myorg/my-cache:build",
					output: "type=build;format=image;name=io.test.io/myorg/my-cache:build",
				},
			}

			for _, testcase := range testcases {
				var cacheFlags CacheOpts
				t.Logf("Testing cache type: %s", testcase.name)
				err := cacheFlags.Set(testcase.input)
				h.AssertNil(t, err)
				h.AssertEq(t, testcase.output, cacheFlags.String())
			}
		})

		it("image cache format with missing options", func() {
			successTestCases := []CacheOptTestCase{
				{
					name:   "Build cache as Image missing: type",
					input:  "format=image;name=io.test.io/myorg/my-cache:build",
					output: "type=build;format=image;name=io.test.io/myorg/my-cache:build",
				},
				{
					name:   "Build cache as Image missing: format",
					input:  "type=build;name=io.test.io/myorg/my-cache:build",
					output: "type=build;format=volume;name=io.test.io/myorg/my-cache:build",
				},
				{
					name:   "Build cache as Image missing: type, format",
					input:  "name=io.test.io/myorg/my-cache:build",
					output: "type=build;format=volume;name=io.test.io/myorg/my-cache:build",
				},
				{
					name:       "Build cache as Image missing: name",
					input:      "type=build;format=image",
					output:     "cache 'name' is required",
					shouldFail: true,
				},
				{
					name:       "Build cache as Image missing: format, name",
					input:      "type=build",
					output:     "cache 'name' is required",
					shouldFail: true,
				},
				{
					name:       "Build cache as Image missing: type, name",
					input:      "format=image",
					output:     "cache 'name' is required",
					shouldFail: true,
				},
			}

			for _, testcase := range successTestCases {
				var cacheFlags CacheOpts
				t.Logf("Testing cache type: %s", testcase.name)
				err := cacheFlags.Set(testcase.input)

				if testcase.shouldFail {
					h.AssertError(t, err, testcase.output)
				} else {
					h.AssertNil(t, err)
					output := cacheFlags.String()
					h.AssertEq(t, testcase.output, output)
				}
			}
		})
	})
}
