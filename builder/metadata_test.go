package builder

import (
	"testing"

	"github.com/Masterminds/semver"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/lifecycle"
	h "github.com/buildpack/pack/testhelpers"
)

func TestMetadata(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "Metadata", testMetadata, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testMetadata(t *testing.T, when spec.G, it spec.S) {
	when("#processMetadata", func() {
		when("the buildpack is the only version", func() {
			it("should resolve unset version", func() {
				md := Metadata{
					Buildpacks: []BuildpackMetadata{
						{ID: "bp.id.1", Version: "1.2.3"},
					},
					Groups: []GroupMetadata{
						{
							Buildpacks: []BuildpackRefMetadata{
								{ID: "bp.id.1", Version: ""},
							},
						},
					},
				}

				err := processMetadata(&md)
				h.AssertNil(t, err)

				h.AssertEq(t, md.Buildpacks[0].Latest, true)
				h.AssertEq(t, md.Groups[0].Buildpacks[0].Version, "1.2.3")
			})
		})

		when("the buildpack has multiple versions", func() {
			when("order contains specific version", func() {
				it("nothing should be updated", func() {
					md := Metadata{
						Buildpacks: []BuildpackMetadata{
							{ID: "bp.id.1", Version: "1.2.3"},
							{ID: "bp.id.1", Version: "4.5.6"},
						},
						Groups: []GroupMetadata{
							{
								Buildpacks: []BuildpackRefMetadata{
									{ID: "bp.id.1", Version: "1.2.3"},
								},
							},
						},
						Lifecycle: lifecycle.Metadata{
							Version: semver.MustParse("0.0.0"),
						},
					}

					err := processMetadata(&md)
					h.AssertNil(t, err)

					expected := Metadata{
						Buildpacks: []BuildpackMetadata{
							{ID: "bp.id.1", Version: "1.2.3"},
							{ID: "bp.id.1", Version: "4.5.6"},
						},
						Groups: []GroupMetadata{
							{
								Buildpacks: []BuildpackRefMetadata{
									{ID: "bp.id.1", Version: "1.2.3"},
								},
							},
						},
						Lifecycle: lifecycle.Metadata{
							Version: semver.MustParse("0.0.0"),
						},
					}

					h.AssertEq(t, md, expected)
				})
			})

			when("order contains 'latest'", func() {
				it("should error", func() {
					md := Metadata{
						Buildpacks: []BuildpackMetadata{
							{ID: "bp.id.1", Version: "1.2.3"},
							{ID: "bp.id.1", Version: "4.5.6"},
						},
						Groups: []GroupMetadata{
							{
								Buildpacks: []BuildpackRefMetadata{
									{ID: "bp.id.1", Version: ""},
								},
							},
						},
					}

					err := processMetadata(&md)
					h.AssertError(t, err, "multiple versions of 'bp.id.1' - must specify an explicit version")
				})
			})
		})

		when("the buildpack has no versions", func() {
			when("order has unset version", func() {
				it("should error", func() {
					md := Metadata{
						Buildpacks: []BuildpackMetadata{
							{ID: "bp.id.1", Version: "1.2.3"},
						},
						Groups: []GroupMetadata{
							{
								Buildpacks: []BuildpackRefMetadata{
									{ID: "bp.id.no-exists", Version: ""},
								},
							},
						},
					}

					err := processMetadata(&md)
					h.AssertError(t, err, "no versions of buildpack 'bp.id.no-exists' were found on the builder")
				})
			})

			when("order does not have unset version", func() {
				it("should error", func() {
					md := Metadata{
						Buildpacks: []BuildpackMetadata{
							{ID: "bp.id.1", Version: "1.2.3"},
						},
						Groups: []GroupMetadata{
							{
								Buildpacks: []BuildpackRefMetadata{
									{ID: "bp.id.no-exists", Version: "4.5.6"},
								},
							},
						},
					}

					err := processMetadata(&md)
					h.AssertError(t, err, "no versions of buildpack 'bp.id.no-exists' were found on the builder")
				})
			})
		})
	})
}
