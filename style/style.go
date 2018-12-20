package style

import (
	"fmt"
	"github.com/fatih/color"
)

var Noop = func(format string, a ...interface{}) string {
	return color.WhiteString("") + fmt.Sprintf(format, a...)
}

var Symbol = func(format string, a ...interface{}) string {
	if color.NoColor {
		format = fmt.Sprintf("'%s'", format)
	}
	return Key(format, a...)
}

var Key = color.MagentaString

var Tip = color.New(color.FgGreen, color.Bold).SprintfFunc()

var Error = color.New(color.FgRed, color.Bold).SprintfFunc()

var Step = func(format string, a ...interface{}) string {
	return color.CyanString("===> "+format, a...)
}

var Prefix = color.CyanString

var TimestampColorCode = color.FgHiBlack
