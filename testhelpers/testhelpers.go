package testhelpers

import (
	"bytes"
	"fmt"
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
				var stderr bytes.Buffer
				cmd := exec.Command("docker", "run", "--log-driver=none", "-i", "-a", "stdin", "-a", "stdout", "-a", "stderr", "--network=host", "alpine/socat", "-", "TCP:localhost:"+port)
				cmd.Stdin = conn
				cmd.Stdout = conn
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					log.Println(stderr.String())
					log.Println(err)
				}
			}(conn)
		}
	}()
	return nil
}

var runRegistryName, runRegistryPort string
var runRegistryOnce sync.Once

func RunRegistry(t *testing.T) (localPort string) {
	t.Helper()
	runRegistryOnce.Do(func() {
		runRegistryName = "test-registry-" + RandString(10)
		Run(t, exec.Command("docker", "run", "--log-driver=none", "-d", "--rm", "-p", ":5000", "--name", runRegistryName, "registry:2"))
		port := Run(t, exec.Command("docker", "inspect", runRegistryName, "-f", `{{index (index (index .NetworkSettings.Ports "5000/tcp") 0) "HostPort"}}`))
		runRegistryPort = strings.TrimSpace(string(port))
		if os.Getenv("DOCKER_HOST") != "" {
			err := proxyDockerHostPort(runRegistryPort)
			AssertNil(t, err)
		}
	})
	return runRegistryPort
}

func StopRegistry(t *testing.T) {
	if runRegistryName != "" {
		Run(t, exec.Command("docker", "kill", runRegistryName))
		RunE(exec.Command("bash", "-c", fmt.Sprintf(`docker rmi -f $(docker images --format='{{.ID}}' 'localhost:%s/*')`, runRegistryPort)))
	}
}

func HttpGet(t *testing.T, url string) string {
	t.Helper()
	if os.Getenv("DOCKER_HOST") == "" {
		resp, err := http.Get(url)
		AssertNil(t, err)
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			t.Fatalf("HTTP Status was bad: %s => %d", url, resp.StatusCode)
		}
		b, err := ioutil.ReadAll(resp.Body)
		AssertNil(t, err)
		return string(b)
	} else {
		return Run(t, exec.Command("docker", "run", "--log-driver=none", "--entrypoint=", "--network=host", "packs/samples", "wget", "-q", "-O", "-", url))
	}
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

func RemoveImage(names ...string) error {
	var firstError error
	for _, name := range names {
		if strings.HasPrefix(name, "localhost:") {
			name = regexp.MustCompile(`localhost:\d+`).ReplaceAllString(name, "localhost:*")
		}
		_, err := RunE(exec.Command("bash", "-c", fmt.Sprintf(`docker rmi -f $(docker images --format='{{.ID}}' %s)`, name)))
		if firstError == nil {
			firstError = err
		}
	}
	return firstError
}

func Run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	txt, err := RunE(cmd)
	AssertNil(t, err)
	return txt
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
