// Vendored from https://github.com/moby/moby/blob/7fd2be6664adc124debfebc63e2d02a7df096056/daemon/volume/mounts/volume_unix.go
//go:build linux || freebsd || darwin

package volume

import (
	"fmt"
	"path/filepath"
	"strings"
)

func (p *linuxParser) HasResource(m *MountPoint, absolutePath string) bool {
	relPath, err := filepath.Rel(m.Destination, absolutePath)
	return err == nil && relPath != ".." && !strings.HasPrefix(relPath, fmt.Sprintf("..%c", filepath.Separator))
}
