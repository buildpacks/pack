package dist

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
)

const AssumedBuildpackAPIVersion = "0.1"
const BuildpacksDir = "/cnb/buildpacks"
const ExtensionsDir = "/cnb/extensions"

type ModuleInfo struct {
	ID          string    `toml:"id,omitempty" json:"id,omitempty" yaml:"id,omitempty"`
	Name        string    `toml:"name,omitempty" json:"name,omitempty" yaml:"name,omitempty"`
	Version     string    `toml:"version,omitempty" json:"version,omitempty" yaml:"version,omitempty"`
	Description string    `toml:"description,omitempty" json:"description,omitempty" yaml:"description,omitempty"`
	Homepage    string    `toml:"homepage,omitempty" json:"homepage,omitempty" yaml:"homepage,omitempty"`
	Keywords    []string  `toml:"keywords,omitempty" json:"keywords,omitempty" yaml:"keywords,omitempty"`
	Licenses    []License `toml:"licenses,omitempty" json:"licenses,omitempty" yaml:"licenses,omitempty"`
}

func (b ModuleInfo) FullName() string {
	if b.Version != "" {
		return b.ID + "@" + b.Version
	}
	return b.ID
}

func (b ModuleInfo) FullNameWithVersion() (string, error) {
	if b.Version == "" {
		return b.ID, errors.Errorf("buildpack %s does not have a version defined", style.Symbol(b.ID))
	}
	return b.ID + "@" + b.Version, nil
}

// Satisfy stringer
func (b ModuleInfo) String() string { return b.FullName() }

// Match compares two buildpacks by ID and Version
func (b ModuleInfo) Match(o ModuleInfo) bool {
	return b.ID == o.ID && b.Version == o.Version
}

type License struct {
	Type string `toml:"type"`
	URI  string `toml:"uri"`
}

type Stack struct {
	ID     string   `json:"id" toml:"id"`
	Mixins []string `json:"mixins,omitempty" toml:"mixins,omitempty"`
}

type Target struct {
	OS            string         `json:"os" toml:"os"`
	Arch          string         `json:"arch" toml:"arch"`
	ArchVariant   string         `json:"variant,omitempty" toml:"variant,omitempty"`
	Distributions []Distribution `json:"distributions,omitempty" toml:"distributions,omitempty"`
	Specs         TargetSpecs    `json:"specs,omitempty" toml:"specs,omitempty"`
}

type Distribution struct {
	Name     string   `json:"name,omitempty" toml:"name,omitempty"`
	Versions []string `json:"versions,omitempty" toml:"versions,omitempty"`
}

type TargetSpecs struct {
	Features       []string          `json:"features,omitempty" toml:"features,omitempty"`
	OSFeatures     []string          `json:"os.features,omitempty" toml:"os.features,omitempty"`
	URLs           []string          `json:"urls,omitempty" toml:"urls,omitempty"`
	Annotations    map[string]string `json:"annotations,omitempty" toml:"annotations,omitempty"`
	Flatten        bool              `json:"flatten,omitempty" toml:"flatten,omitempty"`
	FlattenExclude []string          `json:"flatten.exclude,omitempty" toml:"flatten.exclude,omitempty"`
	Labels         map[string]string `json:"labels,omitempty" toml:"labels,omitempty"`
	OSVersion      string            `json:"os.version,omitempty" toml:"os.version,omitempty"`
	Path           string            `json:"path,omitempty" toml:"path,omitempty"`
	BuildConfigEnv map[string]string `json:"build.envs,omitempty" toml:"build.envs,omitempty"`
}

func (t *Target) Platform() *v1.Platform {
	return &v1.Platform{
		OS:           t.OS,
		Architecture: t.Arch,
		Variant:      t.ArchVariant,
		OSVersion:    t.Specs.OSVersion,
		Features:     t.Specs.Features,
		OSFeatures:   t.Specs.OSFeatures,
	}
}

func (t *Target) Annotations() (map[string]string, error) {
	if len(t.Distributions) == 0 {
		return nil, errors.New("unable to get annotations: distroless target provided.")
	}

	distro := t.Distributions[0]
	return map[string]string{
		"io.buildpacks.base.distro.name":    distro.Name,
		"io.buildpacks.base.distro.version": distro.Versions[0],
	}, nil
}

func (t *Target) URLs() []string {
	return t.Specs.URLs
}
