package buildpack_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestModuleManager(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "ModuleManager", testModuleManager, spec.Report(report.Terminal{}))
}

func testModuleManager(t *testing.T, when spec.G, it spec.S) {
	var (
		moduleManager *buildpack.ModuleManager
		bp1v1         buildpack.BuildModule
		err           error
	)

	it.Before(func() {
		bp1v1, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-1",
			},
			WithStacks: []dist.Stack{{
				ID:     "some.stack.id",
				Mixins: []string{"mixinX", "mixinY"},
			}},
		}, 0644)
		h.AssertNil(t, err)
	})

	when("manager is configured in flatten mode", func() {
		when("#AddModules", func() {

		})

		when("#GetFlattenModules", func() {

		})

		when("#IsFlatten", func() {

		})
	})

	when("manager is not configured in flatten mode", func() {
		it.Before(func() {
			moduleManager = buildpack.NewModuleManager(false, buildpack.FlattenNone)
		})

		when("#Modules", func() {
			it("returns nil when no modules are added", func() {
				modules := moduleManager.Modules()
				h.AssertEq(t, len(modules), 0)
			})

			when("modules are added", func() {
				it.Before(func() {
					moduleManager.AddModules(bp1v1)
				})
				it("returns the modules added", func() {
					modules := moduleManager.Modules()
					h.AssertEq(t, len(modules), 1)
				})
			})
		})

		when("#GetFlattenModules", func() {
			it("returns nil when no modules are added", func() {
				modules := moduleManager.GetFlattenModules()
				h.AssertEq(t, len(modules), 0)
			})

			when("modules are added", func() {
				it.Before(func() {
					moduleManager.AddModules(bp1v1)
				})
				it("returns nil", func() {
					modules := moduleManager.GetFlattenModules()
					h.AssertEq(t, len(modules), 0)
				})
			})
		})

		when("#IsFlatten", func() {
			it("returns false when no modules are added", func() {
				h.AssertFalse(t, moduleManager.IsFlatten(bp1v1))
			})

			when("modules are added", func() {
				it.Before(func() {
					moduleManager.AddModules(bp1v1)
				})
				it("returns false", func() {
					h.AssertFalse(t, moduleManager.IsFlatten(bp1v1))
				})
			})
		})
	})
}
