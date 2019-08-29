package paths

import (
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var schemeRegexp = regexp.MustCompile(`^.+://.*`)

func IsURI(ref string) bool {
	return schemeRegexp.MatchString(ref)
}

func FilePathToUri(path string) (string, error) {
	var err error
	if !filepath.IsAbs(path) {
		path, err = filepath.Abs(path)
		if err != nil {
			return "", err
		}
	}

	if runtime.GOOS == "windows" {
		if strings.HasPrefix(path, `\\`) {
			return "file://" + filepath.ToSlash(strings.TrimPrefix(path, `\\`)), nil
		} else {
			return "file:///" + filepath.ToSlash(path), nil
		}
	} else {
		return "file://" + path, nil
	}
}

// examples:
//
// - unix file: file://laptop/some%20dir/file.tgz
//
// - windows drive: file:///C:/Documents%20and%20Settings/file.tgz
//
// - windows share: file://laptop/My%20Documents/file.tgz
//
func UriToFilePath(uri string) (string, error) {
	var (
		osPath = uri
		err    error
	)

	osPath = filepath.FromSlash(strings.TrimPrefix(uri, "file://"))

	if osPath, err = url.PathUnescape(osPath); err != nil {
		return "", nil
	}

	if runtime.GOOS == "windows" {
		if strings.HasPrefix(osPath, `\`) {
			return strings.TrimPrefix(osPath, `\`), nil
		} else {
			return `\\` + osPath, nil
		}
	} else {
		return osPath, nil
	}
}
