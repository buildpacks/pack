package builder

import "strings"

func (c cmd) String() string {
	return string(c.path)
}

func (c *cmd) Workspace(wd string) {
	c.wd = wd
}

func (c *cmd) Platform(os string) {
	c.os = os
}

func ParseVolume(volume string) (hostPath, ctrPath, perm string) {
	hostPath, other, _ := strings.Cut(volume, ":")
	ctrPath, perm, _ = strings.Cut(other, ":")
	return hostPath, ctrPath, perm
}
