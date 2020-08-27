// +build acceptance

package buildpacks

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"

	h "github.com/buildpacks/pack/testhelpers"

	"github.com/buildpacks/pack/acceptance/invoke"
)

type PackageImage struct {
	testObject           *testing.T
	pack                 *invoke.PackInvoker
	name                 string
	sourceConfigLocation string
	buildpacks           []TestBuildpack
	publish              bool
}

type PackageImageModifier func(p *PackageImage)

func WithRequiredBuildpacks(buildpacks ...TestBuildpack) PackageImageModifier {
	return func(p *PackageImage) {
		p.buildpacks = buildpacks
	}
}

func WithPublish() PackageImageModifier {
	return func(p *PackageImage) {
		p.publish = true
	}
}

func NewPackageImage(
	t *testing.T,
	pack *invoke.PackInvoker,
	name, configLocation string,
	modifiers ...PackageImageModifier,
) PackageImage {
	p := PackageImage{
		testObject:           t,
		pack:                 pack,
		name:                 name,
		sourceConfigLocation: configLocation,
		publish:              false,
	}

	for _, mod := range modifiers {
		mod(&p)
	}

	return p
}

func (p PackageImage) Prepare(sourceDir, destination string) error {
	p.testObject.Helper()
	p.testObject.Log("creating package image...")

	tmpDir, err := ioutil.TempDir("", "package-buildpacks")
	if err != nil {
		return fmt.Errorf("creating temp dir for package buildpacks: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, buildpack := range p.buildpacks {
		err = buildpack.Prepare(sourceDir, tmpDir)
		if err != nil {
			return fmt.Errorf("preparing buildpack %s: %w", buildpack, err)
		}
	}

	configLocation := filepath.Join(tmpDir, "package.toml")
	h.CopyFile(p.testObject, p.sourceConfigLocation, configLocation)

	packArgs := []string{
		p.name,
		"--no-color",
		"-c", configLocation,
	}

	if p.publish {
		packArgs = append(packArgs, "--publish")
	}

	output := p.pack.RunSuccessfully("package-buildpack", packArgs...)

	assertOutput := assertions.NewOutputAssertionManager(p.testObject, output)
	if p.publish {
		assertOutput.ReportsPackagePublished(p.name)
	} else {
		assertOutput.ReportsPackageCreation(p.name)
	}

	return nil
}
