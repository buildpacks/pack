// +build acceptance

package components

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"text/template"

	"github.com/buildpacks/pack/acceptance/variables"

	"github.com/buildpacks/pack/acceptance/assertions"

	"github.com/Masterminds/semver"

	h "github.com/buildpacks/pack/testhelpers"
)

type PackExecutor struct {
	testObject     *testing.T
	path           string
	fixturesPaths  []string
	home           string
	registryConfig *h.TestRegistryConfig
	assert         assertions.AssertionManager
}

func NewPackExecutor(
	testObject *testing.T,
	path string,
	fixtures []string,
	home string,
	registryConfig *h.TestRegistryConfig,
	assert assertions.AssertionManager,
) *PackExecutor {

	testObject.Helper()

	return &PackExecutor{
		testObject:     testObject,
		path:           path,
		fixturesPaths:  fixtures,
		home:           home,
		registryConfig: registryConfig,
		assert:         assert,
	}
}

func (e *PackExecutor) Cleanup() {
	e.testObject.Helper()

	err := os.RemoveAll(e.home)
	e.assert.Nil(err)
}

func (e *PackExecutor) cmd(name string, args ...string) *exec.Cmd {
	e.testObject.Helper()

	cmdArgs := append([]string{name}, args...)
	cmdArgs = append(cmdArgs, "--no-color")
	if e.Supports("--verbose") {
		cmdArgs = append(cmdArgs, "--verbose")
	}

	cmd := e.baseCmd(cmdArgs...)

	cmd.Env = append(os.Environ(), "DOCKER_CONFIG="+e.registryConfig.DockerConfigDir)
	if e.home != "" {
		cmd.Env = append(cmd.Env, "PACK_HOME="+e.home)
	}

	return cmd
}

func (e *PackExecutor) baseCmd(parts ...string) *exec.Cmd {
	return exec.Command(e.path, parts...)
}

func (e *PackExecutor) RunWithCombinedOutput(name string, args ...string) (string, error) {
	e.testObject.Helper()

	output, err := e.cmd(name, args...).CombinedOutput()

	return string(output), err
}

func (e *PackExecutor) SuccessfulRunWithOutput(name string, args ...string) string {
	e.testObject.Helper()

	output, err := e.RunWithCombinedOutput(name, args...)
	e.assert.NilWithMessage(err, output)

	return output
}

func (e *PackExecutor) SuccessfulRun(name string, args ...string) {
	e.testObject.Helper()

	_ = e.SuccessfulRunWithOutput(name, args...)
}

func (e *PackExecutor) StartWithWriter(combinedOutput *bytes.Buffer, name string, args ...string) InterruptCmd {
	cmd := e.cmd(name, args...)
	cmd.Stderr = combinedOutput
	cmd.Stdout = combinedOutput

	err := cmd.Start()
	e.assert.Nil(err)

	return InterruptCmd{
		testObject:     e.testObject,
		assert:         e.assert,
		cmd:            cmd,
		combinedOutput: combinedOutput,
	}
}

type InterruptCmd struct {
	testObject     *testing.T
	assert         assertions.AssertionManager
	cmd            *exec.Cmd
	combinedOutput *bytes.Buffer
	outputMux      sync.Mutex
}

func (c InterruptCmd) TerminateAtStep(pattern string) {
	c.testObject.Helper()

	for {
		c.outputMux.Lock()
		if strings.Contains(c.combinedOutput.String(), pattern) {
			err := c.cmd.Process.Signal(variables.InterruptSignal)
			c.assert.Nil(err)
			return
		}
		c.outputMux.Unlock()
	}
}

func (c InterruptCmd) Wait() error {
	return c.cmd.Wait()
}

func (e *PackExecutor) Version() string {
	e.testObject.Helper()

	output := e.SuccessfulRunWithOutput("version")

	return strings.TrimSpace(output)
}

func (e *PackExecutor) SetDefaultTrustedBuilder(name string) {
	e.testObject.Helper()

	e.SuccessfulRun("set-default-builder", name)
	if e.Supports("trust-builder") {
		e.SuccessfulRun("trust-builder", name)
	}
}

func (e *PackExecutor) Build(appImageName, appSourcePath string, additionalArgs ...string) {
	e.testObject.Helper()

	e.SuccessfulRun("build", append([]string{appImageName, "-p", appSourcePath}, additionalArgs...)...)
}

func (e *PackExecutor) EnableExperimental() {
	e.testObject.Helper()

	err := ioutil.WriteFile(
		filepath.Join(e.home, "config.toml"),
		[]byte("experimental=true"),
		os.ModePerm,
	)
	e.assert.Nil(err)
}

// supports returns whether or not the executor's pack binary supports a
// given command string. The command string can take one of three forms:
//   - "<command>" (e.g. "create-builder")
//   - "<flag>" (e.g. "--verbose")
//   - "<command> <flag>" (e.g. "build --network")
//
// Any other form will return false.
func (e *PackExecutor) Supports(command string) bool {
	e.testObject.Helper()

	parts := strings.Split(command, " ")

	var cmdParts = []string{"help"}

	var search string
	switch len(parts) {
	case 1:
		search = parts[0]
		break
	case 2:
		cmdParts = append(cmdParts, parts[0])
		search = parts[1]
	default:
		return false
	}

	output, err := e.baseCmd(cmdParts...).CombinedOutput()
	e.assert.Nil(err)

	return strings.Contains(string(output), search)
}

