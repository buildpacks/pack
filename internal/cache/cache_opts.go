package cache

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type Cache int
type Format int
type CacheOpts struct {
	CacheType Cache
	Format    Format
	Source    string
}

const (
	Build Cache = iota
	Launch
)
const (
	CacheVolume Format = iota
	CacheImage
)

func (c Cache) String() string {
	switch c {
	case Build:
		return "build"
	case Launch:
		return "launch"
	}
	return ""
}

func (f Format) String() string {
	switch f {
	case CacheImage:
		return "image"
	case CacheVolume:
		return "volume"
	}
	return ""
}

func (c *CacheOpts) Set(value string) error {
	csvReader := csv.NewReader(strings.NewReader(value))
	csvReader.Comma = ';'
	fields, err := csvReader.Read()
	if err != nil {
		return err
	}

	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)

		if len(parts) != 2 {
			return errors.Errorf("invalid field '%s' must be a key=value pair", field)
		}

		if len(parts) == 2 {
			key := strings.ToLower(parts[0])
			value := strings.ToLower(parts[1])
			switch key {
			case "type":
				switch value {
				case "build":
					c.CacheType = Build
				case "launch":
					c.CacheType = Launch
				default:
					return errors.Errorf("invalid cache type '%s'", value)
				}
			case "format":
				switch value {
				case "image":
					c.Format = CacheImage
				default:
					return errors.Errorf("invalid cache format '%s'", value)
				}
			case "name":
				c.Source = value
			}
		}
	}

	err = populateMissing(c)
	if err != nil {
		return err
	}
	return nil
}

func (c *CacheOpts) String() string {
	var cacheFlag string
	if c.CacheType.String() != "" {
		cacheFlag += fmt.Sprintf("type=%s;", c.CacheType)
	}
	if c.Format.String() != "" {
		cacheFlag += fmt.Sprintf("format=%s;", c.Format)
	}
	if c.Source != "" {
		cacheFlag += fmt.Sprintf("name=%s", c.Source)
	}
	return cacheFlag
}

func (c *CacheOpts) Type() string {
	return "cache"
}

func populateMissing(c *CacheOpts) error {
	if c.Source == "" {
		return errors.Errorf("cache 'name' is required")
	}
	return nil
}
