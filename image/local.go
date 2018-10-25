package image

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/buildpack/lifecycle/img"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"io"
	"math/rand"
	"strings"

	"github.com/buildpack/pack/fs"
	"github.com/docker/docker/api/types"
	dockertypes "github.com/docker/docker/api/types"
	dockercli "github.com/docker/docker/client"
	"github.com/pkg/errors"
)

type local struct {
	RepoName         string
	Docker           Docker
	Inspect          types.ImageInspect
	Stdout           io.Writer
	FS               *fs.FS
	currentTempImage string
}

func (f *Factory) NewLocal(repoName string, pull bool) (Image, error) {
	if pull {
		f.Log.Printf("Pulling image '%s'\n", repoName)
		if err := f.Docker.PullImage(repoName); err != nil {
			return nil, fmt.Errorf("failed to pull image '%s' : %s", repoName, err)
		}
	}

	inspect, _, err := f.Docker.ImageInspectWithRaw(context.Background(), repoName)
	if err != nil && !dockercli.IsErrNotFound(err) {
		return nil, errors.Wrap(err, "analyze read previous image config")
	}

	return &local{
		Docker:   f.Docker,
		RepoName: repoName,
		Inspect:  inspect,
		Stdout:   f.Stdout,
		FS:       f.FS,
	}, nil
}

func (l *local) Label(key string) (string, error) {
	if l.Inspect.Config == nil {
		return "", fmt.Errorf("failed to get label, image '%s' does not exist", l.RepoName)
	}
	labels := l.Inspect.Config.Labels
	return labels[key], nil
}

func (l *local) Name() string {
	return l.RepoName
}

func (l *local) Digest() (string, error) {
	if l.Inspect.Config == nil {
		return "", fmt.Errorf("failed to get digest, image '%s' does not exist", l.RepoName)
	}
	if len(l.Inspect.RepoDigests) == 0 {
		return "", fmt.Errorf("failed to get digest, image '%s' has no repo digests", l.RepoName)
	}
	parts := strings.Split(l.Inspect.RepoDigests[0], "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("failed to get digest, image '%s' has malformed digest '%s'", l.RepoName, l.Inspect.RepoDigests[0])
	}
	return parts[1], nil
}

func (l *local) Rebase(baseTopLayer string, newBase Image) error {
	repoStore, err := img.NewDaemon(l.RepoName)
	if err != nil {
		return errors.Wrap(err, "rebase")
	}
	image, err := repoStore.Image()
	if err != nil {
		return errors.Wrap(err, "rebase")
	}

	newBaseStore, err := img.NewDaemon(newBase.Name())
	if err != nil {
		return errors.Wrap(err, "rebase")
	}
	newBaseImage, err := newBaseStore.Image()
	if err != nil {
		return errors.Wrap(err, "rebase")
	}

	oldBase := &subImage{img: image, topSHA: baseTopLayer}
	image, err = mutate.Rebase(image, oldBase, newBaseImage, &mutate.RebaseOptions{})
	if err != nil {
		return errors.Wrap(err, "rebase")
	}

	l.currentTempImage = "pack-rebase-tmp-" + randString(8)
	repoStore, err = img.NewDaemon(l.currentTempImage)
	if err != nil {
		return errors.Wrap(err, "rebase")
	}
	if err := repoStore.Write(image); err != nil {
		return errors.Wrap(err, "rebase")
	}

	return nil
}

func (l *local) SetLabel(key, val string) error {
	if l.Inspect.Config == nil {
		return fmt.Errorf("failed to set label, image '%s' does not exist", l.RepoName)
	}
	l.Inspect.Config.Labels[key] = val
	return nil
}

func (l *local) TopLayer() (string, error) {
	all := l.Inspect.RootFS.Layers
	topLayer := all[len(all)-1]
	return topLayer, nil
}

func (l *local) Save() (string, error) {
	dockerFile := "FROM scratch\n"
	if l.currentTempImage != "" {
		dockerFile = fmt.Sprintf("FROM %s\n", l.currentTempImage)
		defer func() {
			l.Docker.ImageRemove(context.TODO(), l.currentTempImage, dockertypes.ImageRemoveOptions{})
			l.currentTempImage = ""
		}()
	}
	if l.Inspect.Config != nil {
		for k, v := range l.Inspect.Config.Labels {
			dockerFile += fmt.Sprintf("LABEL %s=%s\n", k, v)
		}
	}

	r2, err := l.FS.CreateSingleFileTar("Dockerfile", dockerFile)
	if err != nil {
		return "", errors.Wrap(err, "image build")
	}

	res, err := l.Docker.ImageBuild(context.TODO(), r2, dockertypes.ImageBuildOptions{Tags: []string{l.RepoName}})
	if err != nil {
		return "", errors.Wrap(err, "image build")
	}
	defer res.Body.Close()
	imageID, err := parseImageBuildBody(res.Body, l.Stdout)
	if err != nil {
		return "", errors.Wrap(err, "image build")
	}
	res.Body.Close()

	return imageID, nil
}

// TODO copied from exporter.go
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

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}
