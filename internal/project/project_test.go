package project

import (
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestDecodeSimple(t *testing.T) {
	projectToml := `
[project]
name = "gallant"

[build]
exclude = [ "*.jar" ]

[[build.buildpacks]]
id = "example/lua"
version = "1.0"

[[build.buildpacks]]
uri = "https://example.com/buildpack"

[[build.env]]
name = "JAVA_OPTS"
value = "-Xmx300m"
`
	var projectDescriptor Descriptor
	_, err := toml.Decode(projectToml, &projectDescriptor)
	if err != nil {
		t.Fatal(err)
	}

	var expected string

	expected = "gallant"
	if projectDescriptor.Project.Name != expected {
		t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
			expected, projectDescriptor.Project.Name)
	}

	expected = "example/lua"
	if projectDescriptor.Build.Buildpacks[0].ID != expected {
		t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
			expected, projectDescriptor.Build.Buildpacks[0].ID)
	}

	expected = "1.0"
	if projectDescriptor.Build.Buildpacks[0].Version != expected {
		t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
			expected, projectDescriptor.Build.Buildpacks[0].Version)
	}

	expected = "https://example.com/buildpack"
	if projectDescriptor.Build.Buildpacks[1].URI != expected {
		t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
			expected, projectDescriptor.Build.Buildpacks[1].URI)
	}

	expected = "JAVA_OPTS"
	if projectDescriptor.Build.Env[0].Name != expected {
		t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
			expected, projectDescriptor.Build.Env[0].Name)
	}

	expected = "-Xmx300m"
	if projectDescriptor.Build.Env[0].Value != expected {
		t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
			expected, projectDescriptor.Build.Env[0].Value)
	}
}

func TestFileDoesNotExist(t *testing.T) {
	_, err := ReadProjectDescriptor("/path/that/does/not/exist/project.toml")

	if !os.IsNotExist(err) {
		t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
			"project.toml does not exist error", "no error")
	}
}

func TestReadFile(t *testing.T) {
	tmpFile, err := ioutil.TempFile(os.TempDir(), "project-")
	if err != nil {
		log.Fatal("Cannot create temporary file", err)
	}

	defer os.Remove(tmpFile.Name())

	projectToml := `
[project]
name = "gallant"
`

	if _, err = tmpFile.Write([]byte(projectToml)); err != nil {
		log.Fatal("Failed to write to temporary file", err)
	}

	// Close the file
	if err := tmpFile.Close(); err != nil {
		log.Fatal(err)
	}

	val, err := ReadProjectDescriptor(tmpFile.Name())

	if err != nil {
		t.Fatal(err)
	}

	var expected = "gallant"
	if val.Project.Name != expected {
		t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
			expected, val.Project.Name)
	}
}

func TestEmtpyEnv(t *testing.T) {
	projectToml := `
[project]
name = "gallant"
`
	var val Descriptor
	_, err := toml.Decode(projectToml, &val)
	if err != nil {
		t.Fatal(err)
	}

	expected := 0
	if len(val.Build.Env) != 0 {
		t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
			expected, string(len(val.Build.Env)))
	}

	for _, envVar := range val.Build.Env {
		t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
			"[]", envVar)
	}
}

func TestValidateIncludeExclude(t *testing.T) {
	projectToml := `
[project]
name = "bad excludes and includes"

[build]
exclude = [ "*.jar" ]
include = [ "*.jpg" ]
`
	var projectDescriptor Descriptor
	_, err := toml.Decode(projectToml, &projectDescriptor)
	if err != nil {
		t.Fatal(err)
	}

	err = projectDescriptor.validate()
	if err == nil {
		t.Fatalf(
			"Expected error for having both exclude and include defined")
	}
}

func TestValidateBuildpacksMissingIdUri(t *testing.T) {
	projectToml := `
[project]
name = "missing buildpacks id and uri"

[[build.buildpacks]]
version = "1.2.3"
`
	var projectDescriptor Descriptor
	_, err := toml.Decode(projectToml, &projectDescriptor)
	if err != nil {
		t.Fatal(err)
	}

	err = projectDescriptor.validate()
	if err == nil {
		t.Fatalf("Expected error for NOT having id or uri defined for buildpacks")
	}
}

func TestValidateBuildpacksUriVersion(t *testing.T) {
	projectToml := `
[project]
name = "cannot have both uri and version defined"

[[build.buildpacks]]
uri = "https://example.com/buildpack"
version = "1.2.3"
`
	var projectDescriptor Descriptor
	_, err := toml.Decode(projectToml, &projectDescriptor)
	if err != nil {
		t.Fatal(err)
	}

	err = projectDescriptor.validate()
	if err == nil {
		t.Fatal("Expected error for having both uri and version defined for a buildpack(s)")
	}
}
