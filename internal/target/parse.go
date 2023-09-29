package target

import (
	"fmt"
	"strings"

	"github.com/buildpacks/pack/pkg/dist"
)

func ParseTargets(t []string) (targets []dist.Target, err error) {
	for _, v := range t {
		target, err := ParseTarget(v)
		if err != nil {
			return targets, err
		}
		targets = append(targets, target)
	}
	return targets, err
}

func ParseTarget(t string) (output dist.Target, err error) {
	nonDistro, distros, err := getTarget(t)
	if err != nil {
		return output, err
	}
	os, arch, variant, err := getPlatform(nonDistro)
	if err != nil {
		return output, err
	}
	output = dist.Target{
		OS:            os,
		Arch:          arch,
		ArchVariant:   variant,
		Distributions: ParseDistros(distros),
	}
	return output, err
}

func ParseDistros(distroSlice []string) (distros []dist.Distribution) {
	for _, d := range distroSlice {
		distros = append(distros, ParseDistro(d))
	}
	return distros
}

func ParseDistro(distroString string) (distro dist.Distribution) {
	d := strings.Split(distroString, "@")
	distro = dist.Distribution{
		Name:     d[0],
		Versions: d[1:],
	}
	return distro
}

func getTarget(t string) (nonDistro, distros []string, err error) {
	target := strings.Split(t, ":")
	if i, err := getSliceAt[string](target, 0); err != nil {
		return nonDistro, distros, fmt.Errorf("invalid target %s, atleast one of [os][/arch][/archVariant] must be specified", t)
	} else {
		nonDistro = strings.Split(i, "/")
	}
	if i, err := getSliceAt[string](target, 1); err != nil {
		distros = strings.Split(i, ";")
	}
	return nonDistro, distros, err
}

func getSliceAt[T interface{}](slice []T, index int) (value T, err error) {
	if index < 0 || index >= len(slice) {
		return value, fmt.Errorf("index out of bound, cannot access item at index %d of slice with length %d", index, len(slice))
	}

	return slice[index], err
}
