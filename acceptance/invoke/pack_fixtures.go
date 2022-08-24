//go:build acceptance
// +build acceptance

package invoke

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	h "github.com/buildpacks/pack/testhelpers"
)

type PackFixtureManager struct {
	testObject *testing.T
	assert     h.AssertionManager
	locations  []string
}

func (m PackFixtureManager) FixtureLocation(name string) string {
	m.testObject.Helper()

	for _, dir := range m.locations {
		fixtureLocation := filepath.Join(dir, name)
		_, err := os.Stat(fixtureLocation)
		if !os.IsNotExist(err) {
			return fixtureLocation
		}
	}

	m.testObject.Fatalf("fixture %s does not exist in %v", name, m.locations)

	return ""
}

func (m PackFixtureManager) VersionedFixtureOrFallbackLocation(pattern, version, fallback string) string {
	m.testObject.Helper()

	versionedName := fmt.Sprintf(pattern, version)

	for _, dir := range m.locations {
		fixtureLocation := filepath.Join(dir, versionedName)
		m.testObject.Logf("looking up possible fixture at: %s", fixtureLocation)
		_, err := os.Stat(fixtureLocation)
		if !os.IsNotExist(err) {
			return fixtureLocation
		}
	}

	return m.FixtureLocation(fallback)
}

func (m PackFixtureManager) TemplateFixture(templateName string, templateData map[string]interface{}) string {
	m.testObject.Helper()

	outputTemplate, err := ioutil.ReadFile(m.FixtureLocation(templateName))
	m.assert.Nil(err)

	return m.fillTemplate(outputTemplate, templateData)
}

func (m PackFixtureManager) TemplateVersionedFixture(
	versionedPattern, version, fallback string,
	templateData map[string]interface{},
) string {
	m.testObject.Helper()
	outputTemplate, err := ioutil.ReadFile(m.VersionedFixtureOrFallbackLocation(versionedPattern, version, fallback))
	m.assert.Nil(err)

	return m.fillTemplate(outputTemplate, templateData)
}

func (m PackFixtureManager) TemplateVersionedFixtureToFile(
	tmpDir, versionedPattern, version, fallback string,
	templateData map[string]interface{},
) string {
	m.testObject.Helper()
	fixturePath := m.VersionedFixtureOrFallbackLocation(versionedPattern, version, fallback)
	m.testObject.Logf("using %s for fixture %s", fixturePath, fallback)

	outputTemplate, err := ioutil.ReadFile(fixturePath)
	m.assert.Nil(err)

	tmpFile, err := ioutil.TempFile(tmpDir, "*-"+fallback)
	defer tmpFile.Close()

	_, err = io.WriteString(tmpFile, m.fillTemplate(outputTemplate, templateData))
	m.assert.Nil(err)
	return tmpFile.Name()
}

func (m PackFixtureManager) TemplateFixtureToFile(tmpDir string, fixture string, data map[string]interface{}) string {
	tmpFile, err := ioutil.TempFile(tmpDir, fixture+"-*")
	defer tmpFile.Close()

	_, err = io.WriteString(tmpFile, m.TemplateFixture(fixture, data))
	m.assert.Nil(err)
	return tmpFile.Name()
}

func (m PackFixtureManager) fillTemplate(templateContents []byte, data map[string]interface{}) string {
	tpl, err := template.New("").
		Funcs(template.FuncMap{
			"StringsJoin": strings.Join,
			"StringsDoubleQuote": func(s []string) []string {
				result := []string{}
				for _, str := range s {
					result = append(result, fmt.Sprintf(`"%s"`, str))
				}
				return result
			},
			"StringsEscapeBackslash": func(s string) string {
				result := []rune{}
				for _, elem := range s {
					switch {
					case elem == '\\':
						result = append(result, '\\', '\\')
					default:
						result = append(result, elem)
					}
				}
				return string(result)
			},
		}).
		Parse(string(templateContents))
	m.assert.Nil(err)

	var templatedContent bytes.Buffer
	err = tpl.Execute(&templatedContent, data)
	m.assert.Nil(err)

	return templatedContent.String()
}
