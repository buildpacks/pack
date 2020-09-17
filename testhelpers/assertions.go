package testhelpers

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type AssertionManager struct {
	testObject *testing.T
}

func NewAssertionManager(testObject *testing.T) AssertionManager {
	return AssertionManager{
		testObject: testObject,
	}
}

func (a AssertionManager) TrimmedEq(actual, expected string) {
	a.testObject.Helper()

	actualLines := strings.Split(actual, "\n")
	expectedLines := strings.Split(expected, "\n")
	for lineIdx, line := range actualLines {
		actualLines[lineIdx] = strings.TrimRight(line, "\t \n")
	}

	for lineIdx, line := range expectedLines {
		expectedLines[lineIdx] = strings.TrimRight(line, "\t \n")
	}

	actualTrimmed := strings.Join(actualLines, "\n")
	expectedTrimmed := strings.Join(expectedLines, "\n")

	a.Equal(actualTrimmed, expectedTrimmed)
}

func (a AssertionManager) AssertTrimmedContains(actual, expected string) {
	a.testObject.Helper()

	actualLines := strings.Split(actual, "\n")
	expectedLines := strings.Split(expected, "\n")
	for lineIdx, line := range actualLines {
		actualLines[lineIdx] = strings.TrimRight(line, "\t \n")
	}

	for lineIdx, line := range expectedLines {
		expectedLines[lineIdx] = strings.TrimRight(line, "\t \n")
	}

	actualTrimmed := strings.Join(actualLines, "\n")
	expectedTrimmed := strings.Join(expectedLines, "\n")

	a.Contains(actualTrimmed, expectedTrimmed)
}

func (a AssertionManager) Equal(actual, expected interface{}) {
	a.testObject.Helper()

	if diff := cmp.Diff(actual, expected); diff != "" {
		a.testObject.Fatalf(diff)
	}
}

func (a AssertionManager) Nil(actual interface{}) {
	a.testObject.Helper()

	if !isNil(actual) {
		a.testObject.Fatalf("expected nil: %v", actual)
	}
}

func (a AssertionManager) Succeeds(actual interface{}) {
	a.testObject.Helper()

	a.Nil(actual)
}

func (a AssertionManager) Fails(actual interface{}) {
	a.testObject.Helper()

	a.NotNil(actual)
}

func (a AssertionManager) NilWithMessage(actual interface{}, message string) {
	a.testObject.Helper()

	if !isNil(actual) {
		a.testObject.Fatalf("expected nil: %s: %s", actual, message)
	}
}

func (a AssertionManager) NotNil(actual interface{}) {
	a.testObject.Helper()

	if isNil(actual) {
		a.testObject.Fatal("expect not nil")
	}
}

func (a AssertionManager) Contains(actual, expected string) {
	a.testObject.Helper()

	if !strings.Contains(actual, expected) {
		a.testObject.Fatalf(
			"Expected '%s' to contain '%s'\n\nDiff:%s",
			actual,
			expected,
			cmp.Diff(expected, actual),
		)
	}
}

func (a AssertionManager) ContainsF(actual, expected string, formatArgs ...interface{}) {
	a.testObject.Helper()

	a.Contains(actual, fmt.Sprintf(expected, formatArgs...))
}

// ContainsWithMessage will fail if expected is not contained within actual, messageFormat will be printed as the
// failure message, with actual interpolated in the message
func (a AssertionManager) ContainsWithMessage(actual, expected, messageFormat string) {
	a.testObject.Helper()

	if !strings.Contains(actual, expected) {
		a.testObject.Fatalf(messageFormat, actual)
	}
}

func (a AssertionManager) ContainsAll(actual string, expected ...string) {
	a.testObject.Helper()

	for _, e := range expected {
		a.Contains(actual, e)
	}
}

func (a AssertionManager) Matches(actual string, pattern *regexp.Regexp) {
	a.testObject.Helper()

	if !pattern.MatchString(actual) {
		a.testObject.Fatalf("Expected '%s' to match regex '%s'", actual, pattern)
	}
}

func (a AssertionManager) NoMatches(actual string, pattern *regexp.Regexp) {
	a.testObject.Helper()

	if pattern.MatchString(actual) {
		a.testObject.Fatalf("Expected '%s' not to match regex '%s'", actual, pattern)
	}
}

func (a AssertionManager) MatchesAll(actual string, patterns ...*regexp.Regexp) {
	a.testObject.Helper()

	for _, pattern := range patterns {
		a.Matches(actual, pattern)
	}
}

func (a AssertionManager) NotContains(actual, expected string) {
	a.testObject.Helper()

	if strings.Contains(actual, expected) {
		a.testObject.Fatalf("Expected '%s' not to be in '%s'", expected, actual)
	}
}

// NotContainWithMessage will fail if expected is contained within actual, messageFormat will be printed as the failure
// message, with actual interpolated in the message
func (a AssertionManager) NotContainWithMessage(actual, expected, messageFormat string) {
	a.testObject.Helper()

	if strings.Contains(actual, expected) {
		a.testObject.Fatalf(messageFormat, actual)
	}
}

func (a AssertionManager) Error(actual error) {
	a.testObject.Helper()

	if actual == nil {
		a.testObject.Fatal("Expected an error but got nil")
	}
}

func (a AssertionManager) ErrorContains(actual error, expected string) {
	a.testObject.Helper()

	if actual == nil {
		a.testObject.Fatalf("Expected %q an error but got nil", expected)
	}

	a.Contains(actual.Error(), expected)
}
