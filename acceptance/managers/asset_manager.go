// +build acceptance

package managers

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/acceptance/variables"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/style"
)

const defaultCompilePackVersion = "0.0.0"

var (
	currentPackFixturesDir           = filepath.Join("testdata", "pack_fixtures")
	previousPackFixturesOverridesDir = filepath.Join("testdata", "pack_previous_fixtures_overrides")
	githubAssetFetcher               *GithubAssetFetcher
	lifecycleTgzExp                  = regexp.MustCompile(`lifecycle-v\d+.\d+.\d+\+linux.x86-64.tgz`)
)

type AssetManager struct {
	packPath                    string
	packFixturesPath            string
	previousPackPath            string
	previousPackFixturesPaths   []string
	lifecyclePath               string
	lifecycleDescriptor         builder.LifecycleDescriptor
	previousLifecyclePath       string
	previousLifecycleDescriptor builder.LifecycleDescriptor
	testObject                  *testing.T
}

func ConvergedAssetManager(t *testing.T, inputConfig InputConfigurationManager) (AssetManager, error) {
	t.Helper()

	var (
		convergedCurrentPackPath             string
		convergedPreviousPackPath            string
		convergedPreviousPackFixturesPaths   []string
		convergedDefaultLifecyclePath        string
		convergedDefaultLifecycleDescriptor  builder.LifecycleDescriptor
		convergedPreviousLifecyclePath       string
		convergedPreviousLifecycleDescriptor builder.LifecycleDescriptor
	)

	assetBuilder := assetManagerBuilder{
		testObject:  t,
		inputConfig: inputConfig,
	}

	if inputConfig.combinations.requiresCurrentPack() {
		var err error
		convergedCurrentPackPath, err = assetBuilder.ensureCurrentPack()
		if err != nil {
			return AssetManager{}, errors.Wrap(err, "ensuring a current pack executable exists")
		}
	}

	if inputConfig.combinations.requiresPreviousPack() {
		var err error
		convergedPreviousPackPath, err = assetBuilder.ensurePreviousPack()
		if err != nil {
			return AssetManager{}, errors.Wrap(err, "ensuring a previous pack executable exists")
		}

		convergedPreviousPackFixturesPath, err := assetBuilder.ensurePreviousPackFixtures()
		if err != nil {
			return AssetManager{}, errors.Wrap(err, "ensuring previous pack fixtures exist")
		}

		convergedPreviousPackFixturesPaths = []string{previousPackFixturesOverridesDir, convergedPreviousPackFixturesPath}
	}

	if inputConfig.combinations.requiresDefaultLifecycle() {
		var err error
		convergedDefaultLifecyclePath, convergedDefaultLifecycleDescriptor, err = assetBuilder.ensureDefaultLifecycle()
		if err != nil {
			return AssetManager{}, errors.Wrap(err, "ensuring a default lifecycle tarball exists")
		}
	}

	if inputConfig.combinations.requiresPreviousLifecycle() {
		var err error
		convergedPreviousLifecyclePath, convergedPreviousLifecycleDescriptor, err = assetBuilder.ensurePreviousLifecycle()
		if err != nil {
			return AssetManager{}, errors.Wrap(err, "ensuring a previous lifecycle tarball exists")
		}
	}

	return AssetManager{
		packPath:                    convergedCurrentPackPath,
		packFixturesPath:            currentPackFixturesDir,
		previousPackPath:            convergedPreviousPackPath,
		previousPackFixturesPaths:   convergedPreviousPackFixturesPaths,
		lifecyclePath:               convergedDefaultLifecyclePath,
		lifecycleDescriptor:         convergedDefaultLifecycleDescriptor,
		previousLifecyclePath:       convergedPreviousLifecyclePath,
		previousLifecycleDescriptor: convergedPreviousLifecycleDescriptor,
		testObject:                  t,
	}, nil
}

func (a AssetManager) PackPaths(kind ComboValue) (packPath string, packFixturesPaths []string) {
	a.testObject.Helper()

	switch kind {
	case Current:
		packPath = a.packPath
		packFixturesPaths = []string{a.packFixturesPath}
	case Previous:
		packPath = a.previousPackPath
		packFixturesPaths = a.previousPackFixturesPaths
	default:
		a.testObject.Fatalf("pack kind must be current or previous, was %s", kind)
	}

	return packPath, packFixturesPaths
}

