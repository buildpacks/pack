package registry_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/internal/registry"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestGit(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Git", testGit, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testGit(t *testing.T, when spec.G, it spec.S) {
	var (
		registryCache   registry.Cache
		tmpDir          string
		err             error
		registryFixture string
		outBuf          bytes.Buffer
		logger          logging.Logger
		username        string = "supra08"
	)

	bp := registry.Buildpack{
		Namespace: "example",
		Name:      "python",
		Version:   "1.0.0",
		Yanked:    false,
		Address:   "example.com",
	}

	it.Before(func() {
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

		tmpDir, err = ioutil.TempDir("", "registry")
		h.AssertNil(t, err)

		registryFixture = h.CreateRegistryFixture(t, tmpDir, filepath.Join("..", "..", "testdata", "registry"))
		registryCache, err = registry.NewRegistryCache(logger, tmpDir, registryFixture)
		h.AssertNil(t, err)
	})

	it.After(func() {
		err := os.RemoveAll(tmpDir)
		h.AssertNil(t, err)
	})

	when("#createGitCommit", func() {
		it("should work with a proper buildpack and registry cache", func() {
			err := registry.CreateGitCommit(bp, username, registryCache)
			h.AssertNil(t, err)
		})

		it("should fail with incorrect buildpack format (namespace missing)", func() {
			bp := registry.Buildpack{
				Name:    "python",
				Version: "1.0.0",
				Yanked:  false,
				Address: "example.com",
			}
			err := registry.CreateGitCommit(bp, username, registryCache)
			h.AssertError(t, err, "writing (): empty buildpack namespace")
		})
	})
}