type feature int

const (
	BuilderTomlValidation feature = iota
	ExcludeAndIncludeDescriptor
	CreatorInPack
	CustomVolumeMounts
)

var featureTests = map[feature]func(e *PackExecutor) bool{
	BuilderTomlValidation: func(e *PackExecutor) bool {
		return e.laterThan090()
	},
	ExcludeAndIncludeDescriptor: func(e *PackExecutor) bool {
		return e.laterThan090()
	},
	CreatorInPack: func(e *PackExecutor) bool {
		return e.laterThan0_10_0()
	},
	CustomVolumeMounts: func(e *PackExecutor) bool {
		return e.not0_11_0()
	},
}

func (e *PackExecutor) SupportsFeature(f feature) bool {
	return featureTests[f](e)
}

func (e *PackExecutor) semanticVersion() *semver.Version {
	version := e.Version()
	semanticVersion, err := semver.NewVersion(strings.TrimPrefix(strings.Split(version, " ")[0], "v"))
	e.assert.Nil(err)

	return semanticVersion
}

func (e *PackExecutor) laterThan090() bool {
	ver := e.semanticVersion()
	return ver.Compare(semver.MustParse("0.9.0")) > 0 || ver.Equal(semver.MustParse("0.0.0"))
}

func (e *PackExecutor) laterThan0_10_0() bool {
	ver := e.semanticVersion()
	return ver.GreaterThan(semver.MustParse("0.10.0")) || ver.Equal(semver.MustParse("0.0.0"))
}

func (e *PackExecutor) not0_11_0() bool {
	ver := e.semanticVersion()
	return !ver.Equal(semver.MustParse("0.11.0"))
}

func (e *PackExecutor) FixtureMustExist(name string) string {
	e.testObject.Helper()

	for _, dir := range e.fixturesPaths {
		fixtureLocation := filepath.Join(dir, name)
		_, err := os.Stat(fixtureLocation)
		if !os.IsNotExist(err) {
			return fixtureLocation
		}
	}

	e.testObject.Fatalf("fixture %s does not exist in %v", name, e.fixturesPaths)

	return ""
}

func (e *PackExecutor) VersionedFixtureOrFixtureMustExist(pattern, version, fallback string) string {
	e.testObject.Helper()

	versionedName := fmt.Sprintf(pattern, version)

	for _, dir := range e.fixturesPaths {
		fixtureLocation := filepath.Join(dir, versionedName)
		_, err := os.Stat(fixtureLocation)
		if !os.IsNotExist(err) {
			return fixtureLocation
		}
	}

	return e.FixtureMustExist(fallback)
}

func (e *PackExecutor) PackageBuildpack(name, configPath string, args ...string) string {
	e.testObject.Helper()

	fullArgs := append([]string{name, "-p", configPath}, args...)

	return e.SuccessfulRunWithOutput("package-buildpack", fullArgs...)
}

func (e *PackExecutor) TemplateFixture(templateName string, templateData map[string]interface{}) string {
	e.testObject.Helper()

	outputTemplate, err := ioutil.ReadFile(e.FixtureMustExist(templateName))
	e.assert.Nil(err)

	return e.fillTemplate(outputTemplate, templateData)
}

func (e *PackExecutor) TemplateVersionedFixture(
	versionedPattern, version, fallback string,
	templateData map[string]interface{},
) string {

	e.testObject.Helper()

	outputTemplate, err := ioutil.ReadFile(e.VersionedFixtureOrFixtureMustExist(versionedPattern, version, fallback))
	e.assert.Nil(err)

	return e.fillTemplate(outputTemplate, templateData)
}

func (e *PackExecutor) fillTemplate(templateContents []byte, data map[string]interface{}) string {
	tpl, err := template.New("").Parse(string(templateContents))
	e.assert.Nil(err)

	var templatedContent bytes.Buffer
	err = tpl.Execute(&templatedContent, data)
	e.assert.Nil(err)

	return templatedContent.String()
}

func (e *PackExecutor) TemplateFixtureToFile(
	templateName string,
	destination *os.File,
	templateData map[string]interface{},
) {

	templatedContent := e.TemplateFixture(templateName, templateData)

	_, err := io.WriteString(destination, templatedContent)
	e.assert.Nil(err)
}

func (e *PackExecutor) AggregatePackageFixture(nestedPackageName, buildpackURI, tmpDir string) *os.File {
	e.testObject.Helper()

	e.testObject.Log("package w/ buildpacks and packages")

	aggregatePackageConfigFile, err := ioutil.TempFile(tmpDir, "package_aggregate-*.toml")
	e.assert.Nil(err)
	defer aggregatePackageConfigFile.Close()

	e.TemplateFixtureToFile(
		"package_aggregate.toml",
		aggregatePackageConfigFile,
		map[string]interface{}{
			"BuildpackURI": buildpackURI,
			"PackageName":  nestedPackageName,
		},
	)

	return aggregatePackageConfigFile
}

func (e *PackExecutor) ReadingFromVolumeMessage(path, output string) string {
	if e.laterThan090() {
		return fmt.Sprintf("Detect: Reading file '/platform%s': %s", path, output)
	}

	return fmt.Sprintf("Build: Reading file '/platform%s': %s", path, output)
}
