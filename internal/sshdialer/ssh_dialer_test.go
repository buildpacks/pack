package sshdialer_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/homedir"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/buildpacks/pack/internal/sshdialer"
	th "github.com/buildpacks/pack/testhelpers"
)

const (
	imageName     = "buildpacks/sshdialer-test-img"
	containerName = "sshdialer-test-ctr"
	sshPort       = "22/tcp"
)

func TestCreateDialer(t *testing.T) {
	for _, privateKey := range []string{"id_ed25519", "id_rsa", "id_dsa"} {
		path := filepath.Join("testdata", privateKey)
		fixupPrivateKeyMod(path)
	}

	defer withoutSSHAgent(t)()
	defer withCleanHome(t)()

	connConfig, cleanUp, err := prepareSSHServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanUp()

	type args struct {
		connStr          string
		credentialConfig sshdialer.Config
	}
	type testParams struct {
		name        string
		args        args
		setUpEnv    setUpEnvFn
		skipOnWin   bool
		CreateError string
		DialError   string
	}
	tests := []testParams{
		{
			name: "read password from input",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{PasswordCallback: func() (string, error) {
					return "idkfa", nil
				}},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
		},
		{
			name: "password in url",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
		},
		{
			name: "server key is not in known_hosts (the file doesn't exists)",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome),
			CreateError: sshdialer.ErrKeyUnknownMsg,
		},
		{
			name: "server key is not in known_hosts (the file exists)",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withEmptyKnownHosts),
			CreateError: sshdialer.ErrKeyUnknownMsg,
		},
		{
			name: "server key is not in known_hosts (the filed doesn't exists) - user force trust",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{HostKeyCallback: func(hostPort string, pubKey ssh.PublicKey) error {
					return nil
				}},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome),
		},
		{
			name: "server key is not in known_hosts (the file exists) - user force trust",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{HostKeyCallback: func(hostPort string, pubKey ssh.PublicKey) error {
					return nil
				}},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withEmptyKnownHosts),
		},
		{
			name: "server key does not match the respective key in known_host",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withBadKnownHosts(connConfig)),
			CreateError: sshdialer.ErrKeyMismatchMsg,
		},
		{
			name: "key from identity parameter",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{Identity: filepath.Join("testdata", "id_ed25519")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
		},
		{
			name: "key at standard location with need to read passphrase",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{PassPhraseCallback: func() (string, error) {
					return "idfa", nil
				}},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKey(t, "id_rsa"), withKnowHosts(connConfig)),
		},
		{
			name: "key at standard location with explicitly set passphrase",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{PassPhrase: "idfa"},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKey(t, "id_rsa"), withKnowHosts(connConfig)),
		},
		{
			name: "key at standard location with no passphrase",
			args: args{connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKey(t, "id_ed25519"), withKnowHosts(connConfig)),
		},
		{
			name: "key from ssh-agent",
			args: args{connStr: fmt.Sprintf("ssh://testuser@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv: all(withGoodSSHAgent, withCleanHome, withKnowHosts(connConfig)),
		},
		{
			name: "password in url with IPv6",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@[%s]:%d/home/testuser/test.sock",
				connConfig.hostIPv6,
				connConfig.portIPv6,
			)},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
		},
		{
			name: "broken known host",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withBrokenKnownHosts),
			CreateError: "missing host pattern",
		},
		{
			name: "inaccessible known host",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withInaccessibleKnownHosts),
			skipOnWin:   true,
			CreateError: "permission denied",
		},
		{
			name: "failing pass phrase cbk",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{PassPhraseCallback: func() (string, error) {
					return "", errors.New("test_error_msg")
				}},
			},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withKey(t, "id_rsa"), withKnowHosts(connConfig)),
			CreateError: "test_error_msg",
		},
		{
			name: "with broken key at default location",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withKey(t, "id_dsa"), withKnowHosts(connConfig)),
			CreateError: "failed to parse private key",
		},
		{
			name: "with broken key explicit",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{Identity: filepath.Join("testdata", "id_dsa")},
			},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
			CreateError: "failed to parse private key",
		},
		{
			name: "with inaccessible key",
			args: args{connStr: fmt.Sprintf("ssh://testuser:idkfa@%s:%d/home/testuser/test.sock",
				connConfig.hostIPv4,
				connConfig.portIPv4,
			)},
			setUpEnv:    all(withoutSSHAgent, withCleanHome, withInaccessibleKey("id_rsa"), withKnowHosts(connConfig)),
			skipOnWin:   true,
			CreateError: "failed to read key file",
		},
		{
			name: "socket doesn't exist in remote",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/does/not/exist/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{PasswordCallback: func() (string, error) {
					return "idkfa", nil
				}},
			},
			setUpEnv:  all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig)),
			DialError: "failed to dial unix socket in the remote",
		},
		{
			name: "ssh agent non-existent socket",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/does/not/exist/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
			},
			setUpEnv:    all(withBadSSHAgentSocket, withCleanHome, withKnowHosts(connConfig)),
			CreateError: "failed to connect to ssh-agent's socket",
		},
		{
			name: "bad ssh agent",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d/does/not/exist/test.sock",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
			},
			setUpEnv:    all(withBadSSHAgent, withCleanHome, withKnowHosts(connConfig)),
			CreateError: "failed to get signers from ssh-agent",
		},
		{
			name: "use docker host from remote unix",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{Identity: filepath.Join("testdata", "id_ed25519")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig),
				withRemoteDockerHost("unix:///home/testuser/test.sock", connConfig)),
		},
		{
			name: "use docker host from remote tcp",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{Identity: filepath.Join("testdata", "id_ed25519")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig),
				withRemoteDockerHost("tcp://localhost:1234", connConfig)),
		},
		{
			name: "use docker host from remote fd",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{Identity: filepath.Join("testdata", "id_ed25519")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig),
				withRemoteDockerHost("fd://localhost:1234", connConfig)),
		},
		{
			name: "use docker host from remote npipe",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{Identity: filepath.Join("testdata", "id_ed25519")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig),
				withRemoteDockerHost("npipe:////./pipe/docker_engine", connConfig)),
			CreateError: "not supported",
		},
		{
			name: "use emulated windows with default docker host",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{Identity: filepath.Join("testdata", "id_ed25519")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig),
				withEmulatingWindows(connConfig)),
			CreateError: "not supported",
		},
		{
			name: "use emulated windows with tcp docker host",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{Identity: filepath.Join("testdata", "id_ed25519")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig), withEmulatingWindows(connConfig),
				withRemoteDockerHost("tcp://localhost:1234", connConfig)),
		},
		{
			name: "use docker system dial-stdio",
			args: args{
				connStr: fmt.Sprintf("ssh://testuser@%s:%d",
					connConfig.hostIPv4,
					connConfig.portIPv4,
				),
				credentialConfig: sshdialer.Config{Identity: filepath.Join("testdata", "id_ed25519")},
			},
			setUpEnv: all(withoutSSHAgent, withCleanHome, withKnowHosts(connConfig), withEmulatedDockerSystemDialStdio(connConfig), withFixedUpSSHCLI),
		},
	}

	for _, ttx := range tests {
		tt := ttx
		t.Run(tt.name, func(t *testing.T) {
			// this test cannot be parallelized as they use process wide environment variable $HOME

			u, err := url.Parse(tt.args.connStr)
			if err != nil {
				t.Fatal(err)
			}

			if net.ParseIP(u.Hostname()).To4() == nil && connConfig.hostIPv6 == "" {
				t.Skip("skipping ipv6 test since test environment doesn't support ipv6 connection")
			}

			if tt.skipOnWin && runtime.GOOS == "windows" {
				t.Skip("skipping this test on windows")
			}

			defer tt.setUpEnv(t)()

			dialContext, err := sshdialer.NewDialContext(u, tt.args.credentialConfig)

			if tt.CreateError == "" {
				th.AssertEq(t, err, nil)
			} else {
				// I wish I could use errors.Is(),
				// however foreign code is not wrapping errors thoroughly
				if err != nil {
					th.AssertContains(t, err.Error(), tt.CreateError)
				} else {
					t.Error("expected error but got nil")
				}
			}
			if err != nil {
				return
			}

			transport := http.Transport{DialContext: dialContext}
			httpClient := http.Client{Transport: &transport}
			resp, err := httpClient.Get("http://docker/")
			if tt.DialError == "" {
				th.AssertEq(t, err, nil)
			} else {
				// I wish I could use errors.Is(),
				// however foreign code is not wrapping errors thoroughly
				if err != nil {
					th.AssertContains(t, err.Error(), tt.CreateError)
				} else {
					t.Error("expected error but got nil")
				}
			}
			if err != nil {
				return
			}
			defer resp.Body.Close()

			b, err := ioutil.ReadAll(resp.Body)
			th.AssertTrue(t, err == nil)
			if err != nil {
				return
			}
			th.AssertEq(t, string(b), "Hello there!")
		})
	}
}

