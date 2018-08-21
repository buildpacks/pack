package pack

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/google/uuid"
)

func Build(appDir, detectImage, repoName string, publish bool) error {
	appDir, err := filepath.Abs(appDir)
	if err != nil {
		return err
	}

	uid := uuid.New().String()
	launchVolume := fmt.Sprintf("pack-launch-%x", uid)
	workspaceVolume := fmt.Sprintf("pack-workspace-%x", uid)
	cacheVolume := fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(appDir)))
	defer exec.Command("docker", "volume", "rm", "-f", launchVolume).Run()
	defer exec.Command("docker", "volume", "rm", "-f", workspaceVolume).Run()

	// fmt.Println("*** COPY APP TO VOLUME:")
	if err := copyToVolume(detectImage, launchVolume, appDir, "app"); err != nil {
		return err
	}

	fmt.Println("*** DETECTING:")
	cmd := exec.Command("docker", "run", "--rm", "-v", launchVolume+":/launch", "-v", workspaceVolume+":/workspace", detectImage)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	group, err := groupToml(workspaceVolume, detectImage)
	if err != nil {
		return err
	}

	fmt.Println("*** ANALYZING: Reading information from previous image for possible re-use")
	analyzeTmpDir, err := ioutil.TempDir("", "pack.build.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(analyzeTmpDir)
	if err := analyzer(group, analyzeTmpDir, repoName, !publish); err != nil {
		return err
	}
	if err := copyToVolume(detectImage, launchVolume, analyzeTmpDir, ""); err != nil {
		return err
	}

	fmt.Println("*** BUILDING:")
	cmd = exec.Command("docker", "run",
		"--rm",
		"-v", launchVolume+":/launch",
		"-v", workspaceVolume+":/workspace",
		"-v", cacheVolume+":/cache",
		group.Repository+":build",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	if !publish {
		if out, err := exec.Command("docker", "pull", group.Repository+":run").CombinedOutput(); err != nil {
			fmt.Println(string(out))
			return err
		}
	}

	fmt.Println("*** EXPORTING:")
	localLaunchDir, cleanup, err := exportVolume(detectImage, launchVolume)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := export(group, localLaunchDir, repoName, group.Repository+":run", !publish, !publish); err != nil {
		return err
	}

	return nil
}

func exportVolume(image, volName string) (string, func(), error) {
	tmpDir, err := ioutil.TempDir("", "pack.build.")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	containerName := uuid.New().String()
	if output, err := exec.Command("docker", "container", "create", "--name", containerName, "-v", volName+":/launch:ro", image).CombinedOutput(); err != nil {
		cleanup()
		fmt.Println(string(output))
		return "", func() {}, err
	}
	defer exec.Command("docker", "rm", containerName).Run()
	if output, err := exec.Command("docker", "cp", containerName+":/launch/.", tmpDir).CombinedOutput(); err != nil {
		cleanup()
		fmt.Println(string(output))
		return "", func() {}, err
	}

	return tmpDir, cleanup, nil
}

func copyToVolume(image, volName, srcDir, destDir string) error {
	containerName := uuid.New().String()
	if output, err := exec.Command("docker", "container", "create", "--user", "0", "--name", containerName, "--entrypoint", "", "-v", volName+":/launch", image, "chown", "-R", "packs:packs", "/launch").CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return err
	}
	defer exec.Command("docker", "rm", containerName).Run()
	if output, err := exec.Command("docker", "cp", srcDir+"/.", containerName+":"+filepath.Join("/launch", destDir)).CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return err
	}

	if output, err := exec.Command("docker", "start", containerName).CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return err
	}
	return nil
}

func groupToml(workspaceVolume, detectImage string) (lifecycle.BuildpackGroup, error) {
	var buf bytes.Buffer
	cmd := exec.Command("docker", "run", "--rm", "-v", workspaceVolume+":/workspace:ro", "--entrypoint", "", detectImage, "bash", "-c", "cat $PACK_BP_GROUP_PATH")
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return lifecycle.BuildpackGroup{}, err
	}

	var group lifecycle.BuildpackGroup
	if _, err := toml.Decode(buf.String(), &group); err != nil {
		return lifecycle.BuildpackGroup{}, err
	}

	return group, nil
}
