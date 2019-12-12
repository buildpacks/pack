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
	"testing"
	"time"

	"github.com/buildpacks/pack/internal/archive"

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

func RunRegistry(t *testing.T) *TestRegistryConfig {
	t.Log("run registry")
	t.Helper()

	runRegistryName := "test-registry-" + RandString(10)
	username := RandString(10)
	password := RandString(10)

	runRegistryPort := startRegistry(t, runRegistryName, username, password)
	dockerConfigDir := setupDockerConfigWithAuth(t, username, password, runRegistryPort)

	registryConfig := &TestRegistryConfig{
		runRegistryName: runRegistryName,
		RunRegistryPort: runRegistryPort,
		DockerConfigDir: dockerConfigDir,
		username:        username,
		password:        password,
	}

	return registryConfig
}

func (rc *TestRegistryConfig) AuthConfig() dockertypes.AuthConfig {
	return dockertypes.AuthConfig{
		Username:      rc.username,
		Password:      rc.password,
		ServerAddress: fmt.Sprintf("localhost:%s", rc.RunRegistryPort)}
}

func (rc *TestRegistryConfig) Login(t *testing.T, username string, password string) {
	Eventually(t, func() bool {
		_, err := dockerCli(t).RegistryLogin(context.Background(), dockertypes.AuthConfig{
			Username:      username,
			Password:      password,
			ServerAddress: fmt.Sprintf("localhost:%s", rc.RunRegistryPort)})
		return err == nil
	}, 100*time.Millisecond, 10*time.Second)
}

func startRegistry(t *testing.T, runRegistryName, username, password string) string {
	AssertNil(t, PullImageWithAuth(dockerCli(t), registryContainerName, ""))
	ctx := context.Background()

	htpasswdTar := generateHtpasswd(ctx, t, username, password)

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
		err := proxyDockerHostPort(runRegistryPort)
		AssertNil(t, err)
	}

	return runRegistryPort
}

func generateHtpasswd(ctx context.Context, t *testing.T, username string, password string) io.Reader {
	//https://docs.docker.com/registry/deploying/#restricting-access
	htpasswdCtr, err := dockerCli(t).ContainerCreate(ctx, &dockercontainer.Config{
		Image:      registryContainerName,
		Entrypoint: []string{"htpasswd", "-Bbn", username, password},
	}, &dockercontainer.HostConfig{
		AutoRemove: true,
	}, nil, "")
	AssertNil(t, err)

	var b bytes.Buffer
	err = RunContainer(ctx, dockerCli(t), htpasswdCtr.ID, &b, &b)
	AssertNil(t, err)
	reader, err := archive.CreateSingleFileTarReader("/registry_test_htpasswd", b.String())
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
	return HTTPGetE(fmt.Sprintf("http://localhost:%s/v2/_catalog", rc.RunRegistryPort), map[string]string{
		"Authorization": "Basic " + encodedUserPass(rc.username, rc.password),
	})
}
