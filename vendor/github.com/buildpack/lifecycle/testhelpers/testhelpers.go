package testhelpers

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercli "github.com/docker/docker/client"
	"github.com/google/go-cmp/cmp"

	"github.com/buildpack/lifecycle/archive"
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

func AssertContains(t *testing.T, slice []string, expected string) {
	t.Helper()
	for _, actual := range slice {
		if diff := cmp.Diff(actual, expected); diff == "" {
			return
		}
	}
	t.Fatalf("Expected %+v to contain: %s", slice, expected)

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
	if !strings.Contains(actual.Error(), expected) {
		t.Fatalf(`Expected error to contain "%s", got "%s"`, expected, actual.Error())
	}
}

func AssertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func AssertUidGid(t *testing.T, path string, uid, gid int) {
	fi, err := os.Stat(path)
	AssertNil(t, err)
	stat := fi.Sys().(*syscall.Stat_t)
	AssertEq(t, stat.Uid, uint32(uid))
	AssertEq(t, stat.Gid, uint32(gid))
}

var dockerCliVal *dockercli.Client
var dockerCliOnce sync.Once

func DockerCli(t *testing.T) *dockercli.Client {
	dockerCliOnce.Do(func() {
		var dockerCliErr error
		dockerCliVal, dockerCliErr = dockercli.NewClientWithOpts(dockercli.FromEnv, dockercli.WithVersion("1.38"))
		AssertNil(t, dockerCliErr)
	})
	return dockerCliVal
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

var getBuildImageOnce sync.Once

func CreateImageOnLocal(t *testing.T, dockerCli *dockercli.Client, repoName, dockerFile string, labels map[string]string) {
	ctx := context.Background()

	buildContext, err := CreateSingleFileTar("Dockerfile", dockerFile)
	AssertNil(t, err)

	res, err := dockerCli.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:           []string{repoName},
		SuppressOutput: true,
		Remove:         true,
		ForceRemove:    true,
		Labels:         labels,
	})
	AssertNil(t, err)

	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()
}

func CreateImageOnRemote(t *testing.T, dockerCli *dockercli.Client, repoName, dockerFile string, labels map[string]string) string {
	t.Helper()
	defer DockerRmi(dockerCli, repoName)

	CreateImageOnLocal(t, dockerCli, repoName, dockerFile, labels)

	var topLayer string
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), repoName)
	AssertNil(t, err)
	if len(inspect.RootFS.Layers) > 0 {
		topLayer = inspect.RootFS.Layers[len(inspect.RootFS.Layers)-1]
	} else {
		topLayer = "N/A"
	}

	AssertNil(t, pushImage(dockerCli, repoName))

	return topLayer
}

func PullImage(dockerCli *dockercli.Client, ref string) error {
	rc, err := dockerCli.ImagePull(context.Background(), ref, dockertypes.ImagePullOptions{})
	if err != nil {
		// Retry
		rc, err = dockerCli.ImagePull(context.Background(), ref, dockertypes.ImagePullOptions{})
		if err != nil {
			return err
		}
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

func DockerRmi(dockerCli *dockercli.Client, repoNames ...string) error {
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

func CopySingleFileFromContainer(dockerCli *dockercli.Client, ctrID, path string) (string, error) {
	r, _, err := dockerCli.CopyFromContainer(context.Background(), ctrID, path)
	if err != nil {
		return "", err
	}
	defer r.Close()
	tr := tar.NewReader(r)
	hdr, err := tr.Next()
	if hdr.Name != path && hdr.Name != filepath.Base(path) {
		return "", fmt.Errorf("filenames did not match: %s and %s (%s)", hdr.Name, path, filepath.Base(path))
	}
	b, err := ioutil.ReadAll(tr)
	return string(b), err
}

func CopySingleFileFromImage(dockerCli *dockercli.Client, repoName, path string) (string, error) {
	ctr, err := dockerCli.ContainerCreate(context.Background(),
		&container.Config{
			Image: repoName,
		}, &container.HostConfig{
			AutoRemove: true,
		}, nil, "",
	)
	if err != nil {
		return "", err
	}
	defer dockerCli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{})
	return CopySingleFileFromContainer(dockerCli, ctr.ID, path)
}

func pushImage(dockerCli *dockercli.Client, ref string) error {
	rc, err := dockerCli.ImagePush(context.Background(), ref, dockertypes.ImagePushOptions{RegistryAuth: "{}"})
	if err != nil {
		return err
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

func packTag() string {
	tag := os.Getenv("PACK_TAG")
	if tag == "" {
		return "latest"
	}
	return tag
}

func HttpGetE(url string) (string, error) {
	resp, err := http.DefaultClient.Get(url)
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

func ImageID(t *testing.T, repoName string) string {
	t.Helper()
	inspect, _, err := DockerCli(t).ImageInspectWithRaw(context.Background(), repoName)
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Failed to execute command: %v, %s, %s, %s", cmd.Args, err, stderr.String(), output)
	}

	return string(output), nil
}

func ComputeSHA256ForFile(t *testing.T, path string) string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open file: %s", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		t.Fatalf("failed to copy file to hasher: %s", err)
	}

	return hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
}

func ComputeSHA256ForPath(t *testing.T, path string, uid int, guid int) string {
	hasher := sha256.New()
	err := archive.WriteTarArchive(hasher, path, uid, guid)
	AssertNil(t, err)
	layer5sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
	return layer5sha
}

func RecursiveCopy(t *testing.T, src, dst string) {
	t.Helper()
	fis, err := ioutil.ReadDir(src)
	AssertNil(t, err)
	for _, fi := range fis {
		if fi.Mode().IsRegular() {
			srcFile, err := os.Open(filepath.Join(src, fi.Name()))
			AssertNil(t, err)
			dstFile, err := os.Create(filepath.Join(dst, fi.Name()))
			AssertNil(t, err)
			_, err = io.Copy(dstFile, srcFile)
			AssertNil(t, err)
			modifiedtime := time.Time{}
			err = os.Chtimes(filepath.Join(dst, fi.Name()), modifiedtime, modifiedtime)
			AssertNil(t, err)
			err = os.Chmod(filepath.Join(dst, fi.Name()), 0664)
			AssertNil(t, err)
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

func CreateSingleFileTar(path, txt string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: path, Size: int64(len(txt)), Mode: 0644}); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(txt)); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return bytes.NewReader(buf.Bytes()), nil
}
