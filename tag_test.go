package pack_test

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestTag(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "build", testTag, spec.Report(report.Terminal{}))
}

func testTag(t *testing.T, when spec.G, it spec.S) {
	var (
		tag    pack.Tag
		err    error
		assert = h.NewAssertionManager(t)
	)
	when("Tag", func() {
		it.Before(func() {
			tag, err = pack.NewTag("gcr.io/repo/image-name:tag-name")
			assert.Nil(err)
		})

		when("#Identifier", func() {
			it("returns specified tag", func() {
				assert.Equal(tag.Identifier(), "tag-name")
			})
			it("defaults to latest if no tag is specified", func() {
				tag, err = pack.NewTag("gcr.io/repo/image-name")
				assert.Nil(err)
				assert.Equal(tag.Identifier(), "latest")
			})
		})

		when("#Scope", func() {
			it("returns scope required to access this reference", func() {
				h.AssertEq(
					t,
					tag.Scope("some-action"),
					"repository:repo/image-name:some-action",
				)
			})
		})

		when("#Context", func() {
			when("registry is omitted", func() {
				it.Before(func() {
					tag, err = pack.NewTag("image-name:tag-name")
					assert.Nil(err)
				})
				it("keeps index.docker.io/library prefix", func() {
					assert.Equal(tag.Context().String(), "index.docker.io/library/image-name")
				})
			})
			when("registry is not omitted", func() {
				it("keeps registry context", func() {
					assert.Equal(tag.Context().String(), "gcr.io/repo/image-name")
				})
			})
		})

		when("#Name", func() {
			when("registry prefix is omitted", func() {
				it.Before(func() {
					tag, err = pack.NewTag("image-name:tag-name")
					assert.Nil(err)
				})
				it("omits index.docker.io/library/ prefix", func() {
					assert.Equal(tag.Name(), "image-name:tag-name")
				})
			})

			when("registry prefix is provided", func() {
				it("keeps specified prefix", func() {
					assert.Equal(tag.Name(), "gcr.io/repo/image-name:tag-name")
				})

				when("index.docker.io/library/ prefix is specified", func() {
					it("keeps the prefix", func() {
						tag, err = pack.NewTag("index.docker.io/library/image-name:tag-name")
						assert.Nil(err)
						assert.Equal(tag.Name(), "index.docker.io/library/image-name:tag-name")
					})

					when("index.docker.io prefix is specified", func() {
						it("keeps the  prefix", func() {
							tag, err = pack.NewTag("index.docker.io/other/image-name:tag-name")
							assert.Nil(err)
							assert.Equal(tag.Name(), "index.docker.io/other/image-name:tag-name")
						})
					})
				})
			})
		})

		when("Error cases", func() {
			it("passed malformed tagName", func() {
				_, err := pack.NewTag((":::"))
				assert.ErrorContains(err, "error creating tag: ")
			})
		})
	})
}
