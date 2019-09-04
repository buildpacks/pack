package api

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/style"
)

var regex = regexp.MustCompile(`^v?(\d+)\.(\d*)$`)

type Version struct {
	major,
	minor uint64
}

func MustParse(v string) *Version {
	version, err := NewVersion(v)
	if err != nil {
		panic(err)
	}

	return version
}

func NewVersion(v string) (*Version, error) {
	matches := regex.FindAllStringSubmatch(v, -1)
	if len(matches) == 0 {
		return nil, errors.Errorf("could not parse %s as version", style.Symbol(v))
	}

	var (
		major, minor uint64
		err          error
	)
	if len(matches[0]) == 3 {
		major, err = strconv.ParseUint(matches[0][1], 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing major %s", style.Symbol(matches[0][1]))
		}

		minor, err = strconv.ParseUint(matches[0][2], 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing minor %s", style.Symbol(matches[0][2]))
		}
	} else {
		return nil, errors.Errorf("could not parse version %s", style.Symbol(v))
	}

	return &Version{major: major, minor: minor}, nil
}

func (v *Version) String() string {
	return fmt.Sprintf("%d.%d", v.major, v.minor)
}

// MarshalText makes Version satisfy the encoding.TextMarshaler interface.
func (v *Version) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

// UnmarshalText makes Version satisfy the encoding.TextUnmarshaler interface.
func (v *Version) UnmarshalText(text []byte) error {
	s := string(text)

	parsedVersion, err := NewVersion(s)
	if err != nil {
		return errors.Wrapf(err, "invalid api version %s", s)
	}

	v.major = parsedVersion.major
	v.minor = parsedVersion.minor

	return nil
}

// SupportsVersion determines whether the argument version is compatible based on matching `major` version with the
// exception of pre-stable version. Any version with `major` equal to 0 is not compatible if `minor` value does not
// match.
func (v *Version) SupportsVersion(v2 *Version) bool {
	if v.Compare(v2) == 0 {
		return true
	}

	if v != nil && v2 != nil {
		if v.major > 0 && v.major == v2.major {
			return true
		}
	}

	return false
}

func (v *Version) Compare(v2 *Version) int {
	if v == nil && v2 == nil {
		return 0
	}

	if v == nil {
		return -1
	}

	if v2 == nil {
		return 1
	}

	if v.major != v2.major {
		if v.major < v2.major {
			return -1
		}

		if v.major > v2.major {
			return 1
		}
	}

	if v.minor != v2.minor {
		if v.minor < v2.minor {
			return -1
		}

		if v.minor > v2.minor {
			return 1
		}
	}

	return 0
}