type ConnectionConfig struct {
	hostIPv4 string
	hostIPv6 string
	portIPv4 int
	portIPv6 int
}

// We need to set up the test container running sshd against which we will run tests.
// This will return IPv4 and IPv6 of the container,
// cleanUp procedure to remove the test container and possibly error.
func prepareSSHServer(t *testing.T) (connConfig *ConnectionConfig, cleanUp func(), err error) {
	th.RequireDocker(t)

	var cleanUps []func()
	cleanUp = func() {
		for i := range cleanUps {
			cleanUps[i]()
		}
	}

	ctx := context.Background()

	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return
	}

	info, err := cli.Info(ctx)
	th.SkipIf(t, info.OSType == "windows", "These tests are not yet compatible with Windows-based containers")

	wd, err := os.Getwd()
	if err != nil {
		return
	}

	th.CreateImageFromDir(t, cli, imageName, filepath.Join(wd, "testdata"))

	config := container.Config{
		Image: imageName,
	}

	connConfig = &ConnectionConfig{
		hostIPv4: "127.0.0.1",
		hostIPv6: "",
		portIPv4: 0,
		portIPv6: 0,
	}

	portBindings := []nat.PortBinding{
		{HostIP: connConfig.hostIPv4},
	}

	// docker desktop doesn't support ipv6 port bindings
	// see https://github.com/docker/for-win/issues/8211
	// and https://github.com/docker/for-mac/issues/1432
	if runtime.GOOS == "linux" {
		connConfig.hostIPv6 = "::1"
		portBindings = append(portBindings, nat.PortBinding{HostIP: connConfig.hostIPv6})
	}

	hostConfig := &container.HostConfig{
		PortBindings: map[nat.Port][]nat.PortBinding{sshPort: portBindings},
	}

	// just in case the container has not been cleaned up
	_ = cli.ContainerRemove(ctx, containerName, types.ContainerRemoveOptions{Force: true})

	ctr, err := cli.ContainerCreate(ctx, &config, hostConfig, nil, nil, containerName)
	if err != nil {
		return
	}

	defer func() {
		f := func() { cli.ContainerRemove(ctx, ctr.ID, types.ContainerRemoveOptions{Force: true}) }
		if err != nil {
			f()
		} else {
			cleanUps = append(cleanUps, f)
		}
	}()

	ctrStartOpts := types.ContainerStartOptions{}
	err = cli.ContainerStart(ctx, ctr.ID, ctrStartOpts)
	if err != nil {
		return
	}

	defer func() {
		f := func() { cli.ContainerKill(ctx, ctr.ID, "SIGKILL") }
		if err != nil {
			f()
		} else {
			cleanUps = append(cleanUps, f)
		}
	}()

	var ctrJSON types.ContainerJSON
	ctrJSON, err = cli.ContainerInspect(ctx, ctr.ID)
	if err != nil {
		return
	}

	sshPortBinds := ctrJSON.NetworkSettings.Ports[sshPort]

	var found bool
	connConfig.portIPv4, found = portForHost(sshPortBinds, connConfig.hostIPv4)
	if !found {
		err = errors.Errorf("SSH port for %s not found in %+v", connConfig.hostIPv4, sshPortBinds)
		return
	}

	if connConfig.hostIPv6 != "" {
		connConfig.portIPv6, found = portForHost(sshPortBinds, connConfig.hostIPv6)
		if !found {
			err = errors.Errorf("SSH port for %s not found in %+v", connConfig.hostIPv6, sshPortBinds)
			return
		}
	}

	// wait for ssh container to start serving ssh
	// overall timeout before giving up on connecting
	timeout := time.After(20 * time.Second)
	// wait this amount between retries
	waitTicker := time.Tick(2 * time.Second)
	for {
		select {
		case <-timeout:
			err = fmt.Errorf("test container failed to start serving ssh")
			return
		case <-waitTicker:
		}

		t.Logf("connecting to ssh: %s:%d", connConfig.hostIPv4, connConfig.portIPv4)
		conn, err := net.Dial("tcp", net.JoinHostPort(connConfig.hostIPv4, strconv.Itoa(connConfig.portIPv4)))
		if err != nil {
			continue
		}
		conn.Close()

		break
	}

	return connConfig, cleanUp, err
}

