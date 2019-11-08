package dist

import (
	"fmt"
	"sort"
	"strings"

	"github.com/buildpack/pack/internal/api"
	"github.com/buildpack/pack/internal/style"
)

type BuildpackDescriptor struct {
	API    *api.Version  `toml:"api"`
	Info   BuildpackInfo `toml:"buildpack"`
	Stacks []Stack       `toml:"stacks"`
	Order  Order         `toml:"order"`
}

func (b *BuildpackDescriptor) EscapedID() string {
	return strings.Replace(b.Info.ID, "/", "_", -1)
}

func (b *BuildpackDescriptor) EnsureStackSupport(stackID string, providedMixins []string, validateRunOnlyMixins bool) error {
	avail := map[string]interface{}{}
	for _, m := range providedMixins {
		avail[m] = nil
	}

	if len(b.Stacks) == 0 {
		return nil // Order buildpack, no validation required
	}

	bpMixins, err := b.findMixinsForStack(stackID)
	if err != nil {
		return err
	}

	var missing []string
	for _, m := range bpMixins {
		ignored := !validateRunOnlyMixins && strings.HasPrefix(m, "run:")
		if _, ok := avail[m]; !ignored && !ok {
			missing = append(missing, m)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("buildpack %s requires missing mixin(s): %s", style.Symbol(b.Info.FullName()), strings.Join(missing, ", "))
	}
	return nil
}

func (b *BuildpackDescriptor) findMixinsForStack(stackID string) ([]string, error) {
	for _, s := range b.Stacks {
		if s.ID == stackID {
			return s.Mixins, nil
		}
	}
	return nil, fmt.Errorf("buildpack %s does not support stack %s", style.Symbol(b.Info.FullName()), style.Symbol(stackID))
}
