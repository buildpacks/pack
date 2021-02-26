package config

import "fmt"

const (
	WindowsOS = "windows"
	LinuxOS = "linux"
)

func ValidateOS(os string) error {
	switch os {
	case LinuxOS:
		return nil
	case WindowsOS:
		return nil
	default:
		return fmt.Errorf("unknown os type: %q", os)
	}
}

