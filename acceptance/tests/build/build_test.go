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

	testHarness.RunA(test_app_image_is_runnable_and_rebuildable)
	testHarness.RunA(test_app_image_is_inspectable)
	testHarness.RunA(test_untrusted_daemon)
	testHarness.RunA(test_untrusted_registry)
	testHarness.RunA(test_arg_app_zipfile)
	testHarness.RunA(test_arg_creation_time)
	testHarness.RunA(test_arg_env_file)
	testHarness.RunA(test_arg_env_vars)
	testHarness.RunA(test_arg_no_color)
	testHarness.RunA(test_arg_buildpack)
}