func (a AssetManager) Lifecycle(kind ComboValue) (lifecyclePath string, lifecycleDescriptor builder.LifecycleDescriptor) {
	a.testObject.Helper()

	switch kind {
	case DefaultKind:
		lifecyclePath = a.lifecyclePath
		lifecycleDescriptor = a.lifecycleDescriptor
	case Previous:
		lifecyclePath = a.previousLifecyclePath
		lifecycleDescriptor = a.previousLifecycleDescriptor
	default:
		a.testObject.Fatalf("lifecycle kind must be default or previous, was %s", kind)
	}

	return lifecyclePath, lifecycleDescriptor
}

type assetManagerBuilder struct {
	testObject  *testing.T
	inputConfig InputConfigurationManager
}

func (b assetManagerBuilder) ensureCurrentPack() (string, error) {
	b.testObject.Helper()

	if b.inputConfig.packPath != "" {
		return b.inputConfig.packPath, nil
	}

	compileWithVersion := b.inputConfig.compilePackWithVersion
	if compileWithVersion == "" {
		compileWithVersion = defaultCompilePackVersion
	}

	packPath, err := b.buildPack(compileWithVersion)
	if err != nil {
		return "", errors.Wrap(err, "building current pack")
	}

	return packPath, nil
}

func (b assetManagerBuilder) ensurePreviousPack() (string, error) {
	b.testObject.Helper()

	if b.inputConfig.previousPackPath != "" {
		return b.inputConfig.previousPackPath, nil
	}

	b.testObject.Logf(
		"run combinations %+v require %s to be set",
		b.inputConfig.combinations,
		style.Symbol(envPreviousPackPath),
	)

	err := b.ensureGithubAssetFetcher()
	if err != nil {
		return "", errors.Wrap(err, "initializing github asset fetcher to download previous pack")
	}

	version, err := githubAssetFetcher.FetchReleaseVersion("buildpacks", "pack", 0)
	if err != nil {
		return "", errors.Wrap(err, "fetching release version of previous pack")
	}

	assetDir, err := githubAssetFetcher.FetchReleaseAsset(
		"buildpacks",
		"pack",
		version,
		variables.PackBinaryExp,
		true,
	)
	if err != nil {
		return "", errors.Wrap(err, "fetching release asset")
	}
	assetPath := filepath.Join(assetDir, variables.PackBinaryName)

	b.testObject.Logf("using %s for previous pack path", assetPath)

	return assetPath, nil
}

func (b assetManagerBuilder) ensurePreviousPackFixtures() (string, error) {
	b.testObject.Helper()

	if b.inputConfig.previousPackFixturesPath != "" {
		return b.inputConfig.previousPackFixturesPath, nil
	}

	b.testObject.Logf(
		"run combinations %+v require %s to be set",
		b.inputConfig.combinations,
		style.Symbol(envPreviousPackFixturesPath),
	)

	err := b.ensureGithubAssetFetcher()
	if err != nil {
		return "", errors.Wrap(err, "initializing github asset fetcher to download previous pack fixtures")
	}

	version, err := githubAssetFetcher.FetchReleaseVersion("buildpacks", "pack", 0)
	if err != nil {
		return "", errors.Wrap(err, "fetching release version")
	}

	sourceDir, err := githubAssetFetcher.FetchReleaseSource("buildpacks", "pack", version)
	if err != nil {
		return "", errors.Wrap(err, "fetching release source")
	}

	fis, err := ioutil.ReadDir(sourceDir)
	if err != nil {
		return "", errors.Wrapf(err, "reading directory %s", sourceDir)
	}
	// GitHub source tarballs have a top-level directory whose name includes the current commit sha.
	innerDir := fis[0].Name()
	fixturesDir := filepath.Join(sourceDir, innerDir, "acceptance", "testdata", "pack_fixtures")

	b.testObject.Logf("using %s for previous pack fixtures path", fixturesDir)

	return fixturesDir, nil
}

