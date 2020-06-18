package registry_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/internal/registry"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestRegistryCache(t *testing.T) {
	spec.Run(t, "Cache", func(t *testing.T, when spec.G, it spec.S) {
		var (
			tmpDir          string
			registryFixture string
			registryCache   registry.Cache
			outBuf          bytes.Buffer
			logger          logging.Logger
		)

		it.Before(func() {
			logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

			tmpDir, err := ioutil.TempDir("", "registry")
			h.AssertNil(t, err)

			registryFixture = h.CreateRegistryFixture(t, tmpDir, filepath.Join("..", "..", "testdata", "registry"))

			registryCache, err = registry.NewRegistryCache(logger, tmpDir, registryFixture)
			h.AssertNil(t, err)
		})

		it.After(func() {
			err := os.RemoveAll(tmpDir)
			h.AssertNil(t, err)
		})

		it("locates a buildpack without version", func() {
			bp, err := registryCache.LocateBuildpack("example/java")
			h.AssertNil(t, err)
			h.AssertNotNil(t, bp)

			h.AssertEq(t, bp.Namespace, "example")
			h.AssertEq(t, bp.Name, "java")
			h.AssertEq(t, bp.Version, "1.0.0")
		})

		it("locates a buildpack without version", func() {
			bp, err := registryCache.LocateBuildpack("example/foo")
			h.AssertNil(t, err)
			h.AssertNotNil(t, bp)

			h.AssertEq(t, bp.Namespace, "example")
			h.AssertEq(t, bp.Name, "foo")
			h.AssertEq(t, bp.Version, "1.2.0")
		})

		it("locates a buildpack with version", func() {
			bp, err := registryCache.LocateBuildpack("example/foo@1.1.0")
			h.AssertNil(t, err)
			h.AssertNotNil(t, bp)

			h.AssertEq(t, bp.Namespace, "example")
			h.AssertEq(t, bp.Name, "foo")
			h.AssertEq(t, bp.Version, "1.1.0")
		})

		it("does not locate a buildpack", func() {
			_, err := registryCache.LocateBuildpack("example/quack")
			h.AssertNotNil(t, err)
		})

		when("registry has new commits", func() {
			it.Before(func() {
				err := registryCache.Refresh()
				h.AssertNil(t, err)

				h.AssertGitHeadEq(t, registryFixture, registryCache.Root)

				r, err := git.PlainOpen(registryFixture)
				h.AssertNil(t, err)

				w, err := r.Worktree()
				h.AssertNil(t, err)

				commit, err := w.Commit("second", &git.CommitOptions{
					Author: &object.Signature{
						Name:  "John Doe",
						Email: "john@doe.org",
						When:  time.Now(),
					},
				})
				h.AssertNil(t, err)

				_, err = r.CommitObject(commit)
				h.AssertNil(t, err)
			})

			it("pulls the latest index", func() {
				h.AssertNil(t, registryCache.Refresh())
				h.AssertGitHeadEq(t, registryFixture, registryCache.Root)
			})
		})
	})

	spec.Run(t, "Buildpack", func(t *testing.T, when spec.G, it spec.S) {
		when("#Validate", func() {
			it("errors when address is missing", func() {
				b := registry.Buildpack{
					Address: "",
				}

				h.AssertNotNil(t, b.Validate())
			})

			it("errors when not a digest", func() {
				b := registry.Buildpack{
					Address: "example.com/some/package:18",
				}

				h.AssertNotNil(t, b.Validate())
			})

			it("does not error when address is a digest", func() {
				b := registry.Buildpack{
					Address: "example.com/some/package@sha256:8c27fe111c11b722081701dfed3bd55e039b9ce92865473cf4cdfa918071c566",
				}

				h.AssertNil(t, b.Validate())
			})
		})
	})
}
