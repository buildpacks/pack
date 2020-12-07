package writer

import (
	"fmt"

	"github.com/buildpacks/pack/internal/inspectimage"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type StructuredBOMFormat struct {
	MarshalFunc func(interface{}) ([]byte, error)
}

func (w *StructuredBOMFormat) Print(
	logger logging.Logger,
	generalInfo inspectimage.GeneralInfo,
	local, remote *pack.ImageInfo,
	localErr, remoteErr error,
) error {
	if local == nil && remote == nil {
		return fmt.Errorf("unable to find image '%s' locally or remotely", generalInfo.Name)
	}
	if localErr != nil {
		return fmt.Errorf("preparing BOM output for %s: %w", style.Symbol(generalInfo.Name), localErr)
	}

	if remoteErr != nil {
		return fmt.Errorf("preparing BOM output for %s: %w", style.Symbol(generalInfo.Name), remoteErr)
	}

	out, err := w.MarshalFunc(inspectimage.BOMDisplay{
		Remote: inspectimage.NewBOMDisplay(remote),
		Local:  inspectimage.NewBOMDisplay(local),
	})

	if err != nil {
		panic(err)
	}

	_, err = logger.Writer().Write(out)
	return err
}
