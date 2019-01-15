package testhelpers

import (
	"archive/tar"
	"bytes"
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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buildpack/pack"

	"github.com/dgodd/dockerdial"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-cmp/cmp"

	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
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
	if actual.Error() != expected {
		t.Fatalf(`Expected error to equal "%s", got "%s"`, expected, actual.Error())
	}
}

func AssertContains(t *testing.T, actual, expected string) {
	t.Helper()
	if !strings.Contains(actual, expected) {
		t.Fatalf("Expected: '%s' to contain '%s'", actual, expected)
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

var dockerCliVal *docker.Client
var dockerCliOnce sync.Once
var dockerCliErr error

func dockerCli(t *testing.T) *docker.Client {
	dockerCliOnce.Do(func() {
		dockerCliVal, dockerCliErr = docker.New()
	})
	AssertNil(t, dockerCliErr)
	return dockerCliVal
}

func proxyDockerHostPort(dockerCli *docker.Client, port string) error {
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

		AssertNil(t, PullImage(dockerCli(t), "registry:2"))
		ctx := context.Background()
		ctr, err := dockerCli(t).ContainerCreate(ctx, &container.Config{
			Image:  "registry:2",
			Labels: map[string]string{"author": "pack"},
		}, &container.HostConfig{
			AutoRemove: true,
			PortBindings: nat.PortMap{
				"5000/tcp": []nat.PortBinding{{}},
			},
		}, nil, runRegistryName)
		AssertNil(t, err)
		defer dockerCli(t).ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})
		err = dockerCli(t).ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
		AssertNil(t, err)

		inspect, err := dockerCli(t).ContainerInspect(context.TODO(), ctr.ID)
		AssertNil(t, err)
		runRegistryPort = inspect.NetworkSettings.Ports["5000/tcp"][0].HostPort

		if os.Getenv("DOCKER_HOST") != "" {
			err := proxyDockerHostPort(dockerCli(t), runRegistryPort)
			AssertNil(t, err)
		}

		Eventually(t, func() bool {
			txt, err := HttpGetE(fmt.Sprintf("http://localhost:%s/v2/", runRegistryPort))
			return err == nil && txt != ""
		}, 100*time.Millisecond, 10*time.Second)

		if seedRegistry {
			t.Log("seed registry")
			for _, f := range []func(*testing.T, string) string{DefaultBuildImage, DefaultRunImage, DefaultBuilderImage} {
				AssertNil(t, pushImage(dockerCli(t), f(t, runRegistryPort)))
			}
		}
	})
	return runRegistryPort
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

func ConfigurePackHome(t *testing.T, packHome, registryPort string) {
	t.Helper()
	AssertNil(t, ioutil.WriteFile(filepath.Join(packHome, "config.toml"), []byte(fmt.Sprintf(`
				default-stack-id = "io.buildpacks.stacks.bionic"
                default-builder = "%s"

				[[stacks]]
				  id = "io.buildpacks.stacks.bionic"
				  build-image = "%s"
				  run-images = ["%s"]
			`, DefaultBuilderImage(t, registryPort), DefaultBuildImage(t, registryPort), DefaultRunImage(t, registryPort))), 0666))
}

func StopRegistry(t *testing.T) {
	t.Log("stop registry")
	t.Helper()
	if runRegistryName != "" {
		dockerCli(t).ContainerKill(context.Background(), runRegistryName, "SIGKILL")
		dockerCli(t).ContainerRemove(context.TODO(), runRegistryName, dockertypes.ContainerRemoveOptions{Force: true})
	}
}

var getBuildImageOnce sync.Once

func DefaultBuildImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	getBuildImageOnce.Do(func() {
		if tag == defaultTag {
			AssertNil(t, PullImage(dockerCli(t), fmt.Sprintf("packs/build:%s", tag)))
		}
		AssertNil(t, dockerCli(t).ImageTag(
			context.Background(),
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
		if tag == defaultTag {
			AssertNil(t, PullImage(dockerCli(t), fmt.Sprintf("packs/run:%s", tag)))
		}
		AssertNil(t, dockerCli(t).ImageTag(
			context.Background(),
			fmt.Sprintf("packs/run:%s", tag),
			fmt.Sprintf("localhost:%s/packs/run:%s", registryPort, tag),
		))
	})
	return fmt.Sprintf("localhost:%s/packs/run:%s", registryPort, tag)
}

var getBuilderImageOnce sync.Once

func DefaultBuilderImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	origName := fmt.Sprintf("packs/samples:%s", tag)
	newName := fmt.Sprintf("localhost:%s/%s", registryPort, origName)
	dockerCli := dockerCli(t)
	getBuilderImageOnce.Do(func() {
		if tag == defaultTag {
			AssertNil(t, PullImage(dockerCli, origName))
			AssertNil(t, dockerCli.ImageTag(context.Background(), origName, newName))
		} else {
			runImageName := DefaultRunImage(t, registryPort)

			CreateImageOnLocal(t, dockerCli, newName, fmt.Sprintf(`
					FROM %s
					LABEL %s="{\"runImages\": [\"%s\"]}"
				`, origName, pack.MetadataLabel, runImageName))
		}
	})
	return newName
}

