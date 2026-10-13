// Vendored from https://github.com/moby/moby/blob/7fd2be6664adc124debfebc63e2d02a7df096056/daemon/volume/mounts/validate.go
package volume

import (
	"fmt"

	"github.com/moby/moby/api/types/mount"
	"github.com/pkg/errors"
)

type errMountConfig struct {
	mount *mount.Mount
	err   error
}

func (e *errMountConfig) Error() string {
	return fmt.Sprintf("invalid mount config for type %q: %v", e.mount.Type, e.err.Error())
}

func errBindSourceDoesNotExist(path string) error {
	return errors.Errorf("bind source path does not exist: %s", path)
}

func errExtraField(name string) error {
	return errors.Errorf("field %s must not be specified", name)
}

func errMissingField(name string) error {
	return errors.Errorf("field %s must not be empty", name)
}
