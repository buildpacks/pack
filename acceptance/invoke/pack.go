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
	path            string
	home            string
	dockerConfigDir string
	fixtureManager  PackFixtureManager
}

type packPathsProvider interface {
	Path() string
	FixturePaths() []string
}

func NewPackInvoker(testObject *testing.T, packAssets packPathsProvider, dockerConfigDir string) *PackInvoker {
	testObject.Helper()

	home, err := ioutil.TempDir("", "buildpack.pack.home.")
	if err != nil {
		testObject.Fatalf("couldn't create home folder for pack: %s", err)
	}

	return &PackInvoker{
		testObject:      testObject,
		path:            packAssets.Path(),
		home:            home,
		dockerConfigDir: dockerConfigDir,
		fixtureManager: PackFixtureManager{
			testObject: testObject,
			locations:  packAssets.FixturePaths(),
		},
	}
}

func (i *PackInvoker) Cleanup() {
	i.testObject.Helper()

	err := os.RemoveAll(i.home)
	h.AssertNil(i.testObject, err)
}

func (i *PackInvoker) cmd(name string, args ...string) *exec.Cmd {
	i.testObject.Helper()

	cmdArgs := append([]string{name}, args...)
	cmdArgs = append(cmdArgs, "--no-color")
	if i.Supports("--verbose") {
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

func (i *PackInvoker) RunSuccessfully(name string, args ...string) string {
	i.testObject.Helper()

	output, err := i.Run(name, args...)
	h.AssertNil(i.testObject, err)

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
	h.AssertNil(i.testObject, err)

	return &InterruptCmd{
		testObject:     i.testObject,
		cmd:            cmd,
		combinedOutput: combinedOutput,
	}
}

type InterruptCmd struct {
	testObject     *testing.T
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
	h.AssertNil(i.testObject, err)
}

// supports returns whether or not the executor's pack binary supports a
// given command string. The command string can take one of three forms:
//   - "<command>" (e.g. "create-builder")
//   - "<flag>" (e.g. "--verbose")
//   - "<command> <flag>" (e.g. "build --network")
//
// Any other form will return false.
func (i *PackInvoker) Supports(command string) bool {
	i.testObject.Helper()

	parts := strings.Split(command, " ")

	var cmdParts = []string{"help"}

	var search string
	switch len(parts) {
	case 1:
		search = parts[0]
	case 2:
		cmdParts = append(cmdParts, parts[0])
		search = parts[1]
	default:
		return false
	}

	output, err := i.baseCmd(cmdParts...).CombinedOutput()
	h.AssertNil(i.testObject, err)

	return strings.Contains(string(output), search)
}

type Feature int

const (
	BuilderTomlValidation Feature = iota
	ExcludeAndIncludeDescriptor
	CreatorInPack
	CustomVolumeMounts
	ReadWriteVolumeMounts
	NoColorInBuildpacks
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
	CustomVolumeMounts: func(i *PackInvoker) bool {
		return i.laterThan("0.11.0")
	},
	ReadWriteVolumeMounts: func(i *PackInvoker) bool {
		return i.laterThan("0.12.0")
	},
	NoColorInBuildpacks: func(i *PackInvoker) bool {
		return i.atLeast("0.12.0")
	},
}

func (i *PackInvoker) SupportsFeature(f Feature) bool {
	return featureTests[f](i)
}

func (i *PackInvoker) semanticVersion() *semver.Version {
	version := i.Version()
	semanticVersion, err := semver.NewVersion(strings.TrimPrefix(strings.Split(version, " ")[0], "v"))
	h.AssertNil(i.testObject, err)

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
	h.AssertNil(i.testObject, err)

	return string(contents)
}

func (i *PackInvoker) FixtureManager() PackFixtureManager {
	return i.fixtureManager
}
