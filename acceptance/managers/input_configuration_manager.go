// +build acceptance

package managers

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	envPackPath                 = "PACK_PATH"
	envPreviousPackPath         = "PREVIOUS_PACK_PATH"
	envPreviousPackFixturesPath = "PREVIOUS_PACK_FIXTURES_PATH"
	envLifecyclePath            = "LIFECYCLE_PATH"
	envPreviousLifecyclePath    = "PREVIOUS_LIFECYCLE_PATH"
	envGitHubToken              = "GITHUB_TOKEN"
	envAcceptanceSuiteConfig    = "ACCEPTANCE_SUITE_CONFIG"
	envCompilePackWithVersion   = "COMPILE_PACK_WITH_VERSION"
)

type InputConfigurationManager struct {
	packPath                 string
	previousPackPath         string
	previousPackFixturesPath string
	lifecyclePath            string
	previousLifecyclePath    string
	compilePackWithVersion   string
	githubToken              string
	combinations             ComboSet
}

func NewInputConfigurationManager() (InputConfigurationManager, error) {
	packPath := os.Getenv(envPackPath)
	previousPackPath := os.Getenv(envPreviousPackPath)
	previousPackFixturesPath := os.Getenv(envPreviousPackFixturesPath)
	lifecyclePath := os.Getenv(envLifecyclePath)
	previousLifecyclePath := os.Getenv(envPreviousLifecyclePath)
	compilePackWithVersion := os.Getenv(envCompilePackWithVersion)
	githubToken := os.Getenv(envGitHubToken)

	err := resolveAbsolutePaths(&packPath, &previousPackPath, &previousPackFixturesPath, &lifecyclePath, &previousLifecyclePath)
	if err != nil {
		return InputConfigurationManager{}, err
	}

	var combos ComboSet

	comboConfig := os.Getenv(envAcceptanceSuiteConfig)
	if comboConfig != "" {
		if err := json.Unmarshal([]byte(comboConfig), &combos); err != nil {
			return InputConfigurationManager{}, errors.Errorf("failed to parse combination config: %s", err)
		}
	} else {
		combos = defaultRunCombo
	}

	return InputConfigurationManager{
		packPath:                 packPath,
		previousPackPath:         previousPackPath,
		previousPackFixturesPath: previousPackFixturesPath,
		lifecyclePath:            lifecyclePath,
		previousLifecyclePath:    previousLifecyclePath,
		compilePackWithVersion:   compilePackWithVersion,
		githubToken:              githubToken,
		combinations:             combos,
	}, nil
}

func (i InputConfigurationManager) Combinations() ComboSet {
	return i.combinations
}

func resolveAbsolutePaths(paths ...*string) error {
	for _, path := range paths {
		if *path == "" {
			continue
		}

		absPath, err := filepath.Abs(*path)
		if err != nil {
			return errors.Wrapf(err, "getting absolute path for %s", *path)
		}

		*path = absPath
	}

	return nil
}
