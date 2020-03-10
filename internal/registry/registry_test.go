package registry_test

import (
	"github.com/docker/docker/pkg/archive"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/sclevine/spec"

	"github.com/buildpacks/pack/internal/registry"
	h "github.com/buildpacks/pack/testhelpers"
)

func NewMockRegistryCache(source string) (registry.RegistryCache, error) {
	home, err := ioutil.TempDir("", "registry")
	if err != nil {
		return registry.RegistryCache{}, err
	}

	r := registry.RegistryCache{
		URL:  source,
		Path: home,
	}
	return r, r.Initialize()
}

func createRegistryFixture(t *testing.T) (string) {
	// copy fixture to temp dir
	registryFixtureCopy, err := ioutil.TempDir("", "registry")
	h.AssertNil(t, err)

	err = archive.CopyResource(filepath.Join("..", "..", "testdata", "registry"), registryFixtureCopy, false)
	h.AssertNil(t, err)

	// git init that dir
	repository, err := git.PlainInit(registryFixtureCopy, false)
	h.AssertNil(t, err)

	// git add . that dir
	worktree, err := repository.Worktree()
	h.AssertNil(t, err)

	_, err = worktree.Add(".")
	h.AssertNil(t, err)

	// git commit that dir
	commit, err := worktree.Commit("first", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "John Doe",
			Email: "john@doe.org",
			When:  time.Now(),
		},
	})

	_, err = repository.CommitObject(commit)
	h.AssertNil(t, err)

	return registryFixtureCopy
}

func TestRegistryCache(t *testing.T) {
	spec.Run(t, "RegistryCache", func(t *testing.T, when spec.G, it spec.S) {
		var (
			registryCache registry.RegistryCache
		)

		it.Before(func() {
			registryFixture := createRegistryFixture(t)

			registryCache, err := NewMockRegistryCache(registryFixture)
			h.AssertNil(t, err)
			h.AssertNotNil(t, registryCache)
		})

		it.After(func() {
			// delete registry
		})

		it("locates a buildpack", func() {
			bp, err := registryCache.LocateBuildpack("example/foo")
			h.AssertNil(t, err)
			h.AssertNotNil(t, bp)
		})
	})
}
