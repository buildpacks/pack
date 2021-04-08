package config

import "fmt"

const (
	FileFormat = "file"
	ImageFormat   = "image"
)

func ValidateFormat(format string) error {
	switch format {
	case FileFormat:
		return nil
	case ImageFormat:
		return nil
	default:
		return fmt.Errorf("unknown format type: %q", format)
	}
}
