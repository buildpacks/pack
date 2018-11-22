package testhelpers

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgodd/dockerdial"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

func RandString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

// Assert deep equality (and provide useful difference as a test failure)
func AssertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}

// Assert the simplistic pointer (or literal value) equality
func AssertSameInstance(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if actual != expected {
		t.Fatalf("Expected %s and %s to be pointers to the variable", actual, expected)
	}
}

func AssertMatch(t *testing.T, actual string, expected *regexp.Regexp) {
	t.Helper()
	if !expected.Match([]byte(actual)) {
		t.Fatal(cmp.Diff(actual, expected))
	}
}

func AssertError(t *testing.T, actual error, expected string) {
	t.Helper()
	if actual == nil {
		t.Fatalf("Expected an error but got nil")
	}
	if actual.Error() != expected {
		t.Fatalf(`Expected error to equal "%s", got "%s"`, expected, actual.Error())
	}
}

func AssertContains(t *testing.T, actual, expected string) {
	t.Helper()
	if !strings.Contains(actual, expected) {
		t.Fatalf("Expected: '%s' inside '%s'", expected, actual)
	}
}

func AssertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func AssertNotNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual == nil {
		t.Fatal("Expected not nil")
	}
}

func AssertNotEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff == "" {
		t.Fatalf("Expected values to differ: %s", actual)
	}
}

func AssertDirContainsFileWithContents(t *testing.T, dir string, file string, expected string) {
	t.Helper()
	path := filepath.Join(dir, file)
	bytes, err := ioutil.ReadFile(path)
	AssertNil(t, err)
	if string(bytes) != expected {
		t.Fatalf("file %s in dir %s has wrong contents: %s != %s", file, dir, string(bytes), expected)
	}
}

func proxyDockerHostPort(port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	go func() {
		// TODO exit somehow.
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println(err)
				continue
			}
			go func(conn net.Conn) {
				defer conn.Close()
				c, err := dockerdial.Dial("tcp", "localhost:"+port)
				if err != nil {
					log.Println(err)
					return
				}
				defer c.Close()

				go io.Copy(c, conn)
				io.Copy(conn, c)
			}(conn)
		}
	}()
	return nil
}

var runRegistryName, runRegistryPort string
var runRegistryOnce sync.Once

func RunRegistry(t *testing.T, seedRegistry bool) (localPort string) {
	t.Log("run registry")
	t.Helper()
	runRegistryOnce.Do(func() {
		runRegistryName = "test-registry-" + RandString(10)
		Run(t, exec.Command("docker", "run", "--log-driver=none", "-d", "--rm", "-p", ":5000", "--name", runRegistryName, "registry:2"))
		port := Run(t, exec.Command("docker", "inspect", runRegistryName, "-f", `{{index (index (index .NetworkSettings.Ports "5000/tcp") 0) "HostPort"}}`))
		runRegistryPort = strings.TrimSpace(string(port))

		Eventually(t, func() bool {
			_, err := HttpGetE(fmt.Sprintf("http://localhost:%s/v2/", runRegistryPort))
			return err == nil
		}, 10*time.Millisecond, 2*time.Second)

		if os.Getenv("DOCKER_HOST") != "" {
			err := proxyDockerHostPort(runRegistryPort)
			AssertNil(t, err)
		}
		if seedRegistry {
			t.Log("seed registry")
			for _, f := range []func(*testing.T, string) string{DefaultBuildImage, DefaultRunImage, DefaultBuilderImage} {
				Run(t, exec.Command(
					"docker",
					"push",
					f(t, runRegistryPort),
				))
			}
		}
	})
	return runRegistryPort
}

func Eventually(t *testing.T, test func() bool, every time.Duration, timeout time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			if test() {
				return
			}
		case <-timer.C:
			t.Fatalf("timeout on eventually: %v", timeout)
		}
	}
}

func ConfigurePackHome(t *testing.T, packHome, registryPort string) {
	t.Helper()
	AssertNil(t, ioutil.WriteFile(filepath.Join(packHome, "config.toml"), []byte(fmt.Sprintf(`
				default-stack-id = "io.buildpacks.stacks.bionic"
                default-builder = "%s"

				[[stacks]]
				  id = "io.buildpacks.stacks.bionic"
				  build-images = ["%s"]
				  run-images = ["%s"]
			`, DefaultBuilderImage(t, registryPort), DefaultBuildImage(t, registryPort), DefaultRunImage(t, registryPort))), 0666))
}

