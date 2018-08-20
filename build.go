package pack

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

func Build(appDir, detectImage, analyzeImage, exportImage, repoName, hostMachineIP string, publish bool) error {
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

	if err := recursiveCopy(appDir, filepath.Join(tempDir, "launch", "app")); err != nil {
		return err
	}

	if err := ioutil.WriteFile(filepath.Join(tempDir, "docker-config.json"), []byte(`{}`), 0644); err != nil {
		return err
	}

	fmt.Println("*** DETECTING:")
	cmd := exec.Command("docker", "run", "-v", filepath.Join(tempDir, "launch", "app")+":/launch/app", "-v", filepath.Join(tempDir, "workspace")+":/workspace", detectImage)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Println("*** ANALYZING: Reading information from previous image for possible re-use")
	// TODO: We assume this will need root to access docker.sock, (if so need to chown afterwards)
	args := []string{
		"run",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-v", filepath.Join(tempDir, "docker-config.json") + ":/home/packs/.docker/config.json",
		"-v", filepath.Join(tempDir, "docker-config.json") + ":/etc/docker/daemon.json",
		"-v", filepath.Join(tempDir, "launch") + ":/launch",
		"-v", filepath.Join(tempDir, "workspace") + ":/workspace:ro",
		analyzeImage,
	}
	if hostMachineIP != "" {
		args = append([]string{args[0], "--add-host", "host-machine.local:" + hostMachineIP}, args[1:]...)
	}
	if !publish {
		args = append(args, "-daemon")
	}
	args = append(args, repoName)
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}

	// Read groupRepoImage from ENV:PACK_BP_GROUP_PATH
	groupRepoImage, err := groupTomlRepository(tempDir, detectImage)
	if err != nil {
		return err
	}

	fmt.Println("*** BUILDING:")
	cmd = exec.Command("docker", "run",
		"-v", filepath.Join(tempDir, "launch")+":/launch",
		"-v", filepath.Join(tempDir, "workspace")+":/workspace",
		"-v", cacheDir+":/cache",
		"-v", filepath.Join(tempDir, "platform")+":/platform",
		groupRepoImage+":build",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Println("*** EXPORTING:")
	args = []string{
		"run",
		"--user", "0",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-v", filepath.Join(tempDir, "docker-config.json") + ":/root/.docker/config.json",
		"-v", filepath.Join(tempDir, "docker-config.json") + ":/etc/docker/daemon.json",
		"-v", filepath.Join(tempDir, "launch") + ":/launch:ro",
		"-v", filepath.Join(tempDir, "workspace") + ":/workspace:ro",
		exportImage,
		"-stack", groupRepoImage,
	}
	if hostMachineIP != "" {
		args = append([]string{args[0], "--add-host", "host-machine.local:" + hostMachineIP}, args[1:]...)
	}
	if !publish {
		// TODO: We probably don't want daemon-stack by default
		args = append(args, "-daemon", "-daemon-stack")
	}
	args = append(args, repoName)
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}

	return nil
}

func groupTomlRepository(tempDir, detectImage string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command("docker", "run", "-v", filepath.Join(tempDir, "workspace")+":/workspace:ro", "--entrypoint", "", detectImage, "bash", "-c", "cat $PACK_BP_GROUP_PATH")
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	var groupToml struct {
		Repository string `toml:"repository"`
	}
	if _, err := toml.Decode(buf.String(), &groupToml); err != nil {
		return "", err
	}

	return groupToml.Repository, nil
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

func recursiveCopy(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dest := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.Mkdir(dest, info.Mode())
		}

		destFile, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer destFile.Close()

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		if _, err := io.Copy(destFile, srcFile); err != nil {
			return err
		}

		return nil
	})
}
