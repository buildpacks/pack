package mountpaths

import "strings"

type MountPaths struct {
	volume    string
	separator string
	workspace string
}

func MountPathsForOS(os, workspace string) MountPaths {
	if workspace == "" {
		workspace = "workspace"
	}
	if os == "windows" {
		return MountPaths{
			volume:    `c:`,
			separator: `\`,
			workspace: workspace,
		}
	}
	return MountPaths{
		volume:    "",
		separator: "/",
		workspace: workspace,
	}
}

func (m MountPaths) join(parts ...string) string {
	return strings.Join(parts, m.separator)
}

func (m MountPaths) LayersDir() string {
	return m.join(m.volume, "layers")
}

func (m MountPaths) StackPath() string {
	return m.join(m.LayersDir(), "stack.toml")
}

func (m MountPaths) RunPath() string {
	return m.join(m.LayersDir(), "run.toml")
}

func (m MountPaths) ProjectPath() string {
	return m.join(m.LayersDir(), "project-metadata.toml")
}

func (m MountPaths) ReportPath() string {
	return m.join(m.LayersDir(), "report.toml")
}

func (m MountPaths) AppDirName() string {
	return m.workspace
}

func (m MountPaths) AppDir() string {
	return m.join(m.volume, m.AppDirName())
}

func (m MountPaths) CacheDir() string {
	return m.join(m.volume, "cache")
}

func (m MountPaths) KanikoCacheDir() string {
	return m.join(m.volume, "kaniko")
}

func (m MountPaths) LaunchCacheDir() string {
	return m.join(m.volume, "launch-cache")
}

func (m MountPaths) SbomDir() string {
	return m.join(m.volume, "layers", "sbom")
}

func (m MountPaths) Volume() string {
	return m.volume
}
