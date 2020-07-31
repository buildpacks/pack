package config

import "github.com/pkg/errors"

// PullPolicy defines a policy for how to manage images
type PullPolicy int

const (
	// PullAlways images, even if they are present
	PullAlways PullPolicy = iota
	// PullNever images, even if they are not present
	PullNever
	// PullIfNotPresent pulls images if they aren't present
	PullIfNotPresent
)

// ParsePullPolicy from string
func ParsePullPolicy(policy string) (PullPolicy, error) {
	switch policy {
	case "never":
		return PullNever, nil
	case "if-not-present":
		return PullIfNotPresent, nil
	case "always", "": //Default option
		return PullAlways, nil
	default:
		return PullAlways, errors.Errorf("invalid pull policy %s", policy)
	}
}

//ParsePolicyFromPull parses PullPolicy from boolean
func ParsePolicyFromPull(pull bool) PullPolicy {
	if pull {
		return PullAlways
	}

	return PullNever
}
