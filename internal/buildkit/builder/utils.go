package builder

import (
	// "github.com/docker/buildx/driver"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	// "github.com/pkg/errors"
)

// func notSupported(f driver.Feature, d *driver.DriverHandle, docs string) error {
// 	return errors.Errorf(`%s is not supported for the %s driver.
// Switch to a different driver, or turn on the containerd image store, and try again.
// Learn more at %s`, f, d.Factory().Name(), docs)
// }

// GetImageID returns the image ID - the digest of the image config
func GetImageID(resp map[string]string) string {
	dgst := resp[exptypes.ExporterImageDigestKey]
	if v, ok := resp[exptypes.ExporterImageConfigDigestKey]; ok {
		dgst = v
	}
	return dgst
}