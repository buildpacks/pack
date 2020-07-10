// +build acceptance
// +build windows

package variables

import (
	"regexp"
)

const PackBinaryName = "pack.exe"

var PackBinaryExp = regexp.MustCompile(`pack-v\d+.\d+.\d+-windows`)
