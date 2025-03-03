package image

import (
	"github.com/pkg/errors"
)

// PullPolicy defines a policy for how to manage images
type PullPolicy int

const (
	// PullAlways images, even if they are present
	PullAlways PullPolicy = iota
	// PullNever images, even if they are not present
	PullNever
	// PullIfNotPresent pulls images if they aren't present
	PullIfNotPresent
	// PullIfAvailable pulls images if they are available in the registry else check locally
	PullIfAvailable
)

var nameMap = map[string]PullPolicy{"always": PullAlways, "never": PullNever, "if-not-present": PullIfNotPresent, "try-always": PullIfAvailable, "": PullAlways}

// ParsePullPolicy from string
func ParsePullPolicy(policy string) (PullPolicy, error) {
	if val, ok := nameMap[policy]; ok {
		return val, nil
	}

	return PullAlways, errors.Errorf("invalid pull policy %s", policy)
}

func (p PullPolicy) String() string {
	switch p {
	case PullAlways:
		return "always"
	case PullNever:
		return "never"
	case PullIfNotPresent:
		return "if-not-present"
	case PullIfAvailable:
		return "try-always"
	}

	return ""
}
