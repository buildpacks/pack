// +build acceptance

package components

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver"

	"github.com/buildpacks/pack/internal/builder"
)

type TestLifecycle struct {
	path       string
	descriptor builder.LifecycleDescriptor
}

func NewTestLifecycle(path string, descriptor builder.LifecycleDescriptor) *TestLifecycle {
	return &TestLifecycle{
		path:       path,
		descriptor: descriptor,
	}
}

func (l *TestLifecycle) EscapedPath() string {
	return strings.ReplaceAll(l.path, `\`, `\\`)
}

func (l *TestLifecycle) ShouldShowReference() bool {
	return !l.descriptor.Info.Version.LessThan(semver.MustParse("0.5.0"))
}

func (l *TestLifecycle) ShouldShowProcesses() bool {
	return !l.pre060()
}

type lifecycleFeature int

const (
	DefaultProcess lifecycleFeature = iota
	CreatorInLifecycle
	DetailedCacheLogging
)

var lifecycleFeatureTests = map[lifecycleFeature]func(l *TestLifecycle) bool{
	DefaultProcess: func(l *TestLifecycle) bool {
		return l.atLeast070()
	},
	CreatorInLifecycle: func(l *TestLifecycle) bool {
		return l.atLeast074()
	},
	DetailedCacheLogging: func(l *TestLifecycle) bool {
		return !l.pre060()
	},
}

func (l *TestLifecycle) Version() string {
	return l.descriptor.Info.Version.String()
}

func (l *TestLifecycle) BuildpackAPIVersion() string {
	return l.descriptor.API.BuildpackVersion.String()
}

func (l *TestLifecycle) PlatformAPIVersion() string {
	return l.descriptor.API.PlatformVersion.String()
}

func (l *TestLifecycle) SupportsFeature(f lifecycleFeature) bool {
	return lifecycleFeatureTests[f](l)
}

func (l *TestLifecycle) DoesntSupportFeature(f lifecycleFeature) bool {
	return !l.SupportsFeature(f)
}

func (l *TestLifecycle) pre060() bool {
	return l.descriptor.Info.Version.LessThan(semver.MustParse("0.6.0"))
}

func (l *TestLifecycle) atLeast070() bool {
	return !l.descriptor.Info.Version.LessThan(semver.MustParse("0.7.0"))
}

func (l *TestLifecycle) atLeast074() bool {
	return !l.descriptor.Info.Version.LessThan(semver.MustParse("0.7.4"))
}

func (l *TestLifecycle) BuilderConfigBlock() string {
	return fmt.Sprintf(`
[lifecycle]
  uri = "%s"
  version = "%s"
`, l.EscapedPath(), l.Version())
}
