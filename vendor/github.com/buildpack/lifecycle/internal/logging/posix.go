// +build !windows

package logging

import (
	"regexp"

	"github.com/fatih/color"
)

var colorCodeMatcher = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func maybeStripColor(b []byte) string {
	if color.NoColor {
		return string(colorCodeMatcher.ReplaceAll(b, []byte("")))
	}
	return string(b)
}
