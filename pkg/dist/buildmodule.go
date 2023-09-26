package dist

import (
	"strings"

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
	ArchVariant   string         `json:"variant,omitempty" toml:"variant"`
	Distributions Distributions `json:"distributions,omitempty" toml:"distributions,omitempty"`
}

type Targets []Target

func(*Targets) Type() string {
	return "Targets"
}

func(targets *Targets) String() string {
	var sb strings.Builder
	// each target is converted to string & is separated by `,`
	for _,t := range *targets {
		sb.WriteString(t.String() + ",")
	}
	return sb.String()
}

// set the value of multiple targets in a single command where each target is separated by a `,` or pass multiple `--targets` to specify each target individually.
func(targets *Targets) Set(value string) error {
	// spliting multiple targets into a slice of targets by `,` separater
	s := strings.Split(value, ",")
	for i,v := range *targets {
		// for each individual target set it's value from s[i] & getting an error
		err := v.Set(s[i])
		// if error is not nill return error
		if err != nil {
			return err
		}
	}
	return nil
}

func(target *Target) Type() string {
	return "Target"
}

// `--target` is passed in the format of `[os][/arch][/variant]:[name@osversion]`
// one can specify multiple distro versions in the format `name@osversion1@osversion2@...`
// one can specify multiple distros seperated by `;`
// example:
// `name1@osverion1;name2@osversion2;...`
func(target *Target) String() string {
	var s string
	// if target's OS is not nill append it to string
	if target.OS != "" {
		s += target.OS
	}
	// if target's Arch is not nill append it to string
	if target.Arch != "" {
		// if os is not defined append target's arch without '/'
		if s == "" {
			s += target.Arch
		}
		// if os is defined append target's arch separated by '/'
		s += "/" + target.Arch
	}
	// if target's ArchVariant is not nill append it to string
	if target.ArchVariant != "" {
		// if os and/or arch is/are not defined append target's archVariant without '/'
		if s == "" {
			s += target.ArchVariant
		}
		// if os and/or arch is/are not defined append target's archVariant separated by '/'
		s += "/" + target.ArchVariant
	}

	// if distros are nill return the string `s`
	if target.Distributions != nil  {
		// a map of distributions with distro's name as key and versions(slice of string) as value
		var v map[string][]string
		for _,d := range target.Distributions {
			// for each distro add version to map with key as distro's name
			v[d.Name] = append(v[d.Name], d.Versions...)
		}
		// after adding all distributions to map
		// if [os] or [arch] or [archVariant] is defined append distro to string in format `:[name]@[version1]@[version2];[name1]@[version1]@[version2];`
		if s == "" {
			for k,d := range v {
				if len(d) > 0 {
					s += k + "@" + strings.Join(v[k], "@") + ";"
				}
				// if no version is specified append distro to string in format `[name];[name1];[name2];` without any version
				s += k + ";"
			}
		}
		// after adding all distributions to map
		// if [os] or [arch] or [archVariant] is defined append distro to string in format `[name]@[version1]@[version2];[name1]@[version1]@[version2];`
		s += ":"
		for k,d := range v {
			if len(d) > 0 {
				s += k + "@" + strings.Join(v[k], "@") + ";"
			}
			s += k + ";"
		}
	}
	// return the final stringified version of target
	return s
}

func(target *Target) Set(value string) error {
	// each target can only have one `:` to separate [os] [arch] [archVariant] with distro
	osDistro := strings.Split(value, ":")
	if len(osDistro) > 2 {
		return errors.Errorf("invalid format, target should not have more than one `:` separator. got %s number of separators", len(osDistro) - 1)
	}
	var os, distro string

	if len(osDistro) == 2 {
		// get the [os][/arch][/archVariant] at index 0 if osDistro is of length 2
		os = osDistro[0]
		// get [name@osversion] from index 1 if osDistro has length 2
		distro = osDistro[1]
	} else {
		// if osDistro is nil or with length 1 add value of osDistro[0] to
		// assign to `os` variable if string has separator '/'
		// else assign it to distro
		if v := osDistro[0]; strings.ContainsRune(v, '/') {
			os = v
		} else {
			distro = v
		}
	}
	// separate [os] [arch] [archVariant] with separator '/' and assign values to their respective variable
	osArch := strings.SplitN(os, "/", 3)
	if os := osArch[0]; os != "" {
		target.OS = os
	}
	if arch := osArch[1]; arch != "" {
		target.Arch = arch
	}
	if archVariant := osArch[2]; archVariant != "" {
		target.ArchVariant = archVariant
	}

	// split the distros with the separator ';'
	err := target.Distributions.Set(distro)
	if err != nil {
		return err
	}

	return nil
}

func(distros *Distributions) String() string {
	var s strings.Builder
	for _,d := range *distros {
		s.WriteString(d.String() + ";")
	}
	return s.String()
}

func(distros *Distributions) Set(value string) error {
	v := strings.Split(value, ";")
	for i,d := range *distros {
		err := d.Set(v[i])
		if err != nil {
			return err
		}
	}
	return nil
}

func(distros *Distributions) Type() string {
	return "Distributions"
}

func(distro *Distribution) String() string {
	return distro.Name + strings.Join(distro.Versions, "@")
}

func(distro *Distribution) Set(value string) error {
	v := strings.Split(value, "@")
	for i,d := range v {
		if i == 0 {
			distro.Name = d
		}
		distro.Versions = append(distro.Versions, d)
	}
	return nil
}

func(distro *Distribution) Type() string {
	return "Distribution"
}

type Distribution struct {
	Name     string   `json:"name,omitempty" toml:"name,omitempty"`
	Versions []string `json:"versions,omitempty" toml:"versions,omitempty"`
}

type Distributions []Distribution
