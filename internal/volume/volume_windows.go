// Vendored from https://github.com/moby/moby/blob/7fd2be6664adc124debfebc63e2d02a7df096056/daemon/volume/mounts/volume_windows.go
package volume

func (p *linuxParser) HasResource(m *MountPoint, absolutePath string) bool {
	return false
}
