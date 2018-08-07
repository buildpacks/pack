package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

func Build(appDir string, repoName string, useDaemon bool) error {
	if !useDaemon {
		return errors.New("NOT IMPLEMENTED (must use daemon)")
	}

	tempDir, err := ioutil.TempDir("/tmp", "lifecycle.pack.build.")
	if err != nil {
		return err
	}
	defer os.Remove(tempDir)

	for _, name := range []string{"platform", "cache", "launch", "workspace"} {
		if err := os.Mkdir(filepath.Join(tempDir, name), 0755); err != nil {
			return err
		}
	}

	// TODO implement in go
	if out, err := exec.Command("cp", "-r", appDir, filepath.Join(tempDir, "launch", "app")).CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}

	if out, err := exec.Command("docker", "run", "-v", filepath.Join(tempDir, "launch", "app")+":/launch/app", "-v", filepath.Join(tempDir, "workspace")+":/workspace", "packs/v3:detect").CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}

	// TODO: We assume this will need root to saccess docker.sock, (if so need to chown afterwards)
	if out, err := exec.Command("docker", "run", "-v", "/var/run/docker.sock:/var/run/docker.sock", "-v", filepath.Join(tempDir, "launch")+":/launch", "-v", filepath.Join(tempDir, "workspace")+":/workspace:ro", "packs/v3:analyze", "-daemon", repoName).CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}

	cmd := exec.Command("docker", "run", "-v", filepath.Join(tempDir, "launch")+":/launch", "-v", filepath.Join(tempDir, "workspace")+":/workspace", "-v", filepath.Join(tempDir, "cache")+":/cache", "-v", filepath.Join(tempDir, "platform")+":/platform", "packs/v3:build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	args := []string{"run", "--user", "0", "-v", "/var/run/docker.sock:/var/run/docker.sock", "-v", filepath.Join(tempDir, "launch") + ":/launch:ro", "-v", filepath.Join(tempDir, "workspace") + ":/workspace:ro", "packs/v3:export", "-daemon", "-daemon-stack", "-stack", "packs/v3", repoName}
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}

	return nil
}
