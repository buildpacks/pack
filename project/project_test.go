package project

import (
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/pack/testhelpers"
)

func TestProject(t *testing.T) {
	h.RequireDocker(t)
	color.Disable(true)
	defer color.Disable(false)
	rand.Seed(time.Now().UTC().UnixNano())

	spec.Run(t, "Provider", testProject, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testProject(t *testing.T, when spec.G, it spec.S) {
	when("#ReadProjectDescriptor", func() {
		it("should parse a valid project.toml file", func() {
			projectToml := `
[project]
name = "gallant"
source-url = "git@github.com:buildpacks/pack.git"
version = "v0.18.1-2-g83484845"
[[project.licenses]]
type = "MIT"
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
[metadata]
pipeline = "Lucerne"
`
			tmpProjectToml, err := createTmpProjectTomlFile(projectToml)
			if err != nil {
				t.Fatal(err)
			}

			projectDescriptor, err := ReadProjectDescriptor(tmpProjectToml.Name())
			if err != nil {
				t.Fatal(err)
			}

			var expected string

			expected = "gallant"
			if projectDescriptor.Project.Name != expected {
				t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
					expected, projectDescriptor.Project.Name)
			}
			expected = "v0.18.1-2-g83484845"
			if projectDescriptor.Project.Version != expected {
				t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
					expected, projectDescriptor.Project.Version)

			}
			expected = "git@github.com:buildpacks/pack.git"
			if projectDescriptor.Project.SourceURL != expected {
				t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
					expected, projectDescriptor.Project.SourceURL)

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

			expected = "MIT"
			if projectDescriptor.Project.Licenses[0].Type != expected {
				t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
					expected, projectDescriptor.Project.Licenses[0].Type)
			}

			expected = "Lucerne"
			if projectDescriptor.Metadata["pipeline"] != expected {
				t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
					expected, projectDescriptor.Metadata["pipeline"])
			}
		})

		it("should create empty build ENV", func() {
			projectToml := `
[project]
name = "gallant"
`
			tmpProjectToml, err := createTmpProjectTomlFile(projectToml)
			if err != nil {
				t.Fatal(err)
			}

			projectDescriptor, err := ReadProjectDescriptor(tmpProjectToml.Name())
			if err != nil {
				t.Fatal(err)
			}

			expected := 0
			if len(projectDescriptor.Build.Env) != 0 {
				t.Fatalf("Expected\n-----\n%d\n-----\nbut got\n-----\n%d\n",
					expected, len(projectDescriptor.Build.Env))
			}

			for _, envVar := range projectDescriptor.Build.Env {
				t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
					"[]", envVar)
			}
		})

		it("should fail for an invalid project.toml path", func() {
			_, err := ReadProjectDescriptor("/path/that/does/not/exist/project.toml")

			if !os.IsNotExist(err) {
				t.Fatalf("Expected\n-----\n%#v\n-----\nbut got\n-----\n%#v\n",
					"project.toml does not exist error", "no error")
			}
		})

		it("should enforce mutual exclusivity between exclude and include", func() {
			projectToml := `
[project]
name = "bad excludes and includes"

[build]
exclude = [ "*.jar" ]
include = [ "*.jpg" ]
`
			tmpProjectToml, err := createTmpProjectTomlFile(projectToml)
			if err != nil {
				t.Fatal(err)
			}
			_, err = ReadProjectDescriptor(tmpProjectToml.Name())
			if err == nil {
				t.Fatalf(
					"Expected error for having both exclude and include defined")
			}
		})

		it("should have an id or uri defined for buildpacks", func() {
			projectToml := `
[project]
name = "missing buildpacks id and uri"

[[build.buildpacks]]
version = "1.2.3"
`
			tmpProjectToml, err := createTmpProjectTomlFile(projectToml)
			if err != nil {
				t.Fatal(err)
			}

			_, err = ReadProjectDescriptor(tmpProjectToml.Name())
			if err == nil {
				t.Fatalf("Expected error for NOT having id or uri defined for buildpacks")
			}
		})

		it("should not allow both uri and version", func() {
			projectToml := `
[project]
name = "cannot have both uri and version defined"

[[build.buildpacks]]
uri = "https://example.com/buildpack"
version = "1.2.3"
`
			tmpProjectToml, err := createTmpProjectTomlFile(projectToml)
			if err != nil {
				t.Fatal(err)
			}

			_, err = ReadProjectDescriptor(tmpProjectToml.Name())
			if err == nil {
				t.Fatal("Expected error for having both uri and version defined for a buildpack(s)")
			}
		})

		it("should require either a type or uri for licenses", func() {
			projectToml := `
[project]
name = "licenses should have either a type or uri defined"

[[project.licenses]]
`
			tmpProjectToml, err := createTmpProjectTomlFile(projectToml)
			if err != nil {
				t.Fatal(err)
			}

			_, err = ReadProjectDescriptor(tmpProjectToml.Name())
			if err == nil {
				t.Fatal("Expected error for having neither type or uri defined for licenses")
			}
		})
	})
}

func createTmpProjectTomlFile(projectToml string) (*os.File, error) {
	tmpProjectToml, err := ioutil.TempFile(os.TempDir(), "project-")
	if err != nil {
		log.Fatal("Failed to create temporary project toml file", err)
	}

	if _, err := tmpProjectToml.Write([]byte(projectToml)); err != nil {
		log.Fatal("Failed to write to temporary file", err)
	}
	return tmpProjectToml, err
}
