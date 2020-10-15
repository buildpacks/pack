package writer

import (
	"fmt"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/style"
)

type Factory struct{}

func NewFactory() *Factory {
	return &Factory{}
}

func (f *Factory) Writer(kind string) (commands.BuilderWriter, error) {
	switch kind {
	case "human-readable":
		return NewHumanReadable(), nil
	case "json":
		return NewJSON(), nil
	case "yaml":
		return NewYAML(), nil
	case "toml":
		return NewTOML(), nil
	}

	return nil, fmt.Errorf("output format %s is not supported", style.Symbol(kind))
}
