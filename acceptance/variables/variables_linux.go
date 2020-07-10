// +build acceptance
// +build linux

package variables

import "regexp"

var PackBinaryExp = regexp.MustCompile(`pack-v\d+.\d+.\d+-linux`)
