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
	spec.Run(t, "ManagedCollection", testModuleManager, spec.Report(report.Terminal{}))
}

func testModuleManager(t *testing.T, when spec.G, it spec.S) {
	/* compositeBP1
	 *    /    \
	 *   bp1   compositeBP2
	 *           /   |    \
	 *	      bp21 bp22 compositeBP3
	 *			          |
	 *		            bp31
	 */
	var (
		moduleManager *buildpack.ManagedCollection
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
					},
					{
						ModuleInfo: bp22.Descriptor().Info(),
					},
					{
						ModuleInfo: compositeBP3.Descriptor().Info(),
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
					},
					{
						ModuleInfo: compositeBP2.Descriptor().Info(),
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
				moduleManager.AddModules(compositeBP1, []buildpack.BuildModule{bp1, compositeBP2, bp21, bp22, compositeBP3, bp31}...)
			})

			when("#FlattenedModules", func() {
				it("returns one flatten module (1 layer)", func() {
					modules := moduleManager.FlattenedModules()
					h.AssertEq(t, len(modules), 1)
					h.AssertEq(t, len(modules[0]), 7)
				})
			})

			when("#ExplodedModules", func() {
				it("returns empty", func() {
					modules := moduleManager.ExplodedModules()
					h.AssertEq(t, len(modules), 0)
				})
			})

			when("#AllModules", func() {
				it("returns all explodedModules", func() {
					modules := moduleManager.AllModules()
					h.AssertEq(t, len(modules), 7)
				})
			})

			when("#ShouldFlatten", func() {
				it("returns true for flatten explodedModules", func() {
					h.AssertTrue(t, moduleManager.ShouldFlatten(compositeBP1))
					h.AssertTrue(t, moduleManager.ShouldFlatten(bp1))
					h.AssertTrue(t, moduleManager.ShouldFlatten(compositeBP2))
					h.AssertTrue(t, moduleManager.ShouldFlatten(bp21))
					h.AssertTrue(t, moduleManager.ShouldFlatten(bp22))
					h.AssertTrue(t, moduleManager.ShouldFlatten(compositeBP3))
					h.AssertTrue(t, moduleManager.ShouldFlatten(bp31))
				})
			})
		})

		when("flatten with depth=1", func() {
			it.Before(func() {
				moduleManager = buildpack.NewModuleManager(true, 1)
				moduleManager.AddModules(compositeBP1, []buildpack.BuildModule{bp1, compositeBP2, bp21, bp22, compositeBP3, bp31}...)
			})

			when("#FlattenedModules", func() {
				it("returns 1 flatten module with [compositeBP2, bp21, bp22, compositeBP3, bp31]", func() {
					modules := moduleManager.FlattenedModules()
					h.AssertEq(t, len(modules), 1)
					h.AssertEq(t, len(modules[0]), 5)
				})
			})

			when("#ShouldFlatten", func() {
				it("returns true for flatten explodedModules", func() {
					h.AssertTrue(t, moduleManager.ShouldFlatten(compositeBP2))
					h.AssertTrue(t, moduleManager.ShouldFlatten(bp21))
					h.AssertTrue(t, moduleManager.ShouldFlatten(bp22))
					h.AssertTrue(t, moduleManager.ShouldFlatten(compositeBP3))
					h.AssertTrue(t, moduleManager.ShouldFlatten(bp31))
				})

				it("returns false for no flatten explodedModules", func() {
					h.AssertFalse(t, moduleManager.ShouldFlatten(bp1))
					h.AssertFalse(t, moduleManager.ShouldFlatten(compositeBP1))
				})
			})

			when("#ExplodedModules", func() {
				it("returns [bp1, compositeBP1]", func() {
					modules := moduleManager.ExplodedModules()
					h.AssertEq(t, len(modules), 2)
				})
			})
		})

		when("flatten with depth=2", func() {
			it.Before(func() {
				moduleManager = buildpack.NewModuleManager(true, 2)
				moduleManager.AddModules(compositeBP1, []buildpack.BuildModule{bp1, compositeBP2, bp21, bp22, compositeBP3, bp31}...)
			})

			when("#FlattenedModules", func() {
				it("returns 1 flatten module with [compositeBP3, bp31]", func() {
					modules := moduleManager.FlattenedModules()
					h.AssertEq(t, len(modules), 1)
					h.AssertEq(t, len(modules[0]), 2)
				})
			})

			when("#ShouldFlatten", func() {
				it("returns true for flatten explodedModules", func() {
					h.AssertTrue(t, moduleManager.ShouldFlatten(compositeBP3))
					h.AssertTrue(t, moduleManager.ShouldFlatten(bp31))
				})

				it("returns false for no flatten explodedModules", func() {
					h.AssertFalse(t, moduleManager.ShouldFlatten(compositeBP2))
					h.AssertFalse(t, moduleManager.ShouldFlatten(bp21))
					h.AssertFalse(t, moduleManager.ShouldFlatten(bp22))
					h.AssertFalse(t, moduleManager.ShouldFlatten(bp1))
					h.AssertFalse(t, moduleManager.ShouldFlatten(compositeBP1))
				})
			})

			when("#ExplodedModules", func() {
				it("returns [compositeBP1, bp1, compositeBP2, bp21, bp22]", func() {
					modules := moduleManager.ExplodedModules()
					h.AssertEq(t, len(modules), 5)
				})
			})
		})
	})

	when("manager is not configured in flatten mode", func() {
		it.Before(func() {
			moduleManager = buildpack.NewModuleManager(false, buildpack.FlattenNone)
		})

		when("#ExplodedModules", func() {
			it("returns nil when no explodedModules are added", func() {
				modules := moduleManager.ExplodedModules()
				h.AssertEq(t, len(modules), 0)
			})

			when("explodedModules are added", func() {
				it.Before(func() {
					moduleManager.AddModules(compositeBP1, []buildpack.BuildModule{bp1, compositeBP2, bp21, bp22, compositeBP3, bp31}...)
				})
				it("returns all explodedModules added", func() {
					modules := moduleManager.ExplodedModules()
					h.AssertEq(t, len(modules), 7)
				})
			})
		})

		when("#FlattenedModules", func() {
			it("returns nil when no explodedModules are added", func() {
				modules := moduleManager.FlattenedModules()
				h.AssertEq(t, len(modules), 0)
			})

			when("explodedModules are added", func() {
				it.Before(func() {
					moduleManager.AddModules(compositeBP1, []buildpack.BuildModule{bp1, compositeBP2, bp21, bp22, compositeBP3, bp31}...)
				})
				it("returns nil", func() {
					modules := moduleManager.FlattenedModules()
					h.AssertEq(t, len(modules), 0)
				})
			})
		})

		when("#ShouldFlatten", func() {
			it("returns false when no explodedModules are added", func() {
				h.AssertFalse(t, moduleManager.ShouldFlatten(bp1))
			})

			when("explodedModules are added", func() {
				it.Before(func() {
					moduleManager.AddModules(compositeBP1, []buildpack.BuildModule{bp1, compositeBP2, bp21, bp22, compositeBP3, bp31}...)
				})
				it("returns false", func() {
					h.AssertFalse(t, moduleManager.ShouldFlatten(bp1))
					h.AssertFalse(t, moduleManager.ShouldFlatten(bp21))
					h.AssertFalse(t, moduleManager.ShouldFlatten(bp22))
					h.AssertFalse(t, moduleManager.ShouldFlatten(bp31))
				})
			})
		})
	})
}
