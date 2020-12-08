// +build acceptance

package invoke

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Masterminds/semver"

	acceptanceOS "github.com/buildpacks/pack/acceptance/os"
	h "github.com/buildpacks/pack/testhelpers"
)

type PackInvoker struct {
	testObject      *testing.T
	assert          h.AssertionManager
	path            string
	home            string
	dockerConfigDir string
	fixtureManager  PackFixtureManager
	verbose         bool
}

type packPathsProvider interface {
	Path() string
	FixturePaths() []string
}

func NewPackInvoker(
	testObject *testing.T,
	assert h.AssertionManager,
	packAssets packPathsProvider,
	dockerConfigDir string,
) *PackInvoker {

	testObject.Helper()

	home, err := ioutil.TempDir("", "buildpack.pack.home.")
	if err != nil {
		testObject.Fatalf("couldn't create home folder for pack: %s", err)
	}

	return &PackInvoker{
		testObject:      testObject,
		assert:          assert,
		path:            packAssets.Path(),
		home:            home,
		dockerConfigDir: dockerConfigDir,
		verbose:         true,
		fixtureManager: PackFixtureManager{
			testObject: testObject,
			assert:     assert,
			locations:  packAssets.FixturePaths(),
		},
	}
}

func (i *PackInvoker) Cleanup() {
	i.testObject.Helper()

	err := os.RemoveAll(i.home)
	i.assert.Nil(err)
}

func (i *PackInvoker) cmd(name string, args ...string) *exec.Cmd {
	i.testObject.Helper()

	cmdArgs := append([]string{name}, args...)
	cmdArgs = append(cmdArgs, "--no-color")
	if i.verbose && i.Supports("--verbose") {
		cmdArgs = append(cmdArgs, "--verbose")
	}

	cmd := i.baseCmd(cmdArgs...)

	cmd.Env = append(os.Environ(), "DOCKER_CONFIG="+i.dockerConfigDir)
	if i.home != "" {
		cmd.Env = append(cmd.Env, "PACK_HOME="+i.home)
	}

	return cmd
}

func (i *PackInvoker) baseCmd(parts ...string) *exec.Cmd {
	return exec.Command(i.path, parts...)
}

func (i *PackInvoker) Run(name string, args ...string) (string, error) {
	i.testObject.Helper()

	output, err := i.cmd(name, args...).CombinedOutput()

	return string(output), err
}

func (i *PackInvoker) SetVerbose(verbose bool) {
	i.verbose = verbose
}

func (i *PackInvoker) RunSuccessfully(name string, args ...string) string {
	i.testObject.Helper()

	output, err := i.Run(name, args...)
	i.assert.NilWithMessage(err, output)

	return output
}

func (i *PackInvoker) JustRunSuccessfully(name string, args ...string) {
	i.testObject.Helper()

	_ = i.RunSuccessfully(name, args...)
}

func (i *PackInvoker) StartWithWriter(combinedOutput *bytes.Buffer, name string, args ...string) *InterruptCmd {
	cmd := i.cmd(name, args...)
	cmd.Stderr = combinedOutput
	cmd.Stdout = combinedOutput

	err := cmd.Start()
	i.assert.Nil(err)

	return &InterruptCmd{
		testObject:     i.testObject,
		assert:         i.assert,
		cmd:            cmd,
		combinedOutput: combinedOutput,
	}
}

type InterruptCmd struct {
	testObject     *testing.T
	assert         h.AssertionManager
	cmd            *exec.Cmd
	combinedOutput *bytes.Buffer
	outputMux      sync.Mutex
}

func (c *InterruptCmd) TerminateAtStep(pattern string) {
	c.testObject.Helper()

	for {
		c.outputMux.Lock()
		if strings.Contains(c.combinedOutput.String(), pattern) {
			err := c.cmd.Process.Signal(acceptanceOS.InterruptSignal)
			c.assert.Nil(err)
			h.AssertNil(c.testObject, err)
			return
		}
		c.outputMux.Unlock()
	}
}

