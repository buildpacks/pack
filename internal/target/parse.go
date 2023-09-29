package target

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/dist"
)

func ParseTargets(t []string) (targets []dist.Target, err error) {
	for _, v := range t {
		target, err := ParseTarget(v)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func ParseTarget(t string) (output dist.Target, err error) {
	nonDistro, distros, err := getTarget(t)
	if len(nonDistro) <= 1 && nonDistro[0] == "" {
		return output, errors.Errorf("%s", style.Error("os/arch must be defined"))
	}
	if err != nil {
		return output, err
	}
	os, arch, variant, err := getPlatform(nonDistro)
	if err != nil {
		return output, err
	}
	v, err := ParseDistros(distros)
	if err != nil {
		return output, err
	}
	output = dist.Target{
		OS:            os,
		Arch:          arch,
		ArchVariant:   variant,
		Distributions: v,
	}
	return output, err
}

func ParseDistros(distroSlice string) (distros []dist.Distribution, err error) {
	distro := strings.Split(distroSlice, ";")
	if l := len(distro); l == 1 && distro[0] == "" {
		return nil, nil
	}
	for _, d := range distro {
		v, err, isWarn := ParseDistro(d)
		if err != nil && !isWarn {
			return nil, err
		}
		distros = append(distros, v)
	}
	return distros, nil
}

func ParseDistro(distroString string) (distro dist.Distribution, err error, isWarn bool) {
	d := strings.Split(distroString, "@")
	if d[0] == "" || len(d) == 0 {
		return distro, errors.Errorf("distro's versions %s cannot be specified without distro's name", style.Symbol("@"+strings.Join(d[1:], "@"))), false
	}
	if len(d) <= 2 && d[0] == "" {
		return distro, errors.Errorf("distro with name %s has no specific version!", style.Symbol(d[0])), true
	}
	distro.Name = d[0]
	distro.Versions = d[1:]
	return distro, nil, false
}

func getTarget(t string) (nonDistro []string, distros string, err error) {
	target := strings.Split(t, ":")
	if i, err := getSliceAt[string](target, 0); err != nil || len(target) == 0 {
		return nonDistro, distros, errors.Errorf("invalid target %s, atleast one of [os][/arch][/archVariant] must be specified", t)
	} else {
		nonDistro = strings.Split(i, "/")
	}
	if i, err := getSliceAt[string](target, 1); err == nil {
		distros = i
	}
	return nonDistro, distros, err
}

func getSliceAt[T interface{}](slice []T, index int) (value T, err error) {
	if index < 0 || index >= len(slice) {
		return value, errors.Errorf("index out of bound, cannot access item at index %d of slice with length %d", index, len(slice))
	}

	return slice[index], err
}
