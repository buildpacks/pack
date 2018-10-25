package pack

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/img"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/packs"
)

func exportRegistry(group *lifecycle.BuildpackGroup, uid, gid int, launchDirSrc, repoName, runImage string, stdout, stderr io.Writer) (string, error) {
	images := &image.Client{}
	origImage, err := images.ReadImage(repoName, false)
	if err != nil {
		return "", err
	}

	stackImage, err := images.ReadImage(runImage, false)
	if err != nil || stackImage == nil {
		return "", packs.FailErr(err, "get image for", runImage)
	}

	repoStore, err := img.NewRegistry(repoName)
	if err != nil {
		return "", packs.FailErr(err, "access", repoName)
	}

	tmpDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return "", packs.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(tmpDir)

	exporter := &lifecycle.Exporter{
		Buildpacks: group.Buildpacks,
		TmpDir:     tmpDir,
		Out:        stdout,
		Err:        stderr,
		UID:        uid,
		GID:        gid,
	}
	newImage, err := exporter.Export(
		launchDirSrc,
		defaultLaunchDir,
		stackImage,
		origImage,
	)
	if err != nil {
		return "", packs.FailErrCode(err, packs.CodeFailedBuild)
	}

	if err := repoStore.Write(newImage); err != nil {
		return "", packs.FailErrCode(err, packs.CodeFailedUpdate, "write")
	}

	sha, err := newImage.Digest()
	if err != nil {
		return "", packs.FailErr(err, "calculating image digest")
	}

	return sha.String(), nil
}

func exportDaemon(cli Docker, buildpacks []string, workspaceVolume, repoName, runImage string, stdout io.Writer, uid, gid int) error {
	ctx := context.Background()
	ctr, err := cli.ContainerCreate(ctx, &container.Config{
		Image:      runImage,
		User:       "root",
		Entrypoint: []string{},
		Cmd:        []string{"echo", "hi"},
	}, &container.HostConfig{
		Binds: []string{
			workspaceVolume + ":/workspace",
		},
	}, nil, "")
	if err != nil {
		return errors.Wrap(err, "container create")
	}

	r, _, err := cli.CopyFromContainer(ctx, ctr.ID, "/workspace")
	if err != nil {
		return errors.Wrap(err, "copy from container")
	}

	r2, layerChan, errChan := addDockerfileToTar(uid, gid, runImage, repoName, buildpacks, r)

	res, err := cli.ImageBuild(ctx, r2, dockertypes.ImageBuildOptions{Tags: []string{repoName}})
	if err != nil {
		return errors.Wrap(err, "image build")
	}
	defer res.Body.Close()
	if _, err := parseImageBuildBody(res.Body, stdout); err != nil {
		return errors.Wrap(err, "image build")
	}
	res.Body.Close()

	if err := <-errChan; err != nil {
		return errors.Wrap(err, "modify tar to add dockerfile")
	}
	layerNames := <-layerChan

	// Calculate metadata
	i, _, err := cli.ImageInspectWithRaw(ctx, repoName)
	if err != nil {
		return errors.Wrap(err, "inspect image to find layers")
	}
	runImageDigest, err := daemonImageDigest(cli, runImage)
	if err != nil {
		return errors.Wrap(err, "find run image digest")
	}

	layerIDX := len(i.RootFS.Layers) - len(layerNames)
	metadata := lifecycle.AppImageMetadata{
		RunImage: lifecycle.RunImageMetadata{
			SHA:      runImageDigest,
			TopLayer: i.RootFS.Layers[layerIDX-3],
		},
		App: lifecycle.AppMetadata{
			SHA: i.RootFS.Layers[layerIDX-2],
		},
		Config: lifecycle.ConfigMetadata{
			SHA: i.RootFS.Layers[layerIDX-1],
		},
		Buildpacks: []lifecycle.BuildpackMetadata{},
	}
	for _, buildpack := range buildpacks {
		data := lifecycle.BuildpackMetadata{ID: buildpack, Layers: make(map[string]lifecycle.LayerMetadata)}
		for _, layer := range layerNames {
			if layer.buildpack == buildpack {
				data.Layers[layer.layer] = lifecycle.LayerMetadata{
					SHA:  i.RootFS.Layers[layerIDX],
					Data: layer.data,
				}
				layerIDX++
			}
		}
		metadata.Buildpacks = append(metadata.Buildpacks, data)
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return errors.Wrap(err, "marshal metadata to json")
	}
	if err := addLabelToImage(cli, repoName, map[string]string{lifecycle.MetadataLabel: string(metadataJSON)}, stdout); err != nil {
		return errors.Wrapf(err, "adding %s label to image", lifecycle.MetadataLabel)
	}

	return nil
}
func daemonImageDigest(cli Docker, repoName string) (string, error) {
	i, _, err := cli.ImageInspectWithRaw(context.Background(), repoName)
	if err != nil {
		return "", errors.Wrap(err, "inspect image")
	}
	if len(i.RepoDigests) < 1 {
		return "", errors.Wrap(err, "missing digest")
	}
	parts := strings.Split(i.RepoDigests[0], "@")
	if len(parts) != 2 {
		return "", errors.Wrap(err, "bad digest")
	}
	return parts[1], nil
}

