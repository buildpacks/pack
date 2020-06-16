// +build acceptance
// +build darwin

package variables

import "regexp"

var PackBinaryExp = regexp.MustCompile(`pack-v\d+.\d+.\d+-macos`)
