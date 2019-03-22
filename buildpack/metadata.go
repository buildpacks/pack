package buildpack

import "strings"

type Buildpack struct {
	ID      string `toml:"id"`
	URI     string `toml:"uri"`
	Latest  bool   `toml:"latest"`
	Dir     string
	Version string
}

func (b *Buildpack) EscapedID() string {
	return strings.Replace(b.ID, "/", "_", -1)
}
