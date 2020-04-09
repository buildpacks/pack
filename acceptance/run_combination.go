package acceptance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/style"
)

const envAcceptanceSuiteConfig = "ACCEPTANCE_SUITE_CONFIG"

type runCombo struct {
	Pack              string `json:"pack"`
	PackCreateBuilder string `json:"pack_create_builder"`
	Lifecycle         string `json:"lifecycle"`
}

//nolint:unused // TODO: nolint directives in this file shouldn't be necessary, as
// all of the code is used. However lint errors are returned without these directives.
// Possibly related to https://github.com/golangci/golangci-lint/issues/791.
type resolvedRunCombo struct {
	packCreateBuilderFixturesDir string
	packFixturesDir              string
	packPath                     string
	packCreateBuilderPath        string
	lifecyclePath                string
	lifecycleDescriptor          builder.LifecycleDescriptor
}

func (c *runCombo) String() string {
	return fmt.Sprintf("p_%s cb_%s lc_%s", c.Pack, c.PackCreateBuilder, c.Lifecycle)
}

//nolint
func getRunCombinations() ([]runCombo, error) {
	combos := []runCombo{
		{Pack: "current", PackCreateBuilder: "current", Lifecycle: "default"}, // TODO: the current behavior for
		// `make acceptance` is to test current-current-current which is actually current-current-default when no
		// lifecycle path is provided. Confirm if we should keep this behavior going forward.
	}

	suiteConfig := os.Getenv(envAcceptanceSuiteConfig)
	if suiteConfig == "" {
		return combos, nil
	}

	return parseSuiteConfig(suiteConfig)
}

//nolint:unused
func parseSuiteConfig(config string) ([]runCombo, error) {
	var cfgs []runCombo
	if err := json.Unmarshal([]byte(config), &cfgs); err != nil {
		return nil, errors.Wrap(err, "parse config")
	}

	validate := func(jsonKey, value string) error {
		switch value {
		case "current", "previous":
			return nil
		default:
			return fmt.Errorf("invalid config: %s not valid value for %s", style.Symbol(value), style.Symbol(jsonKey))
		}
	}

	for _, c := range cfgs {
		if err := validate("pack", c.Pack); err != nil {
			return nil, err
		}

		if err := validate("pack_create_builder", c.PackCreateBuilder); err != nil {
			return nil, err
		}

		if err := validate("lifecycle", c.Lifecycle); err != nil {
			return nil, err
		}
	}

	return cfgs, nil
}

//nolint
func resolveRunCombinations(
	combos []runCombo,
	packPath string,
	previousPackPath string,
	previousPackFixturesPath string,
	lifecyclePath string,
	lifecycleDescriptor builder.LifecycleDescriptor,
	previousLifecyclePath string,
	previousLifecycleDescriptor builder.LifecycleDescriptor,
) (map[string]resolvedRunCombo, error) {
	resolved := map[string]resolvedRunCombo{}
	for _, c := range combos {
		rc := resolvedRunCombo{
			packFixturesDir:              filepath.Join("testdata", "pack_fixtures"),
			packCreateBuilderFixturesDir: filepath.Join("testdata", "pack_fixtures"),
			packPath:                     packPath,
			packCreateBuilderPath:        packPath,
			lifecyclePath:                lifecyclePath,
			lifecycleDescriptor:          lifecycleDescriptor,
		}

		if c.Pack == "previous" {
			rc.packPath = previousPackPath
			rc.packFixturesDir = previousPackFixturesPath
		}

		if c.PackCreateBuilder == "previous" {
			rc.packCreateBuilderPath = previousPackPath
			rc.packCreateBuilderFixturesDir = previousPackFixturesPath
		}

		if c.Lifecycle == "previous" {
			rc.lifecyclePath = previousLifecyclePath
			rc.lifecycleDescriptor = previousLifecycleDescriptor
		}

		resolved[c.String()] = rc
	}

	return resolved, nil
}
