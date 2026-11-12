package cmd

import (
	"io"
	"os"
	"testing"

	h "github.com/buildpacks/pack/testhelpers"
)

// saveAndRestoreEnv saves the current value (or unset state) of an environment
func saveAndRestoreEnv(t *testing.T, key string) {
	t.Helper()
	orig, wasSet := os.LookupEnv(key)
	t.Cleanup(func() {
		if wasSet {
			os.Setenv(key, orig)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestNewPackCommand_DockerHostPersistentFlag(t *testing.T) {
	saveAndRestoreEnv(t, "DOCKER_HOST")

	logger := newTestLogger()
	rootCmd, err := NewPackCommand(logger)
	h.AssertNil(t, err)

	flag := rootCmd.PersistentFlags().Lookup("docker-host")
	h.AssertNotNil(t, flag)
	h.AssertEq(t, flag.DefValue, "")
}

func TestNewPackCommand_DockerHostInheritedBySubcommands(t *testing.T) {
	saveAndRestoreEnv(t, "DOCKER_HOST")

	logger := newTestLogger()
	rootCmd, err := NewPackCommand(logger)
	h.AssertNil(t, err)

	for _, childName := range []string{"builder", "buildpack", "rebase"} {
		child, _, err := rootCmd.Find([]string{childName})
		h.AssertNil(t, err)
		flag := child.InheritedFlags().Lookup("docker-host")
		h.AssertNotNil(t, flag)
	}
}

func TestNewPackCommand_DockerHostInheritedByNestedSubcommands(t *testing.T) {
	saveAndRestoreEnv(t, "DOCKER_HOST")

	logger := newTestLogger()
	rootCmd, err := NewPackCommand(logger)
	h.AssertNil(t, err)

	for _, path := range [][]string{
		{"builder", "create"},
		{"builder", "inspect"},
		{"buildpack", "package"},
	} {
		child, _, err := rootCmd.Find(path)
		h.AssertNil(t, err)
		flag := child.InheritedFlags().Lookup("docker-host")
		h.AssertNotNil(t, flag)
	}
}

func TestNewPackCommand_BuildCommandHasLocalDockerHost(t *testing.T) {
	saveAndRestoreEnv(t, "DOCKER_HOST")

	logger := newTestLogger()
	rootCmd, err := NewPackCommand(logger)
	h.AssertNil(t, err)

	buildCmd, _, err := rootCmd.Find([]string{"build"})
	h.AssertNil(t, err)

	localFlag := buildCmd.Flags().Lookup("docker-host")
	h.AssertNotNil(t, localFlag)

	localFlags := buildCmd.LocalFlags()
	h.AssertNotNil(t, localFlags.Lookup("docker-host"))
}

func TestNewPackCommand_DockerHostSetsEnv(t *testing.T) {
	saveAndRestoreEnv(t, "DOCKER_HOST")
	os.Unsetenv("DOCKER_HOST")

	logger := newTestLogger()
	rootCmd, err := NewPackCommand(logger)
	h.AssertNil(t, err)

	rootCmd.SetArgs([]string{"version", "--docker-host", "unix:///custom/docker.sock"})
	err = rootCmd.Execute()
	h.AssertNil(t, err)

	h.AssertEq(t, os.Getenv("DOCKER_HOST"), "unix:///custom/docker.sock")
}

func TestNewPackCommand_DockerHostInheritDoesNotOverrideEnv(t *testing.T) {
	saveAndRestoreEnv(t, "DOCKER_HOST")
	os.Setenv("DOCKER_HOST", "unix:///original/socket.sock")

	logger := newTestLogger()
	rootCmd, err := NewPackCommand(logger)
	h.AssertNil(t, err)

	rootCmd.SetArgs([]string{"version", "--docker-host", "inherit"})
	err = rootCmd.Execute()
	h.AssertNil(t, err)

	h.AssertEq(t, os.Getenv("DOCKER_HOST"), "unix:///original/socket.sock")
}

func newTestLogger() ConfigurableLogger {
	return &testLogger{}
}

type testLogger struct{}

func (l *testLogger) Debug(msg string)                    {}
func (l *testLogger) Debugf(fmt string, v ...interface{}) {}
func (l *testLogger) Info(msg string)                     {}
func (l *testLogger) Infof(fmt string, v ...interface{})  {}
func (l *testLogger) Warn(msg string)                     {}
func (l *testLogger) Warnf(fmt string, v ...interface{})  {}
func (l *testLogger) Error(msg string)                    {}
func (l *testLogger) Errorf(fmt string, v ...interface{}) {}
func (l *testLogger) Writer() io.Writer                   { return os.Stderr }
func (l *testLogger) IsVerbose() bool                     { return false }
func (l *testLogger) WantTime(f bool)                     {}
func (l *testLogger) WantQuiet(f bool)                    {}
func (l *testLogger) WantVerbose(f bool)                  {}
