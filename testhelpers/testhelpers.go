package testhelpers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
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
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/stringset"
	"github.com/buildpacks/pack/internal/style"
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
	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Fatal(diff)
	}
}

func AssertTrue(t *testing.T, actual interface{}) {
	AssertEq(t, actual, true)
}

func AssertFalse(t *testing.T, actual interface{}) {
	AssertEq(t, actual, false)
}

func AssertUnique(t *testing.T, items ...interface{}) {
	t.Helper()
	itemMap := map[interface{}]interface{}{}
	for _, item := range items {
		itemMap[item] = nil
	}
	if len(itemMap) != len(items) {
		t.Fatalf("Expected items in %v to be unique", items)
	}
}

// Assert the simplistic pointer (or literal value) equality
func AssertSameInstance(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if actual != expected {
		t.Fatalf("Expected %s and %s to be the same instance", actual, expected)
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
		t.Fatalf(
			"Expected '%s' to contain '%s'\n\nDiff:%s",
			actual,
			expected,
			cmp.Diff(expected, actual),
		)
	}
}

func AssertContainsMatch(t *testing.T, actual, exp string) {
	t.Helper()
	if !hasMatches(actual, exp) {
		t.Fatalf("Expected '%s' to match expression '%s'", actual, exp)
	}
}

func AssertNotContainsMatch(t *testing.T, actual, exp string) {
	t.Helper()
	if hasMatches(actual, exp) {
		t.Fatalf("Expected '%s' not to match expression '%s'", actual, exp)
	}
}

func AssertNotContains(t *testing.T, actual, expected string) {
	t.Helper()
	if strings.Contains(actual, expected) {
		t.Fatalf("Expected '%s' to not contain '%s'", actual, expected)
	}
}

func AssertSliceContains(t *testing.T, slice []string, expected ...string) {
	t.Helper()
	_, missing, _ := stringset.Compare(slice, expected)
	if len(missing) > 0 {
		t.Fatalf("Expected %s to contain elements %s", slice, missing)
	}
}

func AssertSliceContainsOnly(t *testing.T, slice []string, expected ...string) {
	t.Helper()
	extra, missing, _ := stringset.Compare(slice, expected)
	if len(missing) > 0 {
		t.Fatalf("Expected %s to contain elements %s", slice, missing)
	}
	if len(extra) > 0 {
		t.Fatalf("Expected %s to not contain elements %s", slice, extra)
	}
}

