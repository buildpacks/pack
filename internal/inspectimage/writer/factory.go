package writer

import (
	"fmt"
	"github.com/buildpacks/pack/internal/config"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/logging"

	"github.com/buildpacks/pack/internal/style"
)

type Factory struct{}

type InspectImageWriter interface {
	Print(
		logger logging.Logger,
		sharedInfo *SharedImageInfo,
		local, remote *pack.ImageInfo,
		localErr, remoteErr error,
	) error
}

func NewFactory() *Factory {
	return &Factory{}
}

type SharedImageInfo struct {
	Name            string
	RunImageMirrors []config.RunImage
}

func (f *Factory) Writer(kind string, BOM bool) (InspectImageWriter, error) {
	switch kind {
	case "human-readable":
		return NewHumanReadable(), nil
		//case "json":
		//	return NewJSON(), nil
		//case "yaml":
		//	return NewYAML(), nil
		//case "toml":
		//	return NewTOML(), nil
	}

	return nil, fmt.Errorf("output format %s is not supported", style.Symbol(kind))
}
