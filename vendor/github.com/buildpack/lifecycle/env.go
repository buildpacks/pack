package lifecycle

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type Env struct {
	Getenv  func(key string) string
	Setenv  func(key, value string) error
	Environ func() []string
	Map     map[string][]string
}

func (p *Env) AddRootDir(baseDir string) error {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return err
	}
	for dir, vars := range p.Map {
		newDir := filepath.Join(absBaseDir, dir)
		if _, err := os.Stat(newDir); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		for _, key := range vars {
			value := newDir + prefix(p.Getenv(key), os.PathListSeparator)
			if err := p.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func prefix(s string, prefix byte) string {
	if s == "" {
		return ""
	} else {
		return string(prefix) + s
	}
}

func (p *Env) AddEnvDir(envDir string) error {
	return eachEnvFile(envDir, func(k, v string) error {
		parts := strings.SplitN(k, ".", 2)
		name := parts[0]
		var action string
		if len(parts) > 1 {
			action = strings.ToLower(parts[1])
		}
		switch action {
		case "append":
			return p.Setenv(name, v+p.Getenv(name))
		case "override":
			return p.Setenv(name, v)
		default:
			return p.Setenv(name, v+prefix(p.Getenv(name), os.PathListSeparator))
		}
	})
}

func eachEnvFile(dir string, fn func(k, v string) error) error {
	files, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		value, err := ioutil.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return err
		}
		if err := fn(f.Name(), string(value)); err != nil {
			return err
		}
	}
	return nil
}

func (p *Env) List() []string {
	return p.Environ()
}
