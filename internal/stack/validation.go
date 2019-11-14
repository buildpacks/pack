package stack

import (
	"fmt"
	"sort"
	"strings"

	"github.com/buildpack/pack/internal/style"
)

func validateStageMixins(stackImage Image, invalidPrefix string) error {
	var invalid []string
	for _, m := range stackImage.Mixins() {
		if strings.HasPrefix(m, invalidPrefix+":") {
			invalid = append(invalid, m)
		}
	}

	if len(invalid) > 0 {
		sort.Strings(invalid)
		return fmt.Errorf("%s contains %s-only mixin(s): %s", style.Symbol(stackImage.Name()), invalidPrefix, strings.Join(invalid, ", "))
	}
	return nil
}
