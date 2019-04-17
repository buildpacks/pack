package buildpack

import "strings"

type BuildpackTOML struct {
	Buildpack Buildpack
	Stacks []Stack
}

type Buildpack struct {
	ID      string
	Latest  bool
	Dir     string
	Version string
	Stacks []Stack
}

type Stack struct {
	ID string
}

func (b *Buildpack) EscapedID() string {
	return strings.Replace(b.ID, "/", "_", -1)
}

func (b *Buildpack) SupportsStack(stackID string) bool {
	for _, stack := range b.Stacks {
		if stack.ID == stackID {
			return true
		}
	}
	return false
}
