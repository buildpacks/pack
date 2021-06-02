package assets

import (
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/testhelpers"
)

type Manager struct {
	sourceDir  string
	testObject *testing.T
	assert     testhelpers.AssertionManager
}

func NewManager(t *testing.T, sourceDir string) Manager {
	return Manager{
		sourceDir:  sourceDir,
		testObject: t,
		assert:     testhelpers.NewAssertionManager(t),
	}
}

func (a Manager) GetAssetWithFileName(assetName string) string {
	a.testObject.Helper()

	assetPath, err := filepath.Abs(filepath.Join("testdata", "mock_assets", assetName))
	a.assert.Nil(err)

	return testhelpers.CleanAbsPath(assetPath)
}