func StopRegistry(t *testing.T) {
	t.Log("stop registry")
	t.Helper()
	if runRegistryName != "" {
		Run(t, exec.Command("docker", "kill", runRegistryName))
		RunE(exec.Command("bash", "-c", fmt.Sprintf(`docker rmi -f $(docker images --format='{{.ID}}' 'localhost:%s/*')`, runRegistryPort)))
	}
}

var getBuildImageOnce sync.Once

func DefaultBuildImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	getBuildImageOnce.Do(func() {
		if tag == "latest" {
			Run(t, exec.Command("docker", "pull", fmt.Sprintf("packs/build:%s", tag)))
		}
		Run(t, exec.Command(
			"docker", "tag",
			fmt.Sprintf("packs/build:%s", tag),
			fmt.Sprintf("localhost:%s/packs/build:%s", registryPort, tag),
		))
	})
	return fmt.Sprintf("localhost:%s/packs/build:%s", registryPort, tag)
}

var getRunImageOnce sync.Once

func DefaultRunImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	getRunImageOnce.Do(func() {
		if tag == "latest" {
			Run(t, exec.Command("docker", "pull", fmt.Sprintf("packs/run:%s", tag)))
		}
		Run(t, exec.Command("docker", "tag", fmt.Sprintf("packs/run:%s", tag), fmt.Sprintf("localhost:%s/packs/run:%s", registryPort, tag)))
	})
	return fmt.Sprintf("localhost:%s/packs/run:%s", registryPort, tag)
}

var getBuilderImageOnce sync.Once

func DefaultBuilderImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	getBuilderImageOnce.Do(func() {
		if tag == "latest" {
			Run(t, exec.Command("docker", "pull", fmt.Sprintf("packs/samples:%s", tag)))
		}
		Run(t, exec.Command("docker", "tag", fmt.Sprintf("packs/samples:%s", tag), fmt.Sprintf("localhost:%s/packs/samples:%s", registryPort, tag)))
	})
	return fmt.Sprintf("localhost:%s/packs/samples:%s", registryPort, tag)
}

func packTag() string {
	tag := os.Getenv("PACK_TAG")
	if tag == "" {
		return "latest"
	}
	return tag
}

func HttpGet(t *testing.T, url string) string {
	t.Helper()
	txt, err := HttpGetE(url)
	AssertNil(t, err)
	return txt
}

func HttpGetE(url string) (string, error) {
	var client *http.Client
	if os.Getenv("DOCKER_HOST") == "" {
		client = http.DefaultClient
	} else {
		tr := &http.Transport{Dial: dockerdial.Dial}
		client = &http.Client{Transport: tr}
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP Status was bad: %s => %d", url, resp.StatusCode)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CopyWorkspaceToDocker(t *testing.T, srcPath, destVolume string) {
	t.Helper()
	ctrName := uuid.New().String()
	defer exec.Command("docker", "rm", ctrName).Run()
	Run(t, exec.Command("docker", "create", "--name", ctrName, "-v", destVolume+":/workspace", "packs/samples", "true"))
	Run(t, exec.Command("docker", "cp", srcPath+"/.", ctrName+":/workspace/"))
}

func ReadFromDocker(t *testing.T, volume, path string) string {
	t.Helper()
	return Run(t, exec.Command("docker", "run", "--rm", "--log-driver=none", "-v", volume+":/workspace", "packs/samples", "cat", path))
}

func ImageID(t *testing.T, repoName string) string {
	t.Helper()
	id, err := RunE(exec.Command("docker", "images", "--format={{.ID}}", repoName))
	AssertNil(t, err)
	return strings.TrimSpace(id)
}

func Run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	txt, err := RunE(cmd)
	AssertNil(t, err)
	return txt
}

func CleanDefaultImages(t *testing.T, registryPort string) {
	t.Helper()
	Run(t, exec.Command("docker", "rmi", DefaultRunImage(t, registryPort)))
	Run(t, exec.Command("docker", "rmi", DefaultBuildImage(t, registryPort)))
	Run(t, exec.Command("docker", "rmi", DefaultBuilderImage(t, registryPort)))
}

func RunE(cmd *exec.Cmd) (string, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Failed to execute command: %v, %s, %s, %s", cmd.Args, err, stderr.String(), output)
	}

	return string(output), nil
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func trimEmpty(slice []string) []string {
	out := make([]string, 0, len(slice))
	for _, v := range slice {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
