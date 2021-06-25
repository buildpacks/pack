package config

import "fmt"

const (
	LocalImagePreference  = "prefer-local"
	RemoteImagePreference = "prefer-remote"
	OnlyLocalImage        = "only-local"
	OnlyRemoteImage       = "only-remote"
)

func ValidateImagePreference(os string) error {
	switch os {
	case LocalImagePreference:
		return nil
	case RemoteImagePreference:
		return nil
	case OnlyLocalImage:
		return nil
	case OnlyRemoteImage:
		return nil
	default:
		return fmt.Errorf("unknown image preference: %q", os)
	}
}
