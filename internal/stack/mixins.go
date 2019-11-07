package stack

import (
	"fmt"
	"sort"
	"strings"

	"github.com/buildpack/pack/internal/style"
)

const MixinsLabel = "io.buildpacks.stack.mixins"

func ValidateMixins(buildImageName string, buildImageMixins []string, runImageName string, runImageMixins []string) error {
	bMixins, err := mixinSet(buildImageMixins, buildImageName, false)
	if err != nil {
		return err
	}

	rMixins, err := mixinSet(runImageMixins, runImageName, true)
	if err != nil {
		return err
	}

	if err := validateMissing(rMixins, bMixins, runImageName); err != nil {
		return err
	}
	return nil
}

func mixinSet(mixins []string, imageName string, run bool) (map[string]interface{}, error) {
	set := map[string]interface{}{}
	invalidPrefix := "run"
	excludePrefix := "build"
	if run {
		invalidPrefix = "build"
		excludePrefix = "run"
	}

	var invalid []string
	for _, m := range mixins {
		if strings.HasPrefix(m, invalidPrefix+":") {
			invalid = append(invalid, m)
			continue
		}
		if strings.HasPrefix(m, excludePrefix+":") {
			continue
		}
		set[m] = nil
	}

	if len(invalid) > 0 {
		sort.Strings(invalid)
		return nil, fmt.Errorf("%s contains %s-only mixin(s): %s", style.Symbol(imageName), invalidPrefix, strings.Join(invalid, ", "))
	}
	return set, nil
}

func validateMissing(actual, required map[string]interface{}, actualImageName string) error {
	var missing []string
	for m := range required {
		if _, ok := actual[m]; !ok {
			missing = append(missing, m)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%s missing required mixin(s): %s", style.Symbol(actualImageName), strings.Join(missing, ", "))
	}
	return nil
}