func AssertMatch(t *testing.T, actual string, expected string) {
	t.Helper()
	if !regexp.MustCompile(expected).MatchString(actual) {
		t.Fatalf("Expected '%s' to match regex '%s'", actual, expected)
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

func hasMatches(actual, exp string) bool {
	regex := regexp.MustCompile(exp)
	matches := regex.FindAll([]byte(actual), -1)
	return len(matches) > 0
}

var dockerCliVal client.CommonAPIClient
var dockerCliOnce sync.Once
var dockerCliErr error

func dockerCli(t *testing.T) client.CommonAPIClient {
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

func CreateImage(t *testing.T, dockerCli client.CommonAPIClient, repoName, dockerFile string) {
	t.Helper()

	buildContext, err := archive.CreateSingleFileTarReader("Dockerfile", dockerFile)
	AssertNil(t, err)

	resp, err := dockerCli.ImageBuild(context.Background(), buildContext, dockertypes.ImageBuildOptions{
		Tags:           []string{repoName},
		SuppressOutput: true,
		Remove:         true,
		ForceRemove:    true,
	})
	AssertNil(t, err)

	err = checkResponse(resp)
	AssertNil(t, errors.Wrapf(err, "building image %s", style.Symbol(repoName)))
}

func CreateImageFromDir(t *testing.T, dockerCli client.CommonAPIClient, repoName string, dir string) {
	t.Helper()

	buildContext := archive.ReadDirAsTar(dir, "/", 0, 0, -1)
	resp, err := dockerCli.ImageBuild(context.Background(), buildContext, dockertypes.ImageBuildOptions{
		Tags:           []string{repoName},
		Remove:         true,
		ForceRemove:    true,
		SuppressOutput: false,
	})
	AssertNil(t, err)

	err = checkResponse(resp)
	AssertNil(t, errors.Wrapf(err, "building image %s", style.Symbol(repoName)))
}

func checkResponse(response dockertypes.ImageBuildResponse) error {
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	messages := strings.Builder{}
	for _, line := range bytes.Split(body, []byte("\n")) {
		if len(line) == 0 {
			continue
		}

		var msg jsonmessage.JSONMessage
		err := json.Unmarshal(line, &msg)
		if err != nil {
			return errors.Wrapf(err, "expected JSON: %s", string(line))
		}

		if msg.Stream != "" {
			messages.WriteString(msg.Stream)
		}

		if msg.Error != nil {
			return errors.WithMessage(msg.Error, messages.String())
		}
	}

	return nil
}

func CreateImageOnRemote(t *testing.T, dockerCli client.CommonAPIClient, registryConfig *TestRegistryConfig, repoName, dockerFile string) string {
	t.Helper()
	imageName := registryConfig.RepoName(repoName)

	defer DockerRmi(dockerCli, imageName)
	CreateImage(t, dockerCli, imageName, dockerFile)
	AssertNil(t, PushImage(dockerCli, imageName, registryConfig))
	return imageName
}

func DockerRmi(dockerCli client.CommonAPIClient, repoNames ...string) error {
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

func PushImage(dockerCli client.CommonAPIClient, ref string, registryConfig *TestRegistryConfig) error {
	rc, err := dockerCli.ImagePush(context.Background(), ref, dockertypes.ImagePushOptions{RegistryAuth: registryConfig.RegistryAuth()})
	if err != nil {
		return err
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

func HTTPGetE(url string, headers map[string]string) (string, error) {
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
	return strings.TrimPrefix(inspect.ID, "sha256:")
}

func Digest(t *testing.T, repoName string) string {
	t.Helper()
	inspect, _, err := dockerCli(t).ImageInspectWithRaw(context.Background(), repoName)
	AssertNil(t, err)
	if len(inspect.RepoDigests) < 1 {
		t.Fatalf("image '%s' has no repo digests", repoName)
	}
	parts := strings.Split(inspect.RepoDigests[0], "@")
	if len(parts) < 2 {
		t.Fatalf("repo digest '%s' malformed", inspect.RepoDigests[0])
	}
	return parts[1]
}

func TopLayerDiffID(t *testing.T, repoName string) string {
	t.Helper()
	inspect, _, err := dockerCli(t).ImageInspectWithRaw(context.Background(), repoName)
	AssertNil(t, err)
	if len(inspect.RootFS.Layers) < 1 {
		t.Fatalf("image '%s' has no layers", repoName)
	}
	return inspect.RootFS.Layers[len(inspect.RootFS.Layers)-1]
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

func PullImageWithAuth(dockerCli client.CommonAPIClient, ref, registryAuth string) error {
	rc, err := dockerCli.ImagePull(context.Background(), ref, dockertypes.ImagePullOptions{RegistryAuth: registryAuth})
	if err != nil {
		return nil
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

func CopyFile(t *testing.T, src, dst string) {
	fi, err := os.Stat(src)
	AssertNil(t, err)

	srcFile, err := os.Open(src)
	AssertNil(t, err)
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fi.Mode())
	AssertNil(t, err)
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	AssertNil(t, err)

	modifiedtime := time.Time{}
	err = os.Chtimes(dst, modifiedtime, modifiedtime)
	AssertNil(t, err)
}

func RecursiveCopy(t *testing.T, src, dst string) {
	t.Helper()
	fis, err := ioutil.ReadDir(src)
	AssertNil(t, err)
	for _, fi := range fis {
		if fi.Mode().IsRegular() {
			CopyFile(t, filepath.Join(src, fi.Name()), filepath.Join(dst, fi.Name()))
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
	t.Helper()
	if expression {
		t.Skip(reason)
	}
}

func RunContainer(ctx context.Context, dockerCli client.CommonAPIClient, id string, stdout io.Writer, stderr io.Writer) error {
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

func CreateTGZ(t *testing.T, srcDir, tarDir string, mode int64) string {
	t.Helper()

	fh, err := ioutil.TempFile("", "*.tgz")
	AssertNil(t, err)
	defer fh.Close()

	gw := gzip.NewWriter(fh)
	defer gw.Close()

	writeTAR(t, srcDir, tarDir, mode, gw)

	return fh.Name()
}

func CreateTAR(t *testing.T, srcDir, tarDir string, mode int64) string {
	t.Helper()

	fh, err := ioutil.TempFile("", "*.tgz")
	AssertNil(t, err)
	defer fh.Close()

	writeTAR(t, srcDir, tarDir, mode, fh)

	return fh.Name()
}

func writeTAR(t *testing.T, srcDir, tarDir string, mode int64, w io.Writer) {
	t.Helper()
	tw := tar.NewWriter(w)
	defer tw.Close()

	err := archive.WriteDirToTar(
		tw,
		srcDir,
		tarDir,
		0, 0, mode,
	)
	AssertNil(t, err)
}

func ListTarContents(tarPath string) ([]tar.Header, error) {
	var (
		tarFile    *os.File
		gzipReader *gzip.Reader
		fhFinal    io.Reader
		err        error
	)

	tarFile, err = os.Open(tarPath)
	fhFinal = tarFile
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open tar '%s'", tarPath)
	}

	defer tarFile.Close()

	if filepath.Ext(tarPath) == ".tgz" {
		gzipReader, err = gzip.NewReader(tarFile)
		fhFinal = gzipReader
		if err != nil {
			return nil, errors.Wrap(err, "failed to create gzip reader")
		}

		defer gzipReader.Close()
	}

	var headers []tar.Header
	tr := tar.NewReader(fhFinal)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "failed to get next tar entry")
		}

		headers = append(headers, *header)
	}

	return headers, nil
}
