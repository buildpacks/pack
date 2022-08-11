//go:build acceptance
// +build acceptance

package buildpacks

import (
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/testhelpers"
)

type BuildpackManager struct {
	testObject *testing.T
	assert     testhelpers.AssertionManager
	baseDir    string
	apiVersion string
}

type BuildpackManagerModifier func(b *BuildpackManager)

func WithBaseDir(baseDir string) func(b *BuildpackManager) {
	return func(b *BuildpackManager) {
		b.baseDir = baseDir
	}
}

func WithBuildpackAPIVersion(apiVersion string) func(b *BuildpackManager) {
	return func(b *BuildpackManager) {
		b.apiVersion = apiVersion
	}
}

func NewBuildpackManager(t *testing.T, assert testhelpers.AssertionManager, modifiers ...BuildpackManagerModifier) BuildpackManager {
	m := BuildpackManager{
		testObject: t,
		assert:     assert,
		baseDir:    filepath.Join("testdata", "mock_buildpacks"),
		apiVersion: builder.DefaultBuildpackAPIVersion,
	}

	for _, mod := range modifiers {
		mod(&m)
	}

	return m
}

type TestBuildpack interface {
	Prepare(source, destination string) error
}

func (b BuildpackManager) PrepareBuildpacks(destination string, buildpacks ...TestBuildpack) {
	b.testObject.Helper()

	for _, buildpack := range buildpacks {
		err := buildpack.Prepare(filepath.Join(b.baseDir, b.apiVersion), destination)
		b.assert.Nil(err)
	}
}

type Modifiable interface {
	SetPublish()
	SetBuildpacks([]TestBuildpack)
}
type PackageModifier func(p Modifiable)

func WithRequiredBuildpacks(buildpacks ...TestBuildpack) PackageModifier {
	return func(p Modifiable) {
		p.SetBuildpacks(buildpacks)
	}
}

func WithPublish() PackageModifier {
	return func(p Modifiable) {
		p.SetPublish()
	}
}
