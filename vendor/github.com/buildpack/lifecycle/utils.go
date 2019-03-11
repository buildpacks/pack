package lifecycle

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

func WriteTOML(path string, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(data)
}

func escapeIdentifier(id string) string {
	return strings.Replace(id, "/", "_", -1)
}