func CreateImageOnLocal(t *testing.T, dockerCli *docker.Client, repoName, dockerFile string) {
	ctx := context.Background()

	buildContext, err := (&fs.FS{}).CreateSingleFileTar("Dockerfile", dockerFile)
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

func CreateImageOnRemote(t *testing.T, dockerCli *docker.Client, registryPort, repoName, dockerFile string) string {
	t.Helper()
	imageName := fmt.Sprintf("localhost:%s/%s", registryPort, repoName)
	defer DockerRmi(dockerCli, imageName)
	CreateImageOnLocal(t, dockerCli, imageName, dockerFile)
	AssertNil(t, pushImage(dockerCli, imageName))
	return imageName
}

func DockerRmi(dockerCli *docker.Client, repoNames ...string) error {
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

func CopySingleFileFromContainer(dockerCli *docker.Client, ctrID, path string) (string, error) {
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

func StatSingleFileFromContainer(dockerCli *docker.Client, ctrID, path string) (*tar.Header, error) {
	r, _, err := dockerCli.CopyFromContainer(context.Background(), ctrID, path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	tr := tar.NewReader(r)
	hdr, err := tr.Next()
	if hdr.Name != path && hdr.Name != filepath.Base(path) {
		return nil, fmt.Errorf("filenames did not match: %s and %s (%s)", hdr.Name, path, filepath.Base(path))
	}
	return hdr, err
}

func CopySingleFileFromImage(dockerCli *docker.Client, repoName, path string) (string, error) {
	ctr, err := dockerCli.ContainerCreate(context.Background(),
		&container.Config{
			Image:  repoName,
			Labels: map[string]string{"author": "pack"},
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

func pushImage(dockerCli *docker.Client, ref string) error {
	rc, err := dockerCli.ImagePush(context.Background(), ref, dockertypes.ImagePushOptions{RegistryAuth: "{}"})
	if err != nil {
		return err
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

const defaultTag = "v3alpha2"

func packTag() string {
	tag := os.Getenv("PACK_TAG")
	if tag == "" {
		return defaultTag
	}
	return tag
}

var pullPacksSamplesOnce sync.Once

func pullPacksSamples(d *docker.Client) {
	pullPacksSamplesOnce.Do(func() {
		PullImage(d, "packs/samples")
	})
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

	ctx := context.Background()
	pullPacksSamples(dockerCli(t))
	ctr, err := dockerCli(t).ContainerCreate(ctx, &container.Config{
		User:   "pack",
		Image:  "packs/samples",
		Cmd:    []string{"true"},
		Labels: map[string]string{"author": "pack"},
	}, &container.HostConfig{
		AutoRemove: true,
		Binds:      []string{destVolume + ":/workspace"},
	}, nil, "")
	AssertNil(t, err)
	defer dockerCli(t).ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	tr, errChan := (&fs.FS{}).CreateTarReader(srcPath, "/workspace", 1000, 1000)
	err = dockerCli(t).CopyToContainer(ctx, ctr.ID, "/", tr, dockertypes.CopyToContainerOptions{})
	AssertNil(t, err)
	AssertNil(t, <-errChan)
}

func ReadFromDocker(t *testing.T, volume, path string) string {
	t.Helper()
	pullPacksSamples(dockerCli(t))
	ctr, err := dockerCli(t).ContainerCreate(
		context.Background(),
		&container.Config{
			Image:  "packs/samples",
			Labels: map[string]string{"author": "pack"},
		},
		&container.HostConfig{
			AutoRemove: true,
			Binds:      []string{volume + ":/workspace"},
		},
		nil, "",
	)
	AssertNil(t, err)
	defer dockerCli(t).ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{})
	txt, err := CopySingleFileFromContainer(dockerCli(t), ctr.ID, path)
	AssertNil(t, err)
	return txt
}

func StatFromDocker(t *testing.T, volume, path string) *tar.Header {
	t.Helper()
	pullPacksSamples(dockerCli(t))
	ctr, err := dockerCli(t).ContainerCreate(
		context.Background(),
		&container.Config{
			Image:  "packs/samples",
			Labels: map[string]string{"author": "pack"},
		},
		&container.HostConfig{
			AutoRemove: true,
			Binds:      []string{volume + ":/workspace"},
		},
		nil, "",
	)
	AssertNil(t, err)
	defer dockerCli(t).ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{})
	hdr, err := StatSingleFileFromContainer(dockerCli(t), ctr.ID, path)
	AssertNil(t, err)
	return hdr
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

func CleanDefaultImages(t *testing.T, registryPort string) {
	t.Helper()
	AssertNil(t,
		DockerRmi(
			dockerCli(t),
			DefaultRunImage(t, registryPort),
			DefaultBuildImage(t, registryPort),
			DefaultBuilderImage(t, registryPort),
		),
	)
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

func PullImage(dockerCli *docker.Client, ref string) error {
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