func portForHost(bindings []nat.PortBinding, host string) (int, bool) {
	for _, pb := range bindings {
		if pb.HostIP == host {
			if port, err := strconv.Atoi(pb.HostPort); err == nil {
				return port, true
			}
		}
	}

	return 0, false
}

// function that prepares testing environment and returns clean up function
// this should be used in conjunction with defer: `defer fn()()`
// e.g. sets environment variables or starts mock up services
// it returns clean up procedure that restores old values of environment variables
// or shuts down mock up services
type setUpEnvFn func(t *testing.T) func()

// combines multiple setUp routines into one setUp routine
func all(fns ...setUpEnvFn) setUpEnvFn {
	return func(t *testing.T) func() {
		t.Helper()
		var cleanUps []func()
		for _, fn := range fns {
			cleanUps = append(cleanUps, fn(t))
		}

		return func() {
			for i := len(cleanUps) - 1; i >= 0; i-- {
				cleanUps[i]()
			}
		}
	}
}

func cp(src, dest string) error {
	srcFs, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("the cp() function failed to stat source file: %w", err)
	}

	data, err := ioutil.ReadFile(src)
	if err != nil {
		return fmt.Errorf("the cp() function failed to read source file: %w", err)
	}

	_, err = os.Stat(dest)
	if err == nil {
		return fmt.Errorf("destination file already exists: %w", os.ErrExist)
	}

	return ioutil.WriteFile(dest, data, srcFs.Mode())
}

