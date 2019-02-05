package testhelpers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/buildpack/pack"

	"github.com/buildpack/lifecycle/fs"
	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

var registryContainerName = "registry:2"

type TestRegistryConfig struct {
	runRegistryName string
	RunRegistryPort string
	DockerConfigDir string
	username        string
	password        string
}

func RunRegistry(t *testing.T, seedRegistry bool) *TestRegistryConfig {
	t.Log("run registry")
	t.Helper()

	runRegistryName := "test-registry-" + RandString(10)
	username := RandString(10)
	password := RandString(10)

	runRegistryPort := startRegistry(t, runRegistryName, username, password)

	Eventually(t, func() bool {
		_, err := dockerCli(t).RegistryLogin(context.Background(), dockertypes.AuthConfig{
			Username:      username,
			Password:      password,
			ServerAddress: fmt.Sprintf("localhost:%s", runRegistryPort)})
		return err == nil
	}, 100*time.Millisecond, 10*time.Second)

	dockerConfigDir := setupDockerConfigWithAuth(t, username, password, runRegistryPort)

	registryConfig := &TestRegistryConfig{
		runRegistryName: runRegistryName,
		RunRegistryPort: runRegistryPort,
		DockerConfigDir: dockerConfigDir,
		username:        username,
		password:        password,
	}

	if seedRegistry {
		t.Log("seed registry")
		for _, f := range []func(*testing.T, string) string{DefaultBuildImage, DefaultRunImage, DefaultBuilderImage} {
			AssertNil(t, PushImage(dockerCli(t), f(t, runRegistryPort), registryConfig))
			a := f(t, runRegistryPort)
			fmt.Println(a)
		}
	}

	return registryConfig
}

func startRegistry(t *testing.T, runRegistryName, username, password string) string {
	AssertNil(t, PullImage(dockerCli(t), registryContainerName))
	ctx := context.Background()

	htpasswdTar := generateHtpasswd(t, ctx, username, password)

	ctr, err := dockerCli(t).ContainerCreate(ctx, &dockercontainer.Config{
		Image:  registryContainerName,
		Labels: map[string]string{"author": "pack"},
		Env: []string{
			"REGISTRY_AUTH=htpasswd",
			"REGISTRY_AUTH_HTPASSWD_REALM=Registry Realm",
			"REGISTRY_AUTH_HTPASSWD_PATH=/registry_test_htpasswd",
		},
	}, &dockercontainer.HostConfig{
		AutoRemove: true,
		PortBindings: nat.PortMap{
			"5000/tcp": []nat.PortBinding{{}},
		},
	}, nil, runRegistryName)
	AssertNil(t, err)
	err = dockerCli(t).CopyToContainer(ctx, ctr.ID, "/", htpasswdTar, dockertypes.CopyToContainerOptions{})
	AssertNil(t, err)

	err = dockerCli(t).ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
	AssertNil(t, err)
	inspect, err := dockerCli(t).ContainerInspect(context.TODO(), ctr.ID)
	AssertNil(t, err)
	runRegistryPort := inspect.NetworkSettings.Ports["5000/tcp"][0].HostPort

	if os.Getenv("DOCKER_HOST") != "" {
		err := proxyDockerHostPort(dockerCli(t), runRegistryPort)
		AssertNil(t, err)
	}

	return runRegistryPort
}

func generateHtpasswd(t *testing.T, ctx context.Context, username string, password string) io.Reader {
	//https://docs.docker.com/registry/deploying/#restricting-access
	htpasswdCtr, err := dockerCli(t).ContainerCreate(ctx, &dockercontainer.Config{
		Image:      registryContainerName,
		Entrypoint: []string{"htpasswd", "-Bbn", username, password},
	}, &dockercontainer.HostConfig{
		AutoRemove: true,
	}, nil, "")
	AssertNil(t, err)

	var b bytes.Buffer
	err = dockerCli(t).RunContainer(ctx, htpasswdCtr.ID, &b, &b)
	reader, err := (&fs.FS{}).CreateSingleFileTar("/registry_test_htpasswd", b.String())
	AssertNil(t, err)

	return reader
}

func setupDockerConfigWithAuth(t *testing.T, username string, password string, runRegistryPort string) string {
	dockerConfigDir, err := ioutil.TempDir("", "pack.test.docker.config.dir")
	AssertNil(t, err)

	AssertNil(t, ioutil.WriteFile(filepath.Join(dockerConfigDir, "config.json"), []byte(fmt.Sprintf(`{
			  "auths": {
			    "localhost:%s": {
			      "auth": "%s"
			    }
			  }
			}
			`, runRegistryPort, encodedUserPass(username, password))), 0666))
	return dockerConfigDir
}

func encodedUserPass(username string, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))
}

func (rc *TestRegistryConfig) StopRegistry(t *testing.T) {
	t.Log("stop registry")
	t.Helper()
	err := dockerCli(t).ContainerKill(context.Background(), rc.runRegistryName, "SIGKILL")
	AssertNil(t, err)

	err = os.RemoveAll(rc.DockerConfigDir)
	AssertNil(t, err)
}

func (rc *TestRegistryConfig) RepoName(name string) string {
	return "localhost:" + rc.RunRegistryPort + "/" + name
}

func (rc *TestRegistryConfig) RegistryAuth() string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(`{"username":"%s","password":"%s"}`, rc.username, rc.password)))
}

func (rc *TestRegistryConfig) RegistryCatalog() (string, error) {
	return HttpGetE(fmt.Sprintf("http://localhost:%s/v2/_catalog", rc.RunRegistryPort), map[string]string{
		"Authorization": "Basic " + encodedUserPass(rc.username, rc.password),
	})
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
					LABEL %s="{\"runImage\": {\"image\": \"%s\"}}"
				`, origName, pack.BuilderMetadataLabel, runImageName))
		}
	})
	return newName
}