func (c *InterruptCmd) Wait() error {
	return c.cmd.Wait()
}

func (i *PackInvoker) Version() string {
	i.testObject.Helper()

	output := i.RunSuccessfully("version")

	return strings.TrimSpace(output)
}

func (i *PackInvoker) EnableExperimental() {
	i.testObject.Helper()

	err := ioutil.WriteFile(
		filepath.Join(i.home, "config.toml"),
		[]byte("experimental=true"),
		os.ModePerm,
	)
	i.assert.Nil(err)
}

// supports returns whether or not the executor's pack binary supports a
// given command string. The command string can take one of four forms:
//   - "<command>" (e.g. "create-builder")
//   - "<flag>" (e.g. "--verbose")
//   - "<command> <flag>" (e.g. "build --network")
//   - "<command>... <flag>" (e.g. "config trusted-builder--network")
//
// Any other form may return false.
func (i *PackInvoker) Supports(command string) bool {
	i.testObject.Helper()

	parts := strings.Split(command, " ")

	var cmdParts = []string{"help"}
	last := len(parts) - 1
	cmdParts = append(cmdParts, parts[:last]...)
	search := parts[last]

	output, err := i.baseCmd(cmdParts...).CombinedOutput()
	i.assert.Nil(err)

	return strings.Contains(string(output), search) && !strings.Contains(string(output), "Unknown help topic")
}

type Feature int

const (
	BuilderTomlValidation Feature = iota
	ExcludeAndIncludeDescriptor
	CreatorInPack
	ReadWriteVolumeMounts
	NoColorInBuildpacks
	QuietMode
	InspectBuilderOutputFormat
	OSInPackageTOML
)

var featureTests = map[Feature]func(i *PackInvoker) bool{
	BuilderTomlValidation: func(i *PackInvoker) bool {
		return i.laterThan("0.9.0")
	},
	ExcludeAndIncludeDescriptor: func(i *PackInvoker) bool {
		return i.laterThan("0.9.0")
	},
	CreatorInPack: func(i *PackInvoker) bool {
		return i.atLeast("0.10.0")
	},
	ReadWriteVolumeMounts: func(i *PackInvoker) bool {
		return i.laterThan("0.12.0")
	},
	NoColorInBuildpacks: func(i *PackInvoker) bool {
		return i.atLeast("0.12.0")
	},
	QuietMode: func(i *PackInvoker) bool {
		return i.atLeast("0.13.2")
	},
	InspectBuilderOutputFormat: func(i *PackInvoker) bool {
		return i.laterThan("0.14.2")
	},
	OSInPackageTOML: func(i *PackInvoker) bool {
		return i.laterThan("0.15.0")
	},
}

func (i *PackInvoker) SupportsFeature(f Feature) bool {
	return featureTests[f](i)
}

func (i *PackInvoker) semanticVersion() *semver.Version {
	version := i.Version()
	semanticVersion, err := semver.NewVersion(strings.TrimPrefix(strings.Split(version, " ")[0], "v"))
	i.assert.Nil(err)

	return semanticVersion
}

// laterThan returns true if pack version is older than the provided version
func (i *PackInvoker) laterThan(version string) bool {
	providedVersion := semver.MustParse(version)
	ver := i.semanticVersion()
	return ver.Compare(providedVersion) > 0 || ver.Equal(semver.MustParse("0.0.0"))
}

// atLeast returns true if pack version is the same or older than the provided version
func (i *PackInvoker) atLeast(version string) bool {
	minimalVersion := semver.MustParse(version)
	ver := i.semanticVersion()
	return ver.Equal(minimalVersion) || ver.GreaterThan(minimalVersion) || ver.Equal(semver.MustParse("0.0.0"))
}

func (i *PackInvoker) ConfigFileContents() string {
	i.testObject.Helper()

	contents, err := ioutil.ReadFile(filepath.Join(i.home, "config.toml"))
	i.assert.Nil(err)

	return string(contents)
}

func (i *PackInvoker) FixtureManager() PackFixtureManager {
	return i.fixtureManager
}