// puts key from ./testdata/{keyName} to $HOME/.ssh/{keyName}
// those keys are authorized by the testing ssh server
func withKey(t *testing.T, keyName string) setUpEnvFn {
	t.Helper()

	return func(t *testing.T) func() {
		t.Helper()
		var err error

		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}

		err = os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
		if err != nil {
			t.Fatal(err)
		}

		keySrc := filepath.Join("testdata", keyName)
		keyDest := filepath.Join(home, ".ssh", keyName)
		err = cp(keySrc, keyDest)
		if err != nil {
			t.Fatal(err)
		}

		return func() {
			os.Remove(keyDest)
		}
	}
}

// withInaccessibleKey creates inaccessible key of give type (specified by keyName)
func withInaccessibleKey(keyName string) setUpEnvFn {
	return func(t *testing.T) func() {
		t.Helper()
		var err error

		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}

		err = os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
		if err != nil {
			t.Fatal(err)
		}

		keyDest := filepath.Join(home, ".ssh", keyName)
		_, err = os.OpenFile(keyDest, os.O_CREATE|os.O_WRONLY, 0000)
		if err != nil {
			t.Fatal(err)
		}

		return func() {
			os.Remove(keyDest)
		}
	}
}

// sets clean temporary $HOME for test
// this prevents interaction with actual user home which may contain .ssh/
func withCleanHome(t *testing.T) func() {
	t.Helper()
	homeName := "HOME"
	if runtime.GOOS == "windows" {
		homeName = "USERPROFILE"
	}
	tmpDir, err := ioutil.TempDir("", "tmpHome")
	if err != nil {
		t.Fatal(err)
	}
	oldHome, hadHome := os.LookupEnv(homeName)
	os.Setenv(homeName, tmpDir)

	return func() {
		if hadHome {
			os.Setenv(homeName, oldHome)
		} else {
			os.Unsetenv(homeName)
		}
		os.RemoveAll(tmpDir)
	}
}

