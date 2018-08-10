package pack

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func Build(appDir, stackName, repoName string, useDaemon bool) error {
	if !useDaemon {
		return errors.New("NOT IMPLEMENTED (must use daemon)")
	}

	tempDir, err := ioutil.TempDir("/tmp", "lifecycle.pack.build.")
	if err != nil {
		return err
	}
	defer os.Remove(tempDir)

	cacheDir, err := cacheDir(appDir)
	if err != nil {
		return err
	}

	for _, name := range []string{"platform", "launch", "workspace"} {
		if err := os.Mkdir(filepath.Join(tempDir, name), 0755); err != nil {
			return err
		}
	}

	// TODO implement in go
	if out, err := exec.Command("cp", "-r", appDir, filepath.Join(tempDir, "launch", "app")).CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}

	fmt.Println("*** DETECTING:")
	cmd := exec.Command("docker", "run", "-v", filepath.Join(tempDir, "launch", "app")+":/launch/app", "-v", filepath.Join(tempDir, "workspace")+":/workspace", stackName+":detect")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Println("*** ANALYZING: Reading information from previous image for possible re-use")
	// TODO: We assume this will need root to saccess docker.sock, (if so need to chown afterwards)
	if out, err := exec.Command("docker", "run",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-v", filepath.Join(tempDir, "launch")+":/launch",
		"-v", filepath.Join(tempDir, "workspace")+":/workspace:ro",
		stackName+":analyze",
		"-daemon",
		repoName,
	).CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}

	fmt.Println("*** BUILDING:")
	cmd = exec.Command("docker", "run",
		"-v", filepath.Join(tempDir, "launch")+":/launch",
		"-v", filepath.Join(tempDir, "workspace")+":/workspace",
		"-v", cacheDir+":/cache",
		"-v", filepath.Join(tempDir, "platform")+":/platform",
		stackName+":build",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Println("*** EXPORTING:")
	args := []string{"run", "--user", "0", "-v", "/var/run/docker.sock:/var/run/docker.sock", "-v", filepath.Join(tempDir, "launch") + ":/launch:ro", "-v", filepath.Join(tempDir, "workspace") + ":/workspace:ro", stackName + ":export", "-daemon", "-daemon-stack", "-stack", stackName, repoName}
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}

	return nil
}

func cacheDir(appDir string) (string, error) {
	homeDir := os.Getenv("HOME")
	if runtime.GOOS == "windows" {
		homeDir = filepath.Join(os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"))
	}

	appDir, err := filepath.Abs(appDir)
	if err != nil {
		return "", err
	}
	appSHA := fmt.Sprintf("%x", md5.Sum([]byte(appDir)))
	cacheDir := filepath.Join(homeDir, ".pack", "cache", appSHA)

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}

	return cacheDir, nil
}
