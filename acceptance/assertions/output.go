// +build acceptance

package assertions

import (
	"fmt"
	"regexp"
	"testing"
)

type pack interface {
	ReadingFromVolumeMessage(path, output string) string
}

type OutputAssertionManager struct {
	testObject *testing.T
	assert     AssertionManager
	output     string
}

func (a AssertionManager) NewOutputAssertionManager(output string) OutputAssertionManager {
	return OutputAssertionManager{
		testObject: a.testObject,
		assert:     a,
		output:     output,
	}
}

func (o OutputAssertionManager) ReportsSuccessfulImageBuild(name string) {
	o.testObject.Helper()

	o.assert.Contains(o.output, fmt.Sprintf("Successfully built image '%s'", name))
}

func (o OutputAssertionManager) ReportsSuccessfulRebase(name string) {
	o.testObject.Helper()

	o.assert.Contains(o.output, fmt.Sprintf("Successfully rebased image '%s'", name))
}

func (o OutputAssertionManager) ReportsUsingBuildCacheVolume() {
	o.testObject.Helper()

	o.testObject.Log("uses a build cache volume")
	o.assert.Contains(o.output, "Using build cache volume")
}

func (o OutputAssertionManager) ReportsSelectingRunImageMirror(mirror string) {
	o.testObject.Helper()

	o.testObject.Log("selects expected run image mirror")
	o.assert.Contains(o.output, fmt.Sprintf("Selected run image mirror '%s'", mirror))
}

func (o OutputAssertionManager) ReportsSelectingRunImageMirrorFromLocalConfig(mirror string) {
	o.testObject.Helper()

	o.testObject.Log("local run-image mirror is selected")
	o.assert.Contains(o.output, fmt.Sprintf("Selected run image mirror '%s' from local config", mirror))
}

func (o OutputAssertionManager) ReportsReadingFileContents(path, expectedContent string, pack pack) {
	o.testObject.Helper()

	o.assert.Contains(o.output, pack.ReadingFromVolumeMessage(path, expectedContent))
}

func (o OutputAssertionManager) ReportsBuildStep(message string) {
	o.testObject.Helper()

	o.assert.Contains(o.output, fmt.Sprintf("Build: %s", message))
}

func (o OutputAssertionManager) ReportsAddingBuildpack(name, version string) {
	o.testObject.Helper()

	o.assert.Contains(o.output, fmt.Sprintf("Adding buildpack '%s' version '%s' to builder", name, version))
}

func (o OutputAssertionManager) ReportsPullingImage(image string) {
	o.testObject.Helper()

	o.assert.Contains(o.output, fmt.Sprintf("Pulling image '%s'", image))
}

func (o OutputAssertionManager) ReportsRunImageStackNotMatchingBuilder(runImageStack, builderStack string) {
	o.testObject.Helper()

	o.assert.Contains(
		o.output,
		fmt.Sprintf("run-image stack id '%s' does not match builder stack '%s'", runImageStack, builderStack),
	)
}

func (o OutputAssertionManager) ReportsSettingDefaultBuilder(name string) {
	o.testObject.Helper()

	o.assert.Contains(o.output, fmt.Sprintf("Builder '%s' is now the default builder", name))
}

func (o OutputAssertionManager) IncludesSuggestedBuildersHeading() {
	o.testObject.Helper()

	o.assert.Contains(o.output, "Suggested builders:")
}

func (o OutputAssertionManager) IncludesMessageToSetDefaultBuilder() {
	o.testObject.Helper()

	o.assert.Contains(o.output, "Please select a default builder with:")
}

func (o OutputAssertionManager) IncludesGoogleBuilder() {
	o.testObject.Helper()

	o.assert.Matches(o.output, regexp.MustCompile(`Google:\s+'gcr.io/buildpacks/builder'`))
}

func (o OutputAssertionManager) IncludesHerokuBuilder() {
	o.testObject.Helper()

	o.assert.Matches(o.output, regexp.MustCompile(`Heroku:\s+'heroku/buildpacks:18'`))
}