// withKnowHosts creates $HOME/.ssh/known_hosts with correct entries
func withKnowHosts(connConfig *ConnectionConfig) setUpEnvFn {
	return func(t *testing.T) func() {
		t.Helper()

		knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")

		err := os.MkdirAll(filepath.Join(homedir.Get(), ".ssh"), 0700)
		if err != nil {
			t.Fatal(err)
		}

		_, err = os.Stat(knownHosts)
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			t.Fatal("known_hosts already exists")
		}

		f, err := os.OpenFile(knownHosts, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		// generate known_hosts
		serverKeysDir := filepath.Join("testdata", "etc", "ssh")
		for _, k := range []string{"ecdsa"} {
			keyPath := filepath.Join(serverKeysDir, fmt.Sprintf("ssh_host_%s_key.pub", k))
			key, err := ioutil.ReadFile(keyPath)
			if err != nil {
				t.Fatal(t)
			}

			fmt.Fprintf(f, "%s %s", connConfig.hostIPv4, string(key))
			fmt.Fprintf(f, "[%s]:%d %s", connConfig.hostIPv4, connConfig.portIPv4, string(key))

			if connConfig.hostIPv6 != "" {
				fmt.Fprintf(f, "%s %s", connConfig.hostIPv6, string(key))
				fmt.Fprintf(f, "[%s]:%d %s", connConfig.hostIPv6, connConfig.portIPv6, string(key))
			}
		}

		return func() {
			os.Remove(knownHosts)
		}
	}
}

// withBadKnownHosts creates $HOME/.ssh/known_hosts with incorrect entries
func withBadKnownHosts(connConfig *ConnectionConfig) setUpEnvFn {
	return func(t *testing.T) func() {
		t.Helper()

		knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")

		err := os.MkdirAll(filepath.Join(homedir.Get(), ".ssh"), 0700)
		if err != nil {
			t.Fatal(err)
		}

		_, err = os.Stat(knownHosts)
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			t.Fatal("known_hosts already exists")
		}

		f, err := os.OpenFile(knownHosts, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		knownHostTemplate := `{{range $host := .}}{{$host}} ssh-dss AAAAB3NzaC1kc3MAAACBAKH4ufS3ABVb780oTgEL1eu+pI1p6YOq/1KJn5s3zm+L3cXXq76r5OM/roGEYrXWUDGRtfVpzYTAKoMWuqcVc0AZ2zOdYkoy1fSjJ3MqDGF53QEO3TXIUt3gUzmLOewwmZWle0RgMa9GHccv7XVVIZB36RR68ZEUswLaTnlVhXQ1AAAAFQCl4t/LnY7kuUI+tL2qT2XmxmiyqwAAAIB72XaO+LfyIiqBOaTkQf+5rvH1i6y6LDO1QD9pzGWUYw3y03AEveHJMjW0EjnYBKJjK39wcZNTieRyU54lhH/HWeWABn9NcQ3duEf1WSO/s7SPsFO2R6quqVSsStkqf2Yfdy4fl24mH41olwtNA6ft5nkVfkqrIa51si4jU8fBVAAAAIB8SSvyYBcyMGLUlQjzQqhhhAHer9x/1YbknVz+y5PHJLLjHjMC4ZRfLgNEojvMKQW46Te9Pwnudcwv19ho4F+kkCOfss7xjyH70gQm6Sj76DxClmnnPoSRq3qEAOMy5Oh+7vyzxm68KHqd/aOmUaiT1LgqgViS9+kNdCoVMGAMOg== mvasek@bellatrix
{{$host}} ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBLTxVVaQ93ReqHNlbjg5/nBRpuRuG6JIgNeJXWT1V4Dl+dMMrnad3uJBfyrNpvn8rv2qnn6gMTZVtTbLdo96pG0= mvasek@bellatrix
{{$host}} ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOKymJNQszrxetVffPZRfZGKWK786r0mNcg/Wah4+2wn mvasek@bellatrix
{{$host}} ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC/1/OCwec2Gyv5goNYYvos4iOA+a0NolOGsZA/93jmSArPY1zZS1UWeJ6dDTmxGoL/e7jm9lM6NJY7a/zM0C/GqCNRGR/aCUHBJTIgGtH+79FDKO/LWY6ClGY7Lw8qNgZpugbBw3N3HqTtyb2lELhFLT0FEb+le4WUbryooLK2zsz6DnqV4JvTYyyHcanS0h68iSXC7XbkZchvL99l5LT0gD1oDteBPKKFdNOwIjpMkk/IrbFM24xoNkaTDXN87EpQPQzYDfsoGymprc5OZZ8kzrtErQR+yfuunHfzzqDHWi7ga5pbgkuxNt10djWgCfBRsy07FTEgV0JirS0TCfwTBbqRzdjf3dgi8AP+WtkW3mcv4a1XYeqoBo2o9TbfyiA9kERs79UBN0mCe3KNX3Ns0PvutsRLaHmdJ49eaKWkJ6GgL37aqSlIwTixz2xY3eoDSkqHoZpx6Q1MdpSIl5gGVzlaobM/PNM1jqVdyUj+xpjHyiXwHQMKc3eJna7s8Jc= mvasek@bellatrix
{{end}}`

		tmpl := template.New(knownHostTemplate)
		tmpl, err = tmpl.Parse(knownHostTemplate)
		if err != nil {
			t.Fatal(err)
		}

		hosts := make([]string, 0, 4)
		hosts = append(hosts, connConfig.hostIPv4, fmt.Sprintf("[%s]:%d", connConfig.hostIPv4, connConfig.portIPv4))
		if connConfig.hostIPv6 != "" {
			hosts = append(hosts, connConfig.hostIPv6, fmt.Sprintf("[%s]:%d", connConfig.hostIPv6, connConfig.portIPv4))
		}

		err = tmpl.Execute(f, hosts)
		if err != nil {
			t.Fatal(err)
		}

		return func() {
			os.Remove(knownHosts)
		}
	}
}

