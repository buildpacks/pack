// +build windows

package logging

import (
	"regexp"

	"github.com/fatih/color"
)

func init() {
	// the color library writes gobbledygook to the console on window so disable it
	color.NoColor = true
}

var colorCodeMatcher = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Remove all ANSI color information.
func maybeStripColor(b []byte) string {
	return string(colorCodeMatcher.ReplaceAll(b, []byte("")))
}
