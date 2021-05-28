package assets

import (
	"errors"
	"github.com/buildpacks/pack/acceptance/buildpacks"
	"github.com/buildpacks/pack/testhelpers"
	"io/ioutil"
	"os"
	"testing"
)

type AssetManager struct {
	BpManager buildpacks.BuildpackManager
	TemplateAssets map[string]TestAsset
	OS string
	dest string
}

type TestAsset struct {
	path string
}

func NewAssetManager(t *testing.T, bpSource, OS string) AssetManager {
	return AssetManager{
		BpManager:      buildpacks.NewBuildpackManager(
			t,
			testhelpers.NewAssertionManager(t),
			buildpacks.WithBuildpackSource(bpSource),
		),
		TemplateAssets: nil,
	}
}

func (am *AssetManager) Open() (err error) {
	am.dest, err = ioutil.TempDir("", "asset-manager-")
	if err != nil {
		return err
	}

	return nil
}

func (am *AssetManager) Close() (err error) {
	if am.dest == "" {
		return errors.New("closing unopened AssetManager")
	}

	if err = os.RemoveAll(am.dest); err != nil {
		return err
	}

	am.dest = ""
	return nil
}