// withBrokenKnownHosts creates broken $HOME/.ssh/known_hosts
func withBrokenKnownHosts(t *testing.T) func() {
	t.Helper()

	knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")

	err := os.MkdirAll(filepath.Join(homedir.Get(), ".ssh"), 0700)
	if err != nil {
		t.Fatal(err)
	}

	_, err = os.Stat(knownHosts)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatal("known_hosts already exists")
	}

	f, err := os.OpenFile(knownHosts, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, err = f.WriteString("somegarbage\nsome rubish\n stuff\tqwerty")
	if err != nil {
		t.Fatal(err)
	}

	return func() {
		os.Remove(knownHosts)
	}
}

// withInaccessibleKnownHosts creates inaccessible $HOME/.ssh/known_hosts
func withInaccessibleKnownHosts(t *testing.T) func() {
	t.Helper()

	knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")

	err := os.MkdirAll(filepath.Join(homedir.Get(), ".ssh"), 0700)
	if err != nil {
		t.Fatal(err)
	}

	_, err = os.Stat(knownHosts)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatal("known_hosts already exists")
	}

	f, err := os.OpenFile(knownHosts, os.O_CREATE|os.O_WRONLY, 0000)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	return func() {
		os.Remove(knownHosts)
	}
}

// withEmptyKnownHosts creates empty $HOME/.ssh/known_hosts
func withEmptyKnownHosts(t *testing.T) func() {
	t.Helper()

	knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")

	err := os.MkdirAll(filepath.Join(homedir.Get(), ".ssh"), 0700)
	if err != nil {
		t.Fatal(err)
	}

	_, err = os.Stat(knownHosts)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatal("known_hosts already exists")
	}

	_, err = os.Create(knownHosts)
	if err != nil {
		t.Fatal(err)
	}

	return func() {
		os.Remove(knownHosts)
	}
}

// withoutSSHAgent unsets the SSH_AUTH_SOCK environment variable so ssh-agent is not used by test
func withoutSSHAgent(t *testing.T) func() {
	t.Helper()
	oldAuthSock, hadAuthSock := os.LookupEnv("SSH_AUTH_SOCK")
	os.Unsetenv("SSH_AUTH_SOCK")

	return func() {
		if hadAuthSock {
			os.Setenv("SSH_AUTH_SOCK", oldAuthSock)
		} else {
			os.Unsetenv("SSH_AUTH_SOCK")
		}
	}
}

// withBadSSHAgentSocket sets the SSH_AUTH_SOCK environment variable to non-existing file
func withBadSSHAgentSocket(t *testing.T) func() {
	t.Helper()
	oldAuthSock, hadAuthSock := os.LookupEnv("SSH_AUTH_SOCK")
	os.Setenv("SSH_AUTH_SOCK", "/does/not/exists.sock")

	return func() {
		if hadAuthSock {
			os.Setenv("SSH_AUTH_SOCK", oldAuthSock)
		} else {
			os.Unsetenv("SSH_AUTH_SOCK")
		}
	}
}

