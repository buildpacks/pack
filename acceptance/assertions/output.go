// +build acceptance

package assertions

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	h "github.com/buildpacks/pack/testhelpers"
)

type OutputAssertionManager struct {
	testObject *testing.T
	assert     h.AssertionManager
	output     string
}

func NewOutputAssertionManager(t *testing.T, output string) OutputAssertionManager {
	return OutputAssertionManager{
		testObject: t,
		assert:     h.NewAssertionManager(t),
		output:     output,
	}
}

func (o OutputAssertionManager) ReportsSuccessfulImageBuild(name string) {
	o.testObject.Helper()

	o.assert.ContainsF(o.output, "Successfully built image '%s'", name)
}

func (o OutputAssertionManager) ReportSuccessfulQuietBuild(name string) {
	o.testObject.Helper()
	o.testObject.Log("quiet mode")

	o.assert.Matches(strings.TrimSpace(o.output), regexp.MustCompile(name+`@sha256:[\w]{64}`))
}

func (o OutputAssertionManager) ReportsSuccessfulRebase(name string) {
	o.testObject.Helper()

	o.assert.ContainsF(o.output, "Successfully rebased image '%s'", name)
}

func (o OutputAssertionManager) ReportsUsingBuildCacheVolume() {
	o.testObject.Helper()

	o.testObject.Log("uses a build cache volume")
	o.assert.Contains(o.output, "Using build cache volume")
}

func (o OutputAssertionManager) ReportsSelectingRunImageMirror(mirror string) {
	o.testObject.Helper()

	o.testObject.Log("selects expected run image mirror")
	o.assert.ContainsF(o.output, "Selected run image mirror '%s'", mirror)
}

func (o OutputAssertionManager) ReportsSelectingRunImageMirrorFromLocalConfig(mirror string) {
	o.testObject.Helper()

	o.testObject.Log("local run-image mirror is selected")
	o.assert.ContainsF(o.output, "Selected run image mirror '%s' from local config", mirror)
}

func (o OutputAssertionManager) ReportsSkippingRestore() {
	o.testObject.Helper()
	o.testObject.Log("skips restore")

	o.assert.Contains(o.output, "Skipping 'restore' due to clearing cache")
}

func (o OutputAssertionManager) ReportsRunImageStackNotMatchingBuilder(runImageStack, builderStack string) {
	o.testObject.Helper()

	o.assert.Contains(
		o.output,
		fmt.Sprintf("run-image stack id '%s' does not match builder stack '%s'", runImageStack, builderStack),
	)
}

func (o OutputAssertionManager) WithoutColors() {
	o.testObject.Helper()
	o.testObject.Log("has no color")

	o.assert.NoMatches(o.output, regexp.MustCompile(`\x1b\[[0-9;]*m`))
}

func (o OutputAssertionManager) ReportsAddingBuildpack(name, version string) {
	o.testObject.Helper()

	o.assert.ContainsF(o.output, "Adding buildpack '%s' version '%s' to builder", name, version)
}

func (o OutputAssertionManager) ReportsPullingImage(image string) {
	o.testObject.Helper()

	o.assert.ContainsF(o.output, "Pulling image '%s'", image)
}

func (o OutputAssertionManager) ReportsImageNotExistingOnDaemon(image string) {
	o.testObject.Helper()

	o.assert.ContainsF(o.output, "image '%s' does not exist on the daemon", image)
}

func (o OutputAssertionManager) ReportsPackageCreation(name string) {
	o.testObject.Helper()

	o.assert.ContainsF(o.output, "Successfully created package '%s'", name)
}

func (o OutputAssertionManager) ReportsPackagePublished(name string) {
	o.testObject.Helper()

	o.assert.ContainsF(o.output, "Successfully published package '%s'", name)
}

func (o OutputAssertionManager) ReportsCommandUnknown(command string) {
	o.testObject.Helper()

	o.assert.ContainsF(o.output, `unknown command "%s" for "pack"`, command)
}

func (o OutputAssertionManager) IncludesUsagePrompt() {
	o.testObject.Helper()

	o.assert.Contains(o.output, "Run 'pack --help' for usage.")
}

func (o OutputAssertionManager) ReportsSettingDefaultBuilder(name string) {
	o.testObject.Helper()

	o.assert.ContainsF(o.output, "Builder '%s' is now the default builder", name)
}

func (o OutputAssertionManager) IncludesSuggestedBuildersHeading() {
	o.testObject.Helper()

	o.assert.Contains(o.output, "Suggested builders:")
}

func (o OutputAssertionManager) IncludesMessageToSetDefaultBuilder() {
	o.testObject.Helper()

	o.assert.Contains(o.output, "Please select a default builder with:")
}

func (o OutputAssertionManager) IncludesSuggestedStacksHeading() {
	o.testObject.Helper()

	o.assert.Contains(o.output, "Stacks maintained by the community:")
}

func (o OutputAssertionManager) IncludesTrustedBuildersHeading() {
	o.testObject.Helper()

	o.assert.Contains(o.output, "Trusted Builders:")
}

const googleBuilder = "gcr.io/buildpacks/builder:v1"

func (o OutputAssertionManager) IncludesGoogleBuilder() {
	o.testObject.Helper()

	o.assert.Contains(o.output, googleBuilder)
}

func (o OutputAssertionManager) IncludesPrefixedGoogleBuilder() {
	o.testObject.Helper()

	o.assert.Matches(o.output, regexp.MustCompile(fmt.Sprintf(`Google:\s+'%s'`, googleBuilder)))
}

const herokuBuilder = "heroku/buildpacks:18"

func (o OutputAssertionManager) IncludesHerokuBuilder() {
	o.testObject.Helper()

	o.assert.Contains(o.output, herokuBuilder)
}

func (o OutputAssertionManager) IncludesPrefixedHerokuBuilder() {
	o.testObject.Helper()

	o.assert.Matches(o.output, regexp.MustCompile(fmt.Sprintf(`Heroku:\s+'%s'`, herokuBuilder)))
}

var paketoBuilders = []string{
	"paketobuildpacks/builder:base",
	"paketobuildpacks/builder:full",
	"paketobuildpacks/builder:tiny",
}

func (o OutputAssertionManager) IncludesPaketoBuilders() {
	o.testObject.Helper()

	o.assert.ContainsAll(o.output, paketoBuilders...)
}

func (o OutputAssertionManager) IncludesPrefixedPaketoBuilders() {
	o.testObject.Helper()

	for _, builder := range paketoBuilders {
		o.assert.Matches(o.output, regexp.MustCompile(fmt.Sprintf(`Paketo Buildpacks:\s+'%s'`, builder)))
	}
}

func (o OutputAssertionManager) IncludesDeprecationWarning() {
	o.testObject.Helper()

	o.assert.Matches(o.output, regexp.MustCompile(fmt.Sprintf(`Warning: Command 'pack [\w-]+' has been deprecated, please use 'pack [\w-\s]+' instead`)))
}
