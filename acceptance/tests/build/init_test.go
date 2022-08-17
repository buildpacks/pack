//go:build acceptance
// +build acceptance

package build_test

import (
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/harness"
)

func TestBuild(t *testing.T) {
	testHarness := harness.ContainingBuilder(t, filepath.Join("..", "..", ".."))
	t.Cleanup(testHarness.CleanUp)

	// testHarness.RunA(test_runnable_rebuildable_inspectable)
	// testHarness.RunA(test_untrusted_daemon)
	// testHarness.RunA(test_untrusted_registry)
	// testHarness.RunA(test_no_color)
	// testHarness.RunA(test_app_zipfile)
	// testHarness.RunA(test_env_file)
	testHarness.RunA(test_env_vars)
}
