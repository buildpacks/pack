// +build acceptance

package components

import (
	"fmt"
	"regexp"
	"testing"
)

type MixedComponents struct {
	testObject *testing.T
	lifecycle  *TestLifecycle
	pack       *PackExecutor
}

func NewMixedComponents(
	t *testing.T,
	lifecycle *TestLifecycle,
	pack *PackExecutor,
) MixedComponents {
	return MixedComponents{
		testObject: t,
		lifecycle:  lifecycle,
		pack:       pack,
	}
}

func (m MixedComponents) CacheRestorationMessagePatterns(cachedLayer string) []*regexp.Regexp {
	if m.lifecycle.DoesntSupportFeature(DetailedCacheLogging) {
		return []*regexp.Regexp{
			regexp.MustCompile(fmt.Sprintf(`(?i)\[restorer] restoring cached layer '%s'`, cachedLayer)),
			regexp.MustCompile(fmt.Sprintf(`(?i)\[analyzer] using cached launch layer '%s'`, cachedLayer)),
		}
	}

	return []*regexp.Regexp{
		regexp.MustCompile(m.MessagePatternWithPhase(
			fmt.Sprintf("Restoring data for \"%s\" from cache", cachedLayer),
			"restorer",
		)),
		regexp.MustCompile(m.MessagePatternWithPhase(
			fmt.Sprintf(`Restoring metadata for "%s" from app image`, cachedLayer),
			"analyzer",
		)),
	}
}

func (m MixedComponents) CacheReuseMessagePattern(cachedLayer string) *regexp.Regexp {
	if m.lifecycle.DoesntSupportFeature(DetailedCacheLogging) {
		return regexp.MustCompile(fmt.Sprintf(`(?i)\[cacher] reusing layer '%s'`, cachedLayer))
	}

	return regexp.MustCompile(m.MessagePatternWithPhase(
		fmt.Sprintf("Reusing cache layer '%s'", cachedLayer),
		"exporter",
	))
}

func (m MixedComponents) CacheCreationMessagePattern(cachedLayer string) *regexp.Regexp {
	if m.lifecycle.DoesntSupportFeature(DetailedCacheLogging) {
		return regexp.MustCompile(fmt.Sprintf(`(?i)\[cacher] (Caching|adding) layer '%s'`, cachedLayer))
	}

	return regexp.MustCompile(m.MessagePatternWithPhase(
		fmt.Sprintf("Adding cache layer '%s'", cachedLayer),
		"exporter",
	))
}

func (m MixedComponents) ConnectedToTheInternetMessages() []string {
	if m.UsingCreator() {
		return []string{"RESULT: Connected to the internet"}
	}

	return []string{
		"[detector] RESULT: Connected to the internet",
		"[builder] RESULT: Connected to the internet",
	}
}

func (m MixedComponents) DisconnectedFromInternetMessages() []string {
	if m.UsingCreator() {
		return []string{"RESULT: Disconnected from the internet"}
	}

	return []string{
		"[detector] RESULT: Disconnected from the internet",
		"[builder] RESULT: Disconnected from the internet",
	}
}

func (m MixedComponents) UsingCreator() bool {
	return m.lifecycle.SupportsFeature(CreatorInLifecycle) &&
		m.pack.SupportsFeature(CreatorInPack) &&
		m.pack.Supports("trust-builder")
}

func (m MixedComponents) MessagePatternWithPhase(message, phase string) string {
	if m.UsingCreator() {
		return fmt.Sprintf(`(?i)%s`, message)
	}

	return fmt.Sprintf(`(?i)\[%s] %s`, phase, message)
}