func (o OutputAssertionManager) IncludesPaketoBuilders() {
	o.testObject.Helper()

	o.assert.MatchesAll(
		o.output,
		regexp.MustCompile(`Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:base'`),
		regexp.MustCompile(`Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:full-cf'`),
		regexp.MustCompile(`Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:tiny'`),
	)
}

func (o OutputAssertionManager) IncludesSuggestedStacksHeading() {
	o.testObject.Helper()

	o.assert.Contains(o.output, "Stacks maintained by the community:")
}

func (o OutputAssertionManager) ReportsWindowsContainersExperimental() {
	o.testObject.Helper()

	o.assert.Contains(o.output, "Windows containers support is currently experimental")
}

type mixedComponentMessageManager interface {
	CacheRestorationMessagePatterns(cachedLayer string) []*regexp.Regexp
	CacheReuseMessagePattern(cachedLayer string) *regexp.Regexp
	CacheCreationMessagePattern(cachedLayer string) *regexp.Regexp
	ConnectedToTheInternetMessages() []string
	DisconnectedFromInternetMessages() []string
	MessagePatternWithPhase(message, phase string) string
	UsingCreator() bool
}

type MixedComponentOutputAssertionManager struct {
	testObject         *testing.T
	assert             AssertionManager
	componentMessenger mixedComponentMessageManager
	output             string
}

func (a AssertionManager) NewMixedComponentOutputAssertionManager(
	output string,
	messenger mixedComponentMessageManager,
) MixedComponentOutputAssertionManager {

	return MixedComponentOutputAssertionManager{
		testObject:         a.testObject,
		assert:             a,
		componentMessenger: messenger,
		output:             output,
	}
}

func (m MixedComponentOutputAssertionManager) ReportsRestoresCachedLayer(layer string) {
	m.testObject.Helper()
	m.testObject.Log("restores the cache")

	patterns := m.componentMessenger.CacheRestorationMessagePatterns(layer)
	m.assert.MatchesAll(m.output, patterns...)
}

func (m MixedComponentOutputAssertionManager) ReportsCacheReuse(layer string) {
	m.testObject.Helper()
	m.testObject.Log("reusing unchanged cached layers")

	m.assert.Matches(m.output, m.componentMessenger.CacheReuseMessagePattern(layer))
}

func (m MixedComponentOutputAssertionManager) ReportsCacheCreation(layer string) {
	m.testObject.Helper()
	m.testObject.Log("cacher adds layers")

	m.assert.Matches(m.output, m.componentMessenger.CacheCreationMessagePattern(layer))
}

func (m MixedComponentOutputAssertionManager) ReportsExporterReusesUnchangedLayer(layer string) {
	m.testObject.Helper()
	m.testObject.Log("exporter reuses unchanged layers")

	m.assert.Matches(m.output, regexp.MustCompile(m.componentMessenger.MessagePatternWithPhase(
		fmt.Sprintf("reusing layer '%s'", layer),
		"exporter",
	)))
}

func (m MixedComponentOutputAssertionManager) ReportsSkippingRestore() {
	m.testObject.Helper()
	m.testObject.Log("skips buildpack layer analysis")

	if !m.componentMessenger.UsingCreator() {
		m.assert.Contains(m.output, "Skipping 'restore' due to clearing cache")
	}
}

func (m MixedComponentOutputAssertionManager) ReportsSkippingBuildpackLayerAnalysis() {
	m.testObject.Helper()
	m.testObject.Log("skips buildpack layer analysis")

	m.assert.Matches(m.output, regexp.MustCompile(m.componentMessenger.MessagePatternWithPhase(
		"Skipping buildpack layer analysis",
		"analyzer",
	)))
}

func (m MixedComponentOutputAssertionManager) ReportsConnectedToInternet() {
	m.testObject.Helper()

	m.assert.ContainsAll(m.output, m.componentMessenger.ConnectedToTheInternetMessages()...)
}

func (m MixedComponentOutputAssertionManager) ReportsDisconnectedFromInternet() {
	m.testObject.Helper()

	m.assert.ContainsAll(m.output, m.componentMessenger.DisconnectedFromInternetMessages()...)
}
