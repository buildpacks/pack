//go:build acceptance
// +build acceptance

package build_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/config"
	"github.com/buildpacks/pack/acceptance/harness"
	"github.com/buildpacks/pack/acceptance/invoke"
	h "github.com/buildpacks/pack/testhelpers"
)

func test_arg_creation_time(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	registry := th.Registry()
	imageManager := th.ImageManager()
	assertImage := assertions.NewImageAssertionManager(t, imageManager, &registry)

	pack := combo.Pack()
	lifecycle := combo.Lifecycle()

	h.SkipIf(t, !pack.SupportsFeature(invoke.CreationTime), "pack doesn't support creation time")
	h.SkipIf(t, !lifecycle.SupportsFeature(config.CreationTime), "lifecycle doesn't support creation time")

	appPath := filepath.Join("..", "..", "testdata", "mock_app")

	t.Run("provided as 'now'", func(t *testing.T) {
		repoName := registry.RepoName("sample/" + h.RandString(10))
		expectedTime := time.Now()
		pack.RunSuccessfully(
			"build", repoName,
			"-p", appPath,
			"--creation-time", "now",
		)
		assertImage.HasCreateTime(repoName, expectedTime)
	})

	t.Run("provided as unix timestamp", func(t *testing.T) {
		repoName := registry.RepoName("sample/" + h.RandString(10))
		pack.RunSuccessfully(
			"build", repoName,
			"-p", appPath,
			"--creation-time", "1566172801",
		)
		expectedTime, err := time.Parse("2006-01-02T03:04:05Z", "2019-08-19T00:00:01Z")
		h.AssertNil(t, err)
		assertImage.HasCreateTime(repoName, expectedTime)
	})

	t.Run("not provided", func(t *testing.T) {
		repoName := registry.RepoName("sample/" + h.RandString(10))
		pack.RunSuccessfully(
			"build", repoName,
			"-p", appPath,
		)
		expectedTime, err := time.Parse("2006-01-02T03:04:05Z", "1980-01-01T00:00:01Z")
		h.AssertNil(t, err)
		assertImage.HasCreateTime(repoName, expectedTime)
	})
}
