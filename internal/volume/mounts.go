// Vendored from https://github.com/moby/moby/blob/7fd2be6664adc124debfebc63e2d02a7df096056/daemon/volume/mounts/mounts.go
package volume

import (
	mounttypes "github.com/moby/moby/api/types/mount"
	"github.com/pkg/errors"
)

type MountPoint struct {
	Source      string
	Destination string
	RW          bool
	Name        string
	Driver      string
	Type        mounttypes.Type        `json:",omitempty"`
	Mode        string                 `json:"Relabel,omitempty"`
	Propagation mounttypes.Propagation `json:",omitempty"`
	Spec        mounttypes.Mount
	CopyData    bool
}

func (m *MountPoint) Path() string {
	return m.Source
}

func errInvalidMode(mode string) error {
	return errors.Errorf("invalid mode: %v", mode)
}

func errInvalidSpec(spec string) error {
	return errors.Errorf("invalid volume specification: '%s'", spec)
}
