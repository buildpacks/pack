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
	/* compositeBP1
	 *    /    \
	 *   bp1   compositeBP2
	 *           /   |    \
	 *		  bp21 bp22 compositeBP3
	 *						 |
	 *						bp31
	 */
	var (
		moduleManager *buildpack.ModuleManager
		compositeBP1  buildpack.BuildModule
		bp1           buildpack.BuildModule
		compositeBP2  buildpack.BuildModule
		bp21          buildpack.BuildModule
		bp22          buildpack.BuildModule
		compositeBP3  buildpack.BuildModule
		bp31          buildpack.BuildModule
		err           error
	)

	it.Before(func() {
		bp1, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version",
			},
		}, 0644)
		h.AssertNil(t, err)

		bp21, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "buildpack-21-id",
				Version: "buildpack-21-version",
			},
		}, 0644)
		h.AssertNil(t, err)

		bp22, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "buildpack-22-id",
				Version: "buildpack-22-version",
			},
		}, 0644)
		h.AssertNil(t, err)

		bp31, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "buildpack-31-id",
				Version: "buildpack-31-version",
			},
		}, 0644)
		h.AssertNil(t, err)

		compositeBP3, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "composite-buildpack-3-id",
				Version: "composite-buildpack-3-version",
			},
			WithOrder: []dist.OrderEntry{{
				Group: []dist.ModuleRef{
					{
						ModuleInfo: bp31.Descriptor().Info(),
						Optional:   true,
					},
				},
			}},
		}, 0644)
		h.AssertNil(t, err)

		compositeBP2, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "composite-buildpack-2-id",
				Version: "composite-buildpack-2-version",
			},
			WithOrder: []dist.OrderEntry{{
				Group: []dist.ModuleRef{
					{
						ModuleInfo: bp21.Descriptor().Info(),
						Optional:   true,
					},
					{
						ModuleInfo: bp22.Descriptor().Info(),
						Optional:   false,
					},
					{
						ModuleInfo: compositeBP3.Descriptor().Info(),
						Optional:   false,
					},
				},
			}},
		}, 0644)
		h.AssertNil(t, err)

		compositeBP1, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "composite-buildpack-1-id",
				Version: "composite-buildpack-1-version",
			},
			WithOrder: []dist.OrderEntry{{
				Group: []dist.ModuleRef{
					{
						ModuleInfo: bp1.Descriptor().Info(),
						Optional:   false,
					},
					{
						ModuleInfo: compositeBP2.Descriptor().Info(),
						Optional:   false,
					},
				},
			}},
		}, 0644)
		h.AssertNil(t, err)
	})

	when("manager is configured in flatten mode", func() {
		when("flatten all", func() {
			it.Before(func() {
				moduleManager = buildpack.NewModuleManager(true, buildpack.FlattenMaxDepth)
				moduleManager.AddModules(compositeBP1, []buildpack.BuildModule{bp1, compositeBP2}...)
			})

			when("#GetFlattenModules", func() {
				it("returns one big module", func() {
					modules := moduleManager.GetFlattenModules()
					h.AssertEq(t, len(modules), 1)
				})
			})

			when("#IsFlatten", func() {
				it("returns true", func() {
					// check a composite leaf module
					h.AssertFalse(t, moduleManager.IsFlatten(bp31))
				})
			})
		})

		when("flatten with max depth", func() {
			it.Before(func() {
				moduleManager = buildpack.NewModuleManager(true, 2)
			})

			when("#AddModules", func() {

			})

			when("#GetFlattenModules", func() {

			})

			when("#IsFlatten", func() {

			})
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
					moduleManager.AddModules(compositeBP1, []buildpack.BuildModule{bp1, compositeBP2}...)
				})
				it("returns the modules added", func() {
					modules := moduleManager.Modules()
					h.AssertEq(t, len(modules), 3)
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
					moduleManager.AddModules(compositeBP1, []buildpack.BuildModule{bp1, compositeBP2}...)
				})
				it("returns nil", func() {
					modules := moduleManager.GetFlattenModules()
					h.AssertEq(t, len(modules), 0)
				})
			})
		})

		when("#IsFlatten", func() {
			it("returns false when no modules are added", func() {
				h.AssertFalse(t, moduleManager.IsFlatten(bp1))
			})

			when("modules are added", func() {
				it.Before(func() {
					moduleManager.AddModules(compositeBP1, []buildpack.BuildModule{bp1, compositeBP2}...)
				})
				it("returns false", func() {
					h.AssertFalse(t, moduleManager.IsFlatten(compositeBP2))
				})
			})
		})
	})
}
