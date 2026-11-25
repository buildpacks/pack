//go:build linux || windows

package client

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/internal/volume"
)

func processVolumes(imgOS string, volumes []string) (processed []string, warnings []string, err error) {
	var parser volume.Parser
	switch "windows" {
	case imgOS:
		parser = volume.NewWindowsParser()
	case runtime.GOOS:
		parser = volume.NewLCOWParser()
	default:
		parser = volume.NewLinuxParser()
	}
	for _, v := range volumes {
		vol, err := parser.ParseMountRaw(v, "")
		if err != nil {
			return nil, nil, errors.Wrapf(err, "platform volume %q has invalid format", v)
		}

		sensitiveDirs := []string{"/cnb", "/layers", "/workspace"}
		if imgOS == "windows" {
			sensitiveDirs = []string{`c:/cnb`, `c:\cnb`, `c:/layers`, `c:\layers`, `c:/workspace`, `c:\workspace`}
		}
		for _, p := range sensitiveDirs {
			if strings.HasPrefix(strings.ToLower(vol.Spec.Target), p) {
				warnings = append(warnings, fmt.Sprintf("Mounting to a sensitive directory %s", style.Symbol(vol.Spec.Target)))
			}
		}

		processed = append(processed, fmt.Sprintf("%s:%s:%s", vol.Spec.Source, vol.Spec.Target, processMode(vol.Mode)))
	}
	return processed, warnings, nil
}

func processMode(mode string) string {
	if mode == "" {
		return "ro"
	}

	return mode
}
