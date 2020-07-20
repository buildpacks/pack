// +build acceptance

package config

type PackAssetManager struct {
	path         string
	fixturePaths []string
}

func (a AssetManager) NewPackAssetManager(kind ComboValue) PackAssetManager {
	path, fixtures := a.PackPaths(kind)

	return PackAssetManager{
		path:         path,
		fixturePaths: fixtures,
	}
}

func (p PackAssetManager) Path() string {
	return p.path
}

func (p PackAssetManager) FixturePaths() []string {
	return p.fixturePaths
}
