// +build acceptance

package assertions

import (
	"testing"

	h "github.com/buildpacks/pack/testhelpers"
)

type AcceptanceAssertionManager struct {
	testObject *testing.T
	assert     h.AssertionManager
}

func NewAcceptanceAssertionManager(t *testing.T, assert h.AssertionManager) AcceptanceAssertionManager {
	return AcceptanceAssertionManager{
		testObject: t,
		assert:     assert,
	}
}

func (a AcceptanceAssertionManager) OutputAssertionManager(output string) OutputAssertionManager {
	return OutputAssertionManager{
		testObject: a.testObject,
		assert:     a.assert,
		output:     output,
	}
}
