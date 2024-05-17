package builder

func (c cmd) String() string {
	return string(c.path)
}

func (c *cmd) Workspace(wd string) {
	c.wd = wd
}

func (c *cmd) Platform(os string) {
	c.os = os
}