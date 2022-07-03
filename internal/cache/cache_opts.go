package cache

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type CacheOpts struct {
	CacheType string
	Format    string
	Source    string
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

		if len(parts) == 2 {
			key := strings.ToLower(parts[0])
			value := strings.ToLower(parts[1])
			switch key {
			case "type":
				if value != "build" {
					return errors.Errorf("invalid cache type '%s'", value)
				}
				c.CacheType = value
			case "format":
				if value != "image" {
					return errors.Errorf("invalid cache format '%s'", value)
				}
				c.Format = value
			case "name":
				c.Source = value
			}
		}

		if len(parts) != 2 {
			return errors.Errorf("invalid field '%s' must be a key=value pair", field)
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
	if c.CacheType != "" {
		cacheFlag += fmt.Sprintf("type=%s;", c.CacheType)
	}
	if c.Format != "" {
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
	if c.CacheType == "" {
		c.CacheType = "build"
	}
	if c.Format == "" {
		c.Format = "volume"
	}
	if c.Source == "" {
		return errors.Errorf("cache 'name' is required")
	}
	return nil
}
