package acceptance

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/buildpacks/pack/logging"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
)

const (
	envPackPath                 = "PACK_PATH"
	envPreviousPackPath         = "PREVIOUS_PACK_PATH"
	envPreviousPackFixturesPath = "PREVIOUS_PACK_FIXTURES_PATH"
	envLifecyclePath            = "LIFECYCLE_PATH"
	envPreviousLifecyclePath    = "PREVIOUS_LIFECYCLE_PATH"
)

type InputPathsManager struct {
	packPath                 string
	previousPackPath         string
	previousPackFixturesPath string
	lifecyclePath            string
	previousLifecyclePath    string
	logger                   logging.Logger
}

func NewInputPathsManager(logger logging.Logger) (*InputPathsManager, error) {
	packPath := os.Getenv(envPackPath)
	previousPackPath := os.Getenv(envPreviousPackPath)
	previousPackFixturesPath := os.Getenv(envPreviousPackFixturesPath)
	lifecyclePath := os.Getenv(envLifecyclePath)
	previousLifecyclePath := os.Getenv(envPreviousLifecyclePath)

	inputPaths := []string{
		packPath,
		previousPackPath,
		previousPackFixturesPath,
		lifecyclePath,
		previousLifecyclePath,
	}

	for idx, inputPath := range inputPaths {
		if inputPath == "" {
			continue
		}
		updatedInputPath, err := filepath.Abs(inputPath)
		if err != nil {
			return nil, errors.Wrapf(err, "getting absolute path for %s", inputPath)
		}
		inputPaths[idx] = updatedInputPath
	}

	return &InputPathsManager{
		packPath:                 inputPaths[0],
		previousPackPath:         inputPaths[1],
		previousPackFixturesPath: inputPaths[2],
		lifecyclePath:            inputPaths[3],
		previousLifecyclePath:    inputPaths[4],
		logger:                   logger,
	}, nil
}

func (m *InputPathsManager) FillInRequiredPaths(c runCombo) error {
	githubAssetFetcher, err := NewGithubAssetFetcher(m.logger)
	if err != nil {
		return errors.Wrap(err, "initializing GitHub asset fetcher")
	}

	if (c.Pack == "previous" || c.PackCreateBuilder == "previous") && m.previousPackPath == "" {
		m.logger.Infof("run combination %+v requires %s to be set\n", c, style.Symbol(envPreviousPackPath))

		version, err := githubAssetFetcher.FetchReleaseVersion("buildpacks", "pack", 0)
		if err != nil {
			return errors.Wrap(err, "fetching release version")
		}

		assetDir, err := githubAssetFetcher.FetchReleaseAsset(
			"buildpacks",
			"pack",
			version,
			packBinaryExp(),
			true,
		)
		if err != nil {
			return errors.Wrap(err, "fetching release asset")
		}
		assetPath := filepath.Join(assetDir, packBinaryName())

		m.logger.Infof("using %s for previous pack path\n", assetPath)
		m.previousPackPath = assetPath
	}
	if (c.Pack == "previous" || c.PackCreateBuilder == "previous") && m.previousPackFixturesPath == "" {
		m.logger.Infof("run combination %+v requires %s to be set\n", c, style.Symbol(envPreviousPackFixturesPath))

		version, err := githubAssetFetcher.FetchReleaseVersion("buildpacks", "pack", 0)
		if err != nil {
			return errors.Wrap(err, "fetching release version")
		}

		sourceDir, err := githubAssetFetcher.FetchReleaseSource("buildpacks", "pack", version)
		if err != nil {
			return errors.Wrap(err, "fetching release source")
		}

		fis, err := ioutil.ReadDir(sourceDir)
		if err != nil {
			return errors.Wrapf(err, "reading directory %s", sourceDir)
		}
		// GitHub source tarballs have a top-level directory whose name includes the current commit sha.
		innerDir := fis[0].Name()
		fixturesDir := filepath.Join(sourceDir, innerDir, "acceptance", "testdata", "pack_fixtures")

		m.logger.Infof("using %s for previous pack fixtures path\n", fixturesDir)
		m.previousPackFixturesPath = fixturesDir
	}
	if c.Lifecycle == "current" && m.lifecyclePath == "" {
		m.logger.Infof("run combination %+v requires %s to be set\n", c, style.Symbol(envLifecyclePath))

		version, err := githubAssetFetcher.FetchReleaseVersion("buildpacks", "lifecycle", 0)
		if err != nil {
			return errors.Wrap(err, "fetching release version")
		}

		assetPath, err := githubAssetFetcher.FetchReleaseAsset(
			"buildpacks",
			"lifecycle",
			version,
			lifecycleTgzExp(),
			false,
		)
		if err != nil {
			return errors.Wrap(err, "fetching release asset")
		}

		m.logger.Infof("using %s for lifecycle path\n", assetPath)
		m.lifecyclePath = assetPath
	}
	if c.Lifecycle == "previous" && m.previousLifecyclePath == "" {
		m.logger.Infof("run combination %+v requires %s to be set\n", c, style.Symbol(envPreviousLifecyclePath))

		version, err := githubAssetFetcher.FetchReleaseVersion("buildpacks", "lifecycle", -1)
		if err != nil {
			return errors.Wrap(err, "fetching release version")
		}

		assetPath, err := githubAssetFetcher.FetchReleaseAsset(
			"buildpacks",
			"lifecycle",
			version,
			lifecycleTgzExp(),
			false,
		)
		if err != nil {
			return errors.Wrap(err, "fetching release asset")
		}

		m.logger.Infof("using %s for previous lifecycle path\n", assetPath)
		m.previousLifecyclePath = assetPath
	}
	return nil
}

func lifecycleTgzExp() *regexp.Regexp {
	switch runtime.GOOS {
	case "darwin", "linux":
		return regexp.MustCompile(`lifecycle-v\d+.\d+.\d+\+linux.x86-64.tgz`)
	default:
		return nil
	}
}

func packBinaryExp() *regexp.Regexp {
	// Omit extension so that this expression could be used when we write to the asset cache.
	switch runtime.GOOS {
	case "darwin":
		return regexp.MustCompile(`pack-v\d+.\d+.\d+-macos`)
	case "linux":
		return regexp.MustCompile(`pack-v\d+.\d+.\d+-linux`)
	case "windows":
		return regexp.MustCompile(`pack-v\d+.\d+.\d+-windows`)
	default:
		return nil
	}
}

func packBinaryName() string {
	if runtime.GOOS == "windows" {
		return "pack.exe"
	}
	return "pack"
}
