// +build acceptance

package components

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
)

type TestApplication struct {
	testObject *testing.T
	assert     assertions.AssertionManager
	baseDir    string
	descriptor Descriptor
}

type Descriptor int

const (
	TestExcludeDescriptor Descriptor = iota
	TestIncludeDescriptor
)

const (
	excludeDescriptorContents = `
[project]
name = "exclude test"
[[project.licenses]]
type = "MIT"
[build]
exclude = [ "*.sh", "secrets/", "media/metadata" ]
`
	includeDescriptorContents = `
[project]
name = "include test"
[[project.licenses]]
type = "MIT"
[build]
include = [ "*.jar", "media/mountain.jpg", "media/person.png" ]
`
)

func (d Descriptor) fileName() string {
	switch d {
	case TestExcludeDescriptor:
		return "exlude.toml"
	case TestIncludeDescriptor:
		return "include.toml"
	default:
		return ""
	}
}

func (d Descriptor) fileContents() []byte {
	switch d {
	case TestExcludeDescriptor:
		return []byte(excludeDescriptorContents)
	case TestIncludeDescriptor:
		return []byte(includeDescriptorContents)
	default:
		return []byte{}
	}
}

func NewTestApplication(
	t *testing.T,
	assert assertions.AssertionManager,
	parentDir string,
	descriptor Descriptor,
) TestApplication {

	baseDir, err := ioutil.TempDir(parentDir, "descriptor-app")
	assert.Nil(err)

	return TestApplication{
		testObject: t,
		assert:     assert,
		baseDir:    baseDir,
		descriptor: descriptor,
	}
}

// TODO: Can this be a fixture?
func (a TestApplication) Create() {
	// Create test directories and files:
	//
	// ├── cookie.jar
	// ├── secrets
	// │   ├── api_keys.json
	// |   |── user_token
	// ├── media
	// │   ├── mountain.jpg
	// │   └── person.png
	// └── test.sh
	err := os.Mkdir(filepath.Join(a.baseDir, "secrets"), 0755)
	a.assert.Nil(err)
	err = ioutil.WriteFile(filepath.Join(a.baseDir, "secrets", "api_keys.json"), []byte("{}"), 0755)
	a.assert.Nil(err)
	err = ioutil.WriteFile(filepath.Join(a.baseDir, "secrets", "user_token"), []byte("token"), 0755)
	a.assert.Nil(err)

	err = os.Mkdir(filepath.Join(a.baseDir, "media"), 0755)
	a.assert.Nil(err)
	err = ioutil.WriteFile(filepath.Join(a.baseDir, "media", "mountain.jpg"), []byte("fake image bytes"), 0755)
	a.assert.Nil(err)
	err = ioutil.WriteFile(filepath.Join(a.baseDir, "media", "person.png"), []byte("fake image bytes"), 0755)
	a.assert.Nil(err)

	err = ioutil.WriteFile(filepath.Join(a.baseDir, "cookie.jar"), []byte("chocolate chip"), 0755)
	a.assert.Nil(err)
	err = ioutil.WriteFile(filepath.Join(a.baseDir, "test.sh"), []byte("echo test"), 0755)
	a.assert.Nil(err)
}

func (a TestApplication) AddTestExcludeDescriptor() {
	err := ioutil.WriteFile(filepath.Join(a.baseDir, a.descriptor.fileName()), a.descriptor.fileContents(), 0755)
	a.assert.Nil(err)
}

func (a TestApplication) Path() string {
	return a.baseDir
}

func (a TestApplication) DescriptorPath() string {
	return filepath.Join(a.baseDir, a.descriptor.fileName())
}
