package acceptance_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"net/http"
	"os"
	"strings"
	"time"
	"math/rand"
)

func TestPack(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "pack", testPack, spec.Report(report.Terminal{}))
}

func testPack(t *testing.T, when spec.G, it spec.S) {
	var pack string

	it.Before(func() {
		pack = os.Getenv("PACK_PATH")
		if pack == "" {
			t.Fatal("PACK_PATH environment variable is not set")
		}
		if _, err := os.Stat(pack); os.IsNotExist(err) {
			t.Fatal("No file found at PACK_PATH environment variable")
		}
	})

	when("subcommand is invalid", func() {
		it("prints usage", func() {
			cmd := exec.Command(pack, "some-bad-command")
			output, _ := cmd.CombinedOutput()
			if !strings.Contains(string(output), "USAGE: pack build -daemon [ -dir <app-dir> ] -name <image-repo-name>") {
				t.Fatal("Failed to print usage", string(output))
			}
		})
	})

	when("build on daemon", func() {
		var sourceCodePath, repoName, containerName string
		it.Before(func() {
			sourceCodePath = filepath.Join("fixtures", "node_app")
			repoName = "some-org/" + randString(10)
			containerName = "test-" + randString(10)
		})
		it.After(func() {
			exec.Command("docker", "kill", containerName).Run()
			exec.Command("docker", "rmi", repoName).Run()
		})

		it("builds and exports an image", func() {
			cmd := exec.Command(pack, "-dir", sourceCodePath, "-name", repoName, "-daemon", "build")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("Failed to build the image: %s, %s", output, err)
			}
			cmd = exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", "8091:8080", repoName)
			if err := cmd.Run(); err != nil {
				t.Fatal("Failed to run the image", err)
			}

			time.Sleep(2 * time.Second)
			resp, err := http.Get("http://localhost:8091")
			if err != nil {
				t.Fatal("Container is not running", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Request returned bad status code: %d", resp.StatusCode)
			}
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))
}


func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}
