package dist

import (
	"fmt"
	"strings"

	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/internal/style"
)

type ExtensionDescriptor struct {
	WithAPI          *api.Version `toml:"api"`
	WithInfo         ModuleInfo   `toml:"extension"`
	WithTargets      []Target     `toml:"targets,omitempty"`
	WithWindowsBuild bool
	WithLinuxBuild   bool
}

func (e *ExtensionDescriptor) EnsureStackSupport(_ string, _ []string, _ bool) error {
	return nil
}

func (e *ExtensionDescriptor) EnsureTargetSupport(os, arch, distroName, distroVersion string) error {
	if len(e.Targets()) == 0 {
		if !e.WithLinuxBuild && !e.WithWindowsBuild { // nolint
			return nil // Order extension, no validation required
		} else if e.WithLinuxBuild && os == DefaultTargetOSLinux && arch == DefaultTargetArch {
			return nil
		} else if e.WithWindowsBuild && os == DefaultTargetOSWindows && arch == DefaultTargetArch {
			return nil
		}
	}
	for _, target := range e.Targets() {
		if target.OS == os {
			if target.Arch == "" || arch == "" || target.Arch == arch {
				if len(target.Distributions) == 0 || distroName == "" || distroVersion == "" {
					return nil
				}
				for _, distro := range target.Distributions {
					if distro.Name == distroName {
						if len(distro.Versions) == 0 {
							return nil
						}
						for _, version := range distro.Versions {
							if version == distroVersion {
								return nil
							}
						}
					}
				}
			}
		}
	}
	type osDistribution struct {
		Name    string `json:"name,omitempty"`
		Version string `json:"version,omitempty"`
	}
	type target struct {
		OS           string         `json:"os"`
		Arch         string         `json:"arch"`
		Distribution osDistribution `json:"distribution"`
	}
	return fmt.Errorf(
		"unable to satisfy target os/arch constraints; build image: %s, extension %s: %s",
		toJSONMaybe(target{
			OS:           os,
			Arch:         arch,
			Distribution: osDistribution{Name: distroName, Version: distroVersion},
		}),
		style.Symbol(e.Info().FullName()),
		toJSONMaybe(e.Targets()),
	)
}

func (e *ExtensionDescriptor) EscapedID() string {
	return strings.ReplaceAll(e.Info().ID, "/", "_")
}

func (e *ExtensionDescriptor) Kind() string {
	return "extension"
}

func (e *ExtensionDescriptor) API() *api.Version {
	return e.WithAPI
}

func (e *ExtensionDescriptor) Info() ModuleInfo {
	return e.WithInfo
}

func (e *ExtensionDescriptor) Order() Order {
	return nil
}

func (e *ExtensionDescriptor) Stacks() []Stack {
	return nil
}

func (e *ExtensionDescriptor) Targets() []Target {
	return e.WithTargets
}
