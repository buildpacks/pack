package dist

import (
	"strings"

	"github.com/buildpacks/lifecycle/api"
)

type ExtensionDescriptor struct {
	API  *api.Version  `toml:"api"`
	Info BuildpackInfo `toml:"extension"`
}

func (e *ExtensionDescriptor) EnsureStackSupport(_ string, _ []string, _ bool) error {
	return nil
}

func (e *ExtensionDescriptor) EscapedID() string {
	return strings.ReplaceAll(e.Info.ID, "/", "_")
}

func (e *ExtensionDescriptor) Kind() string {
	return "extension"
}

func (e *ExtensionDescriptor) ModuleAPI() *api.Version {
	return e.API
}

func (e *ExtensionDescriptor) ModuleInfo() BuildpackInfo {
	return e.Info
}

func (e *ExtensionDescriptor) ModuleOrder() Order {
	return nil
}

func (e *ExtensionDescriptor) ModuleStacks() []Stack {
	return nil
}
