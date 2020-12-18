package style_test

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/style"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestStyle(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "testStyle", testStyle, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testStyle(t *testing.T, when spec.G, it spec.S) {
	when("#Style", func() {
		it("Symbol function should return the expected value", func() {
			h.AssertEq(t, style.Symbol("Symbol"), "'Symbol'")
		})

		it("Symbol function should return an empty string", func() {
			h.AssertEq(t, style.Symbol(""), "''")
		})

		it("Map function should return a string with all key value pairs", func() {
			h.AssertEq(t, style.Map(map[string]string{"FOO": "foo", "BAR": "bar"}, "", " "), "'FOO=foo BAR=bar'")
			h.AssertEq(t, style.Map(map[string]string{"FOO": "foo", "BAR": "bar"}, "  ", "\n"), "'FOO=foo\n  BAR=bar'")
		})

		it("Map function should return an empty string", func() {
			h.AssertEq(t, style.Map(map[string]string{}, "", " "), "''")
		})
	})
}
