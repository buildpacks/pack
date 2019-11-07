package stack

import (
	"fmt"
	"sort"
	"strings"

	"github.com/buildpack/pack/internal/style"
)

type mixiner interface {
	Mixins() []string
	Name() string
}

func validateStageMixins(image mixiner, run bool) error {
	invalidPrefix := "run"
	if run {
		invalidPrefix = "build"
	}

	var invalid []string
	for _, m := range image.Mixins() {
		if strings.HasPrefix(m, invalidPrefix+":") {
			invalid = append(invalid, m)
		}
	}

	if len(invalid) > 0 {
		sort.Strings(invalid)
		return fmt.Errorf("%s contains %s-only mixin(s): %s", style.Symbol(image.Name()), invalidPrefix, strings.Join(invalid, ", "))
	}
	return nil
}