// withGoodSSHAgent starts serving ssh-agent on temporary unix socket.
// It sets the SSH_AUTH_SOCK environment variable to the temporary socket.
// The agent will return correct keys for the testing ssh server.
func withGoodSSHAgent(t *testing.T) func() {
	t.Helper()

	key, err := ioutil.ReadFile(filepath.Join("testdata", "id_ed25519"))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}

	return withSSHAgent(t, signerAgent{signer})
}

// withBadSSHAgent starts serving ssh-agent on temporary unix socket.
// It sets the SSH_AUTH_SOCK environment variable to the temporary socket.
// The agent will return incorrect keys for the testing ssh server.
func withBadSSHAgent(t *testing.T) func() {
	return withSSHAgent(t, badAgent{})
}

func withSSHAgent(t *testing.T, ag agent.Agent) func() {
	t.Helper()
	tmpDirForSocket, err := ioutil.TempDir("", "forAuthSock")
	if err != nil {
		t.Fatal(err)
	}
	agentSocketPath := filepath.Join(tmpDirForSocket, "agent.sock")
	unixListener, err := net.Listen("unix", agentSocketPath)
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("SSH_AUTH_SOCK", agentSocketPath)

	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	var wg sync.WaitGroup

	go func() {
		for {
			conn, err := unixListener.Accept()
			if err != nil {
				errChan <- err

				return
			}

			wg.Add(1)
			go func(conn net.Conn) {
				defer wg.Done()
				go func() {
					<-ctx.Done()
					conn.Close()
				}()
				err := agent.ServeAgent(ag, conn)
				if err != nil {
					if !errors.Is(err, net.ErrClosed) {
						fmt.Fprintf(os.Stderr, "agent.ServeAgent() failed: %v\n", err)
					}
				}
			}(conn)
		}
	}()

	return func() {
		os.Unsetenv("SSH_AUTH_SOCK")

		err := unixListener.Close()
		if err != nil {
			t.Fatal(err)
		}
		err = <-errChan

		if !errors.Is(err, net.ErrClosed) {
			t.Fatal(err)
		}
		cancel()
		wg.Wait()
		os.RemoveAll(tmpDirForSocket)
	}
}

type signerAgent struct {
	impl ssh.Signer
}

func (a signerAgent) List() ([]*agent.Key, error) {
	return []*agent.Key{{
		Format: a.impl.PublicKey().Type(),
		Blob:   a.impl.PublicKey().Marshal(),
	}}, nil
}

func (a signerAgent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return a.impl.Sign(nil, data)
}

func (a signerAgent) Add(key agent.AddedKey) error {
	panic("implement me")
}

func (a signerAgent) Remove(key ssh.PublicKey) error {
	panic("implement me")
}

func (a signerAgent) RemoveAll() error {
	panic("implement me")
}

func (a signerAgent) Lock(passphrase []byte) error {
	panic("implement me")
}

func (a signerAgent) Unlock(passphrase []byte) error {
	panic("implement me")
}

func (a signerAgent) Signers() ([]ssh.Signer, error) {
	panic("implement me")
}

var errBadAgent = errors.New("bad agent error")

type badAgent struct{}

func (b badAgent) List() ([]*agent.Key, error) {
	return nil, errBadAgent
}

func (b badAgent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return nil, errBadAgent
}

func (b badAgent) Add(key agent.AddedKey) error {
	return errBadAgent
}

func (b badAgent) Remove(key ssh.PublicKey) error {
	return errBadAgent
}

func (b badAgent) RemoveAll() error {
	return errBadAgent
}

func (b badAgent) Lock(passphrase []byte) error {
	return errBadAgent
}

func (b badAgent) Unlock(passphrase []byte) error {
	return errBadAgent
}

func (b badAgent) Signers() ([]ssh.Signer, error) {
	return nil, errBadAgent
}

