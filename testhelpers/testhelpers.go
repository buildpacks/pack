package testhelpers

import (
	"archive/tar"
	"compress/gzip"
	"context"
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
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgodd/dockerdial"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/archive"
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

func AssertError(t *testing.T, actual error, expected string) {
	t.Helper()
	if actual == nil {
		t.Fatalf("Expected an error but got nil")
	}
	if !strings.Contains(actual.Error(), expected) {
		t.Fatalf(`Expected error to contain "%s", got "%s"`, expected, actual.Error())
	}
}

func AssertContains(t *testing.T, actual, expected string) {
	t.Helper()
	if !strings.Contains(actual, expected) {
		t.Fatalf("Expected: '%s' to contain '%s'", actual, expected)
	}
}

func AssertContainsMatch(t *testing.T, actual, exp string) {
	t.Helper()
	regex := regexp.MustCompile(exp)
	matches := regex.FindAll([]byte(actual), -1)
	if len(matches) < 1 {
		t.Fatalf("Expected: '%s' to match expression '%s'", actual, exp)
	}
}

func AssertNotContains(t *testing.T, actual, expected string) {
	t.Helper()
	if strings.Contains(actual, expected) {
		t.Fatalf("Expected: '%s' to not contain '%s'", actual, expected)
	}
}

func AssertSliceContains(t *testing.T, slice []string, value string) {
	t.Helper()
	for _, s := range slice {
		if value == s {
			return
		}
	}
	t.Fatalf("Expected: '%s' to contain element '%s'", slice, value)
}

func AssertMatch(t *testing.T, actual string, expected string) {
	t.Helper()
	if !regexp.MustCompile(expected).MatchString(actual) {
		t.Fatalf("Expected: '%s' to match regex '%s'", actual, expected)
	}
}

func AssertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if !isNil(actual) {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func AssertNotNil(t *testing.T, actual interface{}) {
	t.Helper()
	if isNil(actual) {
		t.Fatal("Expected not nil")
	}
}

func isNil(value interface{}) bool {
	return value == nil || (reflect.TypeOf(value).Kind() == reflect.Ptr && reflect.ValueOf(value).IsNil())
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

var dockerCliVal *client.Client
var dockerCliOnce sync.Once
var dockerCliErr error

func dockerCli(t *testing.T) *client.Client {
	dockerCliOnce.Do(func() {
		dockerCliVal, dockerCliErr = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	})
	AssertNil(t, dockerCliErr)
	return dockerCliVal
}

func proxyDockerHostPort(port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	go func() {
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

func Eventually(t *testing.T, test func() bool, every time.Duration, timeout time.Duration) {
	t.Helper()

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

func CreateImageOnLocal(t *testing.T, dockerCli *client.Client, repoName, dockerFile string) {
	t.Helper()
	ctx := context.Background()

	buildContext, err := archive.CreateSingleFileTarReader("Dockerfile", dockerFile)
	AssertNil(t, err)

	res, err := dockerCli.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:           []string{repoName},
		SuppressOutput: true,
		Remove:         true,
		ForceRemove:    true,
	})
	AssertNil(t, err)

	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()
}

func CreateImageOnRemote(t *testing.T, dockerCli *client.Client, registryConfig *TestRegistryConfig, repoName, dockerFile string) string {
	t.Helper()
	imageName := registryConfig.RepoName(repoName)

	defer DockerRmi(dockerCli, imageName)
	CreateImageOnLocal(t, dockerCli, imageName, dockerFile)
	AssertNil(t, PushImage(dockerCli, imageName, registryConfig))
	return imageName
}

func DockerRmi(dockerCli *client.Client, repoNames ...string) error {
	var err error
	ctx := context.Background()
	for _, name := range repoNames {
		_, e := dockerCli.ImageRemove(
			ctx,
			name,
			dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true},
		)
		if e != nil && err == nil {
			err = e
		}
	}
	return err
}

func PushImage(dockerCli *client.Client, ref string, registryConfig *TestRegistryConfig) error {
	rc, err := dockerCli.ImagePush(context.Background(), ref, dockertypes.ImagePushOptions{RegistryAuth: registryConfig.RegistryAuth()})
	if err != nil {
		return err
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

func HttpGetE(url string, headers map[string]string) (string, error) {
	var client *http.Client
	if os.Getenv("DOCKER_HOST") == "" {
		client = http.DefaultClient
	} else {
		tr := &http.Transport{Dial: dockerdial.Dial}
		client = &http.Client{Transport: tr}
	}

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", errors.Wrap(err, "making new request")
	}

	for key, val := range headers {
		request.Header.Set(key, val)
	}

	resp, err := client.Do(request)
	if err != nil {
		return "", errors.Wrap(err, "doing request")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP Status was bad: %s => %d", url, resp.StatusCode)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading body")
	}
	return string(b), nil
}

func ImageID(t *testing.T, repoName string) string {
	t.Helper()
	inspect, _, err := dockerCli(t).ImageInspectWithRaw(context.Background(), repoName)
	AssertNil(t, err)
	return inspect.ID
}

func Run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	txt, err := RunE(cmd)
	AssertNil(t, err)
	return txt
}

func RunE(cmd *exec.Cmd) (string, error) {
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("failed to execute command: %v, %s, %s", cmd.Args, err, output)
	}

	return string(output), nil
}

func PullImageWithAuth(dockerCli *client.Client, ref, registryAuth string) error {
	rc, err := dockerCli.ImagePull(context.Background(), ref, dockertypes.ImagePullOptions{RegistryAuth: registryAuth})
	if err != nil {
		return nil
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

func RecursiveCopy(t *testing.T, src, dst string) {
	t.Helper()
	fis, err := ioutil.ReadDir(src)
	AssertNil(t, err)
	for _, fi := range fis {
		if fi.Mode().IsRegular() {
			func() {
				srcFile, err := os.Open(filepath.Join(src, fi.Name()))
				AssertNil(t, err)
				defer srcFile.Close()

				dstFile, err := os.OpenFile(filepath.Join(dst, fi.Name()), os.O_RDWR|os.O_CREATE|os.O_TRUNC, fi.Mode())
				AssertNil(t, err)
				defer dstFile.Close()

				_, err = io.Copy(dstFile, srcFile)
				AssertNil(t, err)

				modifiedtime := time.Time{}
				err = os.Chtimes(filepath.Join(dst, fi.Name()), modifiedtime, modifiedtime)
				AssertNil(t, err)
			}()
		}
		if fi.IsDir() {
			err = os.Mkdir(filepath.Join(dst, fi.Name()), fi.Mode())
			AssertNil(t, err)
			RecursiveCopy(t, filepath.Join(src, fi.Name()), filepath.Join(dst, fi.Name()))
		}
	}
	modifiedtime := time.Time{}
	err = os.Chtimes(dst, modifiedtime, modifiedtime)
	AssertNil(t, err)
	err = os.Chmod(dst, 0775)
	AssertNil(t, err)
}

func RequireDocker(t *testing.T) {
	_, isSet := os.LookupEnv("NO_DOCKER")
	SkipIf(t, isSet, "Skipping because docker daemon unavailable")
}

func SkipIf(t *testing.T, expression bool, reason string) {
	if expression {
		t.Skip(reason)
	}
}

func RunContainer(ctx context.Context, dockerCli *client.Client, id string, stdout io.Writer, stderr io.Writer) error {
	bodyChan, errChan := dockerCli.ContainerWait(ctx, id, container.WaitConditionNextExit)

	if err := dockerCli.ContainerStart(ctx, id, dockertypes.ContainerStartOptions{}); err != nil {
		return errors.Wrap(err, "container start")
	}
	logs, err := dockerCli.ContainerLogs(ctx, id, dockertypes.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return errors.Wrap(err, "container logs stdout")
	}

	copyErr := make(chan error)
	go func() {
		_, err := stdcopy.StdCopy(stdout, stderr, logs)
		copyErr <- err
	}()

	select {
	case body := <-bodyChan:
		if body.StatusCode != 0 {
			return fmt.Errorf("failed with status code: %d", body.StatusCode)
		}
	case err := <-errChan:
		return err
	}
	return <-copyErr
}

func GetFreePort() (string, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return "", err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return "", err
	}
	defer l.Close()

	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port), nil
}

func CreateTgz(t *testing.T, srcDir, tarDir string, mode int64) string {
	t.Helper()

	fh, err := ioutil.TempFile("", "*.tgz")
	AssertNil(t, err)
	defer fh.Close()

	gw := gzip.NewWriter(fh)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	err = archive.WriteDirToTar(
		tw,
		srcDir,
		tarDir,
		0, 0, mode,
	)
	AssertNil(t, err)

	return fh.Name()
}
