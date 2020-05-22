// +build acceptance

package managers

import (
	"encoding/json"
	"fmt"

	"github.com/buildpacks/pack/internal/style"
	"github.com/pkg/errors"
)

type ComboValue int

const (
	Current ComboValue = iota
	Previous
	DefaultKind
)

func (v ComboValue) String() string {
	switch v {
	case Current:
		return "current"
	case Previous:
		return "previous"
	case DefaultKind:
		return "default"
	default:
		return ""
	}
}

var defaultRunCombo = []*RunCombo{
	{Pack: Current, PackCreateBuilder: Current, Lifecycle: DefaultKind},
}

type RunCombo struct {
	Pack              ComboValue `json:"pack"`
	PackCreateBuilder ComboValue `json:"pack_create_builder"`
	Lifecycle         ComboValue `json:"lifecycle"`
}

func (c *RunCombo) UnmarshalJSON(b []byte) error {
	var o map[string]string

	if err := json.Unmarshal(b, &o); err != nil {
		return errors.Errorf(`failed to unmarshal run combo element: %s`, b)
	}

	for k, v := range o {
		switch k {
		case "pack":
			val, err := validatedPackKind(v)
			if err != nil {
				return errors.Errorf("failed to parse kind of %s: %s", style.Symbol("pack"), err)
			}

			c.Pack = val
		case "pack_create_builder":
			val, err := validatedPackKind(v)
			if err != nil {
				return errors.Errorf(
					"failed to parse kind of %s: %s", style.Symbol("pack_create_builder"),
					err,
				)
			}

			c.PackCreateBuilder = val
		case "lifecycle":
			val, err := validateLifecycleKind(v)
			if err != nil {
				return errors.Errorf("failed to parse kind of %s: %s", style.Symbol("lifecycle"), err)
			}

			c.Lifecycle = val
		default:
			return errors.Errorf("unknown key %s in run combo", style.Symbol(k))
		}
	}

	return nil
}

func validatedPackKind(k string) (ComboValue, error) {
	switch k {
	case "current":
		return Current, nil
	case "previous":
		return Previous, nil
	default:
		return Current, errors.Errorf("must be either current or previous, was %s", style.Symbol(k))
	}
}

func validateLifecycleKind(k string) (ComboValue, error) {
	switch k {
	case "default":
		return DefaultKind, nil
	case "previous":
		return Previous, nil
	default:
		return DefaultKind, errors.Errorf("must be either default or previous, was %s", style.Symbol(k))
	}
}

func (c *RunCombo) String() string {
	return fmt.Sprintf("p_%s cb_%s lc_%s", c.Pack, c.PackCreateBuilder, c.Lifecycle)
}

func (c *RunCombo) Describe(assets AssetManager) string {
	packPath, packFixturesPaths := assets.PackPaths(c.Pack)
	cbPackPath, cbPackFixturesPaths := assets.PackPaths(c.PackCreateBuilder)
	lifecyclePath, lifecycleDescriptor := assets.Lifecycle(c.Lifecycle)

	return fmt.Sprintf(`
pack:
|__ path: %s
|__ fixtures: %s

create builder:
|__ pack path: %s
|__ pack fixtures: %s

lifecycle:
|__ path: %s
|__ version: %s
|__ buildpack api: %s
|__ platform api: %s
`,
		packPath,
		packFixturesPaths,
		cbPackPath,
		cbPackFixturesPaths,
		lifecyclePath,
		lifecycleDescriptor.Info.Version,
		lifecycleDescriptor.API.BuildpackVersion,
		lifecycleDescriptor.API.PlatformVersion,
	)
}

type comboSet []*RunCombo

func (combos comboSet) requiresCurrentPack() bool {
	return combos.requiresPackKind(Current)
}

func (combos comboSet) requiresPreviousPack() bool {
	return combos.requiresPackKind(Previous)
}

func (combos comboSet) requiresPackKind(k ComboValue) bool {
	for _, c := range combos {
		if c.Pack == k || c.PackCreateBuilder == k {
			return true
		}
	}

	return false
}

func (combos comboSet) IncludesCurrentSubjectPack() bool {
	for _, c := range combos {
		if c.Pack == Current {
			return true
		}
	}

	return false
}

func (combos comboSet) requiresDefaultLifecycle() bool {
	return combos.requiresLifecycleKind(DefaultKind)
}

func (combos comboSet) requiresPreviousLifecycle() bool {
	return combos.requiresLifecycleKind(Previous)
}

func (combos comboSet) requiresLifecycleKind(k ComboValue) bool {
	for _, c := range combos {
		if c.Lifecycle == k {
			return true
		}
	}

	return false
}
