package writer

import (
	"fmt"

	"github.com/buildpacks/pack/internal/style"

	"github.com/buildpacks/pack/internal/inspectimage"

	"github.com/buildpacks/pack"
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
	if localErr != nil && remoteErr != nil {
		return fmt.Errorf("preparing BOM output for %s: local :%s remote: %s", style.Symbol(generalInfo.Name), localErr, remoteErr)
	}

	out, err := w.MarshalFunc(inspectimage.BOMDisplay{
		Remote:    inspectimage.NewBOMDisplay(remote),
		Local:     inspectimage.NewBOMDisplay(local),
		RemoteErr: errorString(remoteErr),
		LocalErr:  errorString(localErr),
	})

	if err != nil {
		return err
	}

	_, err = logger.Writer().Write(out)
	return err
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
