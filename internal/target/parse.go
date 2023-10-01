package target

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/internal/warn"
	"github.com/buildpacks/pack/pkg/dist"
)

func ParseTargets(t []string) (targets []dist.Target, warn warn.Warn, err error) {
	for _, v := range t {
		target, w, err := ParseTarget(v)
		warn.AddWarn(w)
		if err != nil {
			return nil, warn, err
		}
		targets = append(targets, target)
	}
	return targets, warn, nil
}

func ParseTarget(t string) (output dist.Target, warn warn.Warn, err error) {
	nonDistro, distros, w, err := getTarget(t)
	warn.AddWarn(w)
	if v, _ := getSliceAt[string](nonDistro, 0); len(nonDistro) <= 1 && v == "" {
		warn.Add(style.Error("os/arch must be defined"))
	}
	if err != nil {
		return output, warn, err
	}
	os, arch, variant,w, err := getPlatform(nonDistro)
	warn.AddWarn(w)
	if err != nil {
		return output, warn, err
	}
	v, w, err := ParseDistros(distros)
	warn.AddWarn(w)
	if err != nil {
		return output, warn, err
	}
	output = dist.Target{
		OS:            os,
		Arch:          arch,
		ArchVariant:   variant,
		Distributions: v,
	}
	return output, warn, err
}

func ParseDistros(distroSlice string) (distros []dist.Distribution, warn warn.Warn, err error) {
	distro := strings.Split(distroSlice, ";")
	if l := len(distro); l == 1 && distro[0] == "" {
		return nil, warn, err
	}
	for _, d := range distro {
		v, w, err := ParseDistro(d)
		warn.AddWarn(w)
		if err != nil {
			return nil, warn, err
		}
		distros = append(distros, v)
	}
	return distros, warn, nil
}

func ParseDistro(distroString string) (distro dist.Distribution, warn warn.Warn, err error) {
	d := strings.Split(distroString, "@")
	if d[0] == "" || len(d) == 0 {
		return distro, warn, errors.Errorf("distro's versions %s cannot be specified without distro's name", style.Symbol("@"+strings.Join(d[1:], "@")))
	}
	if len(d) <= 2 && d[0] == "" {
		warn.Add(fmt.Sprintf("distro with name %s has no specific version!", style.Symbol(d[0])))
	}
	distro.Name = d[0]
	distro.Versions = d[1:]
	return distro, warn, err
}

func getTarget(t string) (nonDistro []string, distros string, warn warn.Warn, err error) {
	target := strings.Split(t, ":")
	if i, err := getSliceAt[string](target, 0); err != nil {
		return nonDistro, distros, warn, errors.Errorf("invalid target %s, atleast one of [os][/arch][/archVariant] must be specified", t)
	} else if len(target) == 2 && target[0] == "" {
		v,_ := getSliceAt[string](target, 1)
		warn.Add(style.Warn("adding distros %s without [os][/arch][/variant]", v))
	} else {
		nonDistro = strings.Split(i, "/")
	}
	if i, err := getSliceAt[string](target, 1); err == nil {
		distros = i
	}
	return nonDistro, distros, warn, err
}

func getSliceAt[T interface{}](slice []T, index int) (value T, err error) {
	if index < 0 || index >= len(slice) {
		return value, errors.Errorf("index out of bound, cannot access item at index %d of slice with length %d", index, len(slice))
	}

	return slice[index], err
}