// openSSH CLI doesn't take the HOME/USERPROFILE environment variable into account.
// It gets user home in different way (e.g. reading /etc/passwd).
// This means tests cannot mock home dir just by setting environment variable.
// withFixedUpSSHCLI works around the problem, it forces usage of known_hosts from HOME/USERPROFILE.
func withFixedUpSSHCLI(t *testing.T) func() {
	t.Helper()

	which := "which"
	if runtime.GOOS == "windows" {
		which = "where"
	}

	out, err := exec.Command(which, "ssh").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	sshAbsPath := string(out)
	sshAbsPath = strings.Trim(sshAbsPath, "\r\n")

	sshScript := `#!/bin/sh
SSH_BIN -o PasswordAuthentication=no -o ConnectTimeout=3 -o UserKnownHostsFile="$HOME/.ssh/known_hosts" $@
`
	if runtime.GOOS == "windows" {
		sshScript = `@echo off
SSH_BIN -o PasswordAuthentication=no -o ConnectTimeout=3 -o UserKnownHostsFile=%USERPROFILE%\.ssh\known_hosts %*
`
	}
	sshScript = strings.ReplaceAll(sshScript, "SSH_BIN", sshAbsPath)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	homeBin := filepath.Join(home, "bin")
	err = os.MkdirAll(homeBin, 0700)
	if err != nil {
		t.Fatal(err)
	}

	sshScriptName := "ssh"
	if runtime.GOOS == "windows" {
		sshScriptName = "ssh.bat"
	}

	sshScriptFullPath := filepath.Join(homeBin, sshScriptName)
	err = ioutil.WriteFile(sshScriptFullPath, []byte(sshScript), 0700)
	if err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", homeBin+string(os.PathListSeparator)+oldPath)
	return func() {
		os.Setenv("PATH", oldPath)
		os.RemoveAll(homeBin)
	}
}

// withEmulatedDockerSystemDialStdio makes `docker system dial-stdio` viable in the testing ssh server.
// It does so by appending definition of shell function named `docker` into .bashrc .
func withEmulatedDockerSystemDialStdio(connConfig *ConnectionConfig) setUpEnvFn {
	return func(t *testing.T) func() {
		t.Helper()

		_, err := runRemote(`echo 'docker () {
			if [ "$1" = "system" ] && [ "$2" = "dial-stdio" ]; then
				if [ "$3" = "--help" ]; then
				  echo "\nProxy the stdio stream to the daemon connection.";
				else
				socat - /home/testuser/test.sock;
			  fi
			fi
		  }' >> ~/.bashrc`, connConfig)
		if err != nil {
			t.Fatal(err)
		}

		return func() {
			_, _ = runRemote(`echo 'unset -f docker' >> ~/.bashrc`, connConfig)
		}
	}
}

// withEmulatingWindows makes changes to the testing ssh server such that
// the server appears to be Windows server for simple check done calling the `systeminfo` command
func withEmulatingWindows(connConfig *ConnectionConfig) setUpEnvFn {
	return func(t *testing.T) func() {
		t.Helper()

		_, err := runRemote(`echo 'systeminfo () {
		echo '\nWindows\n'
	  }' >> ~/.bashrc`, connConfig)
		if err != nil {
			t.Fatal(err)
		}

		return func() {
			_, _ = runRemote(`echo 'unset -f systeminfo' >> ~/.bashrc`, connConfig)
		}
	}
}

// withRemoteDockerHost sets the DOCKER_HOST environment variable in the testing ssh server.
// It does so by appending export statement to .bashrc .
func withRemoteDockerHost(host string, connConfig *ConnectionConfig) setUpEnvFn {
	return func(t *testing.T) func() {
		t.Helper()
		_, err := runRemote(fmt.Sprintf(`echo 'export DOCKER_HOST=%s' >> ~/.bashrc`, host), connConfig)
		if err != nil {
			t.Fatal(err)
		}
		return func() {
			runRemote(`echo 'unset DOCKER_HOST' >> ~/.bashrc`, connConfig)
		}
	}
}

// runRemote runs command it the testing ssh server
func runRemote(cmd string, connConfig *ConnectionConfig) ([]byte, error) {
	u, err := url.Parse(fmt.Sprintf("ssh://testuser@%s:%d",
		connConfig.hostIPv4,
		connConfig.portIPv4,
	))
	if err != nil {
		return nil, errors.Wrap(err, "parsing url")
	}

	sshClientConfig, err := sshdialer.NewSSHClientConfig(u, sshdialer.Config{
		HostKeyCallback: func(hostPort string, pubKey ssh.PublicKey) error {
			return nil
		},
		PasswordCallback: func() (string, error) {
			return "idkfa", nil
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "creating ssh config")
	}

	sshClient, err := ssh.Dial("tcp", u.Host, sshClientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "connecting")
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		return nil, errors.Wrap(err, "starting session")
	}
	defer session.Close()

	return session.CombinedOutput(cmd)
}