func (b assetManagerBuilder) ensureDefaultLifecycle() (string, builder.LifecycleDescriptor, error) {
	b.testObject.Helper()

	lifecyclePath := b.inputConfig.lifecyclePath

	if lifecyclePath == "" {
		b.testObject.Logf(
			"run combinations %+v require %s to be set",
			b.inputConfig.combinations,
			style.Symbol(envLifecyclePath),
		)

		var err error
		lifecyclePath, err = b.downloadLifecycle(0)
		if err != nil {
			return "", builder.LifecycleDescriptor{}, errors.Wrap(err, "fetching default lifecycle")
		}

		b.testObject.Logf("using %s for default lifecycle path", lifecyclePath)
	}

	lifecycle, err := builder.NewLifecycle(blob.NewBlob(lifecyclePath))
	if err != nil {
		return "",
			builder.LifecycleDescriptor{},
			errors.Wrapf(err, "reading default lifecycle from %s", lifecyclePath)
	}

	return lifecyclePath, lifecycle.Descriptor(), nil
}

func (b assetManagerBuilder) ensurePreviousLifecycle() (string, builder.LifecycleDescriptor, error) {
	b.testObject.Helper()

	previousLifecyclePath := b.inputConfig.lifecyclePath

	if previousLifecyclePath == "" {
		b.testObject.Logf(
			"run combinations %+v require %s to be set",
			b.inputConfig.combinations,
			style.Symbol(envPreviousLifecyclePath),
		)

		var err error
		previousLifecyclePath, err = b.downloadLifecycle(-1)
		if err != nil {
			return "", builder.LifecycleDescriptor{}, errors.Wrap(err, "fetching previous lifecycle")
		}

		b.testObject.Logf("using %s for previous lifecycle path", previousLifecyclePath)
	}

	lifecycle, err := builder.NewLifecycle(blob.NewBlob(previousLifecyclePath))
	if err != nil {
		return "",
			builder.LifecycleDescriptor{},
			errors.Wrapf(err, "reading previous lifecycle from %s", previousLifecyclePath)
	}

	return previousLifecyclePath, lifecycle.Descriptor(), nil
}

func (b assetManagerBuilder) downloadLifecycle(relativeVersion int) (string, error) {
	b.testObject.Helper()

	b.ensureGithubAssetFetcher()

	version, err := githubAssetFetcher.FetchReleaseVersion("buildpacks", "lifecycle", relativeVersion)
	if err != nil {
		return "", errors.Wrap(err, "fetching release version")
	}

	path, err := githubAssetFetcher.FetchReleaseAsset(
		"buildpacks",
		"lifecycle",
		version,
		lifecycleTgzExp,
		false,
	)
	if err != nil {
		return "", errors.Wrap(err, "fetching release asset")
	}

	return path, nil
}

func (b assetManagerBuilder) ensureGithubAssetFetcher() error {
	b.testObject.Helper()

	if githubAssetFetcher != nil {
		return nil
	}

	var err error
	githubAssetFetcher, err = NewGithubAssetFetcher(b.testObject, b.inputConfig.githubToken)
	if err != nil {
		return errors.Wrap(err, "fetching release version")
	}

	return nil
}

func (b assetManagerBuilder) buildPack(compileVersion string) (string, error) {
	b.testObject.Helper()

	packTmpDir, err := ioutil.TempDir("", "pack.acceptance.binary.")
	if err != nil {
		return "", errors.Wrap(err, "creating temp folder for pack")
	}

	packPath := filepath.Join(packTmpDir, variables.PackBinaryName)

	cwd, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(err, "getting current working directory for pack build")
	}

	cmd := exec.Command("go", "build",
		"-ldflags", fmt.Sprintf("-X 'github.com/buildpacks/pack/cmd.Version=%s'", compileVersion),
		"-mod=vendor",
		"-o", packPath,
		"./cmd/pack",
	)
	if filepath.Base(cwd) == "acceptance" {
		cmd.Dir = filepath.Dir(cwd)
	}

	b.testObject.Logf("building pack: [CWD=%s] %s", cmd.Dir, cmd.Args)
	if txt, err := cmd.CombinedOutput(); err != nil {
		return "", errors.Wrapf(err, "building pack cli: %s", string(txt))
	}

	return packPath, nil
}
