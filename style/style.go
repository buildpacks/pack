package style

import (
	"fmt"
	"github.com/fatih/color"
)

var Symbol = func(format string, a ...interface{}) string {
	if color.NoColor {
		format = fmt.Sprintf("'%s'", format)
	}
	return color.New(color.FgMagenta).Sprintf(format, a...)
}

var Tip = color.New(color.FgHiGreen, color.Bold).SprintfFunc()

var Error = color.New(color.FgRed, color.Bold).SprintfFunc()

var Step = func(format string, a ...interface{}) string {
	return color.HiCyanString("===> "+format, a...)
}

var Prefix = color.HiCyanString
