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

func Build(appDir, buildImage, runImage, repoName string, publish bool) error {
	return (&BuildFlags{
		AppDir:     appDir,
		BuildImage: buildImage,
		RunImage:   runImage,
		RepoName:   repoName,
		Publish:    publish,
	}).Run()
}

type BuildFlags struct {
	AppDir     string
	BuildImage string
	RunImage   string
	RepoName   string
	Publish    bool
}

func (b *BuildFlags) Run() error {
	var err error
	b.AppDir, err = filepath.Abs(b.AppDir)
	if err != nil {
		return err
	}

	uid := uuid.New().String()
	workspaceVolume := fmt.Sprintf("pack-workspace-%x", uid)
	cacheVolume := fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(b.AppDir)))
	defer exec.Command("docker", "volume", "rm", "-f", workspaceVolume).Run()

	fmt.Println("*** COPY APP TO VOLUME:")
	if err := copyToVolume(b.BuildImage, workspaceVolume, b.AppDir, "app"); err != nil {
		return err
	}

	fmt.Println("*** DETECTING:")
	cmd := exec.Command("docker", "run", "--rm", "-v", workspaceVolume+":/workspace", b.BuildImage, "/lifecycle/detector")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	group, err := groupToml(workspaceVolume, b.BuildImage)
	if err != nil {
		return err
	}

	fmt.Println("*** ANALYZING: Reading information from previous image for possible re-use")
	analyzeTmpDir, err := ioutil.TempDir("", "pack.build.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(analyzeTmpDir)
	if err := analyzer(group, analyzeTmpDir, b.RepoName, !b.Publish); err != nil {
		return err
	}
	if err := copyToVolume(b.BuildImage, workspaceVolume, analyzeTmpDir, ""); err != nil {
		return err
	}

	fmt.Println("*** BUILDING:")
	cmd = exec.Command("docker", "run",
		"--rm",
		"-v", workspaceVolume+":/workspace",
		"-v", cacheVolume+":/cache",
		b.BuildImage,
		"/lifecycle/builder",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	if !b.Publish {
		fmt.Println("*** PULLING RUN IMAGE LOCALLY:")
		if out, err := exec.Command("docker", "pull", b.RunImage).CombinedOutput(); err != nil {
			fmt.Println(string(out))
			return err
		}
	}

	fmt.Println("*** EXPORTING:")
	if b.Publish {
		localWorkspaceDir, cleanup, err := exportVolume(b.BuildImage, workspaceVolume)
		if err != nil {
			return err
		}
		defer cleanup()

		imgSHA, err := exportRegistry(&group, localWorkspaceDir, b.RepoName, b.RunImage)
		if err != nil {
			return err
		}
		fmt.Printf("\n*** Image: %s@%s\n", b.RepoName, imgSHA)
	} else {
		var buildpacks []string
		for _, b := range group.Buildpacks {
			buildpacks = append(buildpacks, b.ID)
		}

		if err := exportDaemon(buildpacks, workspaceVolume, b.RepoName, b.RunImage); err != nil {
			return err
		}
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
	if output, err := exec.Command("docker", "container", "create", "--name", containerName, "-v", volName+":/workspace:ro", image).CombinedOutput(); err != nil {
		cleanup()
		fmt.Println(string(output))
		return "", func() {}, err
	}
	defer exec.Command("docker", "rm", containerName).Run()
	if output, err := exec.Command("docker", "cp", containerName+":/workspace/.", tmpDir).CombinedOutput(); err != nil {
		cleanup()
		fmt.Println(string(output))
		return "", func() {}, err
	}

	return tmpDir, cleanup, nil
}

func copyToVolume(image, volName, srcDir, destDir string) error {
	containerName := uuid.New().String()
	if output, err := exec.Command("docker", "container", "create", "--user", "0", "--name", containerName, "--entrypoint", "", "-v", volName+":/workspace", image, "chown", "-R", "pack:pack", "/workspace").CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return err
	}
	defer exec.Command("docker", "rm", containerName).Run()
	if output, err := exec.Command("docker", "cp", srcDir+"/.", containerName+":"+filepath.Join("/workspace", destDir)).CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return err
	}

	if output, err := exec.Command("docker", "start", containerName).CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return err
	}
	return nil
}

func groupToml(workspaceVolume, buildImage string) (lifecycle.BuildpackGroup, error) {
	var buf bytes.Buffer
	cmd := exec.Command("docker", "run", "--rm", "-v", workspaceVolume+":/workspace:ro", "--entrypoint", "", buildImage, "bash", "-c", "cat $PACK_GROUP_PATH")
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