func addLabelToImage(cli Docker, repoName string, labels map[string]string, stdout io.Writer) error {
	dockerfile := "FROM " + repoName + "\n"
	for k, v := range labels {
		dockerfile += fmt.Sprintf("LABEL %s='%s'\n", k, v)
	}
	f := &fs.FS{}
	tr, err := f.CreateSingleFileTar("Dockerfile", dockerfile)
	if err != nil {
		return err
	}
	res, err := cli.ImageBuild(context.Background(), tr, dockertypes.ImageBuildOptions{
		Tags: []string{repoName},
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if _, err := parseImageBuildBody(res.Body, stdout); err != nil {
		return errors.Wrap(err, "image build")
	}
	return err
}

type dockerfileLayer struct {
	buildpack string
	layer     string
	data      interface{}
}

func addDockerfileToTar(uid, gid int, runImage, repoName string, buildpacks []string, r io.Reader) (io.Reader, chan []dockerfileLayer, chan error) {
	chownUser := fmt.Sprintf("%d:%d", uid, gid)
	dockerFile := "FROM " + runImage + "\n"
	dockerFile += fmt.Sprintf("ADD --chown=%s /workspace/app /workspace/app\n", chownUser)
	dockerFile += fmt.Sprintf("ADD --chown=%s /workspace/config /workspace/config\n", chownUser)
	layerChan := make(chan []dockerfileLayer, 1)
	var layerNames []dockerfileLayer
	errChan := make(chan error, 1)

	pr, pw := io.Pipe()
	tw := tar.NewWriter(pw)

	isBuildpack := make(map[string]bool)
	for _, b := range buildpacks {
		isBuildpack[b] = true
	}

	go func() {
		defer pw.Close()
		tr := tar.NewReader(r)
		tomlFiles := make(map[string]map[string]interface{})
		dirs := make(map[string]map[string]bool)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break // End of archive
			}
			if err != nil {
				layerChan <- nil
				errChan <- errors.Wrap(err, "tr.Next")
				return
			}

			tw.WriteHeader(hdr)

			arr := strings.Split(strings.TrimPrefix(strings.TrimSuffix(hdr.Name, "/"), "/"), "/")
			if len(arr) == 3 && isBuildpack[arr[1]] && strings.HasSuffix(arr[2], ".toml") && arr[2] != "launch.toml" {
				if tomlFiles[arr[1]] == nil {
					tomlFiles[arr[1]] = make(map[string]interface{})
				}

				buf, err := ioutil.ReadAll(tr)
				if err != nil {
					layerChan <- nil
					errChan <- errors.Wrap(err, "read toml file")
					return
				}
				if _, err := tw.Write(buf); err != nil {
					layerChan <- nil
					errChan <- errors.Wrap(err, "write toml file")
					return
				}

				var data interface{}
				if _, err := toml.Decode(string(buf), &data); err != nil {
					layerChan <- nil
					errChan <- errors.Wrap(err, "parsing toml file: "+hdr.Name)
					return
				}
				tomlFiles[arr[1]][strings.TrimSuffix(arr[2], ".toml")] = data
			} else if len(arr) == 3 && isBuildpack[arr[1]] && hdr.Typeflag == tar.TypeDir {
				if dirs[arr[1]] == nil {
					dirs[arr[1]] = make(map[string]bool)
				}
				dirs[arr[1]][arr[2]] = true
			}

			// TODO is it OK to do this if we have already read it above? eg. toml file
			if _, err := io.Copy(tw, tr); err != nil {
				layerChan <- nil
				errChan <- errors.Wrap(err, "io copy")
				return
			}
		}

		copyFromPrev := false
		for _, buildpack := range buildpacks {
			layers := sortedKeys(tomlFiles[buildpack])
			for _, layer := range layers {
				layerNames = append(layerNames, dockerfileLayer{buildpack, layer, tomlFiles[buildpack][layer]})
				if dirs[buildpack][layer] {
					dockerFile += fmt.Sprintf("ADD --chown=%s /workspace/%s/%s /workspace/%s/%s\n", chownUser, buildpack, layer, buildpack, layer)
				} else {
					dockerFile += fmt.Sprintf("COPY --from=prev --chown=%s /workspace/%s/%s /workspace/%s/%s\n", chownUser, buildpack, layer, buildpack, layer)
					copyFromPrev = true
				}
			}
		}
		if copyFromPrev {
			dockerFile = "FROM " + repoName + " AS prev\n\n" + dockerFile
		}

		if err := tw.WriteHeader(&tar.Header{Name: "Dockerfile", Size: int64(len(dockerFile)), Mode: 0666}); err != nil {
			layerChan <- nil
			errChan <- errors.Wrap(err, "write tar header for Dockerfile")
			return
		}
		if _, err := tw.Write([]byte(dockerFile)); err != nil {
			layerChan <- nil
			errChan <- errors.Wrap(err, "write Dockerfile to tar")
			return
		}

		tw.Close()
		pw.Close()
		layerChan <- layerNames
		errChan <- nil
	}()

	return pr, layerChan, errChan
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parseImageBuildBody(r io.Reader, out io.Writer) (string, error) {
	jr := json.NewDecoder(r)
	var id string
	var streamError error
	var obj struct {
		Stream string `json:"stream"`
		Error  string `json:"error"`
		Aux    struct {
			ID string `json:"ID"`
		} `json:"aux"`
	}
	for {
		err := jr.Decode(&obj)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		if obj.Aux.ID != "" {
			id = obj.Aux.ID
		}
		if txt := strings.TrimSpace(obj.Stream); txt != "" {
			fmt.Fprintln(out, txt)
		}
		if txt := strings.TrimSpace(obj.Error); txt != "" {
			streamError = errors.New(txt)
		}
	}
	return id, streamError
}
