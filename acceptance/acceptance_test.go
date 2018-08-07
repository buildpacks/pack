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
)

func TestPack(t *testing.T) {
	spec.Run(t, "pack", testPack, spec.Report(report.Terminal{}))
}

func testPack(t *testing.T, when spec.G, it spec.S) {
	var sourceCodePath string
	var pack string

	it.Before(func() {
		sourceCodePath = filepath.Join("fixtures", "node_app")
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

	when("build", func() {
		it("builds and exports an image", func() {
			cmd := exec.Command(pack, "-dir", sourceCodePath, "-name", "some-org/some-image", "-daemon", "build")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("Failed to build the image: %s, %s", output, err)
			}
			cmd = exec.Command("docker", "run", "--name=some-container", "--rm=true", "-d", "-e", "PORT=8080", "-p", "8080:8080", "some-org/some-image")
			if err := cmd.Run(); err != nil {
				t.Fatal("Failed to run the image", err)
			}
			defer exec.Command("docker", "kill", "some-container").Run()

			time.Sleep(2 * time.Second)
			resp, err := http.Get("http://localhost:8080")
			if err != nil {
				t.Fatal("Container is not running", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Request returned bad status code: %d", resp.StatusCode)
			}
		})
	})
}
