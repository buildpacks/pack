package image

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/archive"
)

type local struct {
	RepoName         string
	Docker           *dockerclient.Client
	Inspect          types.ImageInspect
	layerPaths       []string
	Stdout           io.Writer
	currentTempImage string
	prevDir          string
	prevMap          map[string]string
	prevOnce         *sync.Once
	easyAddLayers    []string
}

func (f *Factory) NewLocal(repoName string) (Image, error) {
	inspect, _, err := f.Docker.ImageInspectWithRaw(context.Background(), repoName)
	if err != nil && !dockerclient.IsErrNotFound(err) {
		return nil, err
	}

	return &local{
		Docker:     f.Docker,
		RepoName:   repoName,
		Inspect:    inspect,
		layerPaths: make([]string, len(inspect.RootFS.Layers)),
		prevOnce:   &sync.Once{},
	}, nil
}

func (f *Factory) NewEmptyLocal(repoName string) Image {
	inspect := dockertypes.ImageInspect{}
	inspect.Config = &container.Config{
		Labels: map[string]string{},
	}
	return &local{
		RepoName: repoName,
		Docker:   f.Docker,
		Inspect:  inspect,
		prevOnce: &sync.Once{},
	}
}

func (l *local) Label(key string) (string, error) {
	if l.Inspect.Config == nil {
		return "", fmt.Errorf("failed to get label, image '%s' does not exist", l.RepoName)
	}
	labels := l.Inspect.Config.Labels
	return labels[key], nil
}

func (l *local) Env(key string) (string, error) {
	if l.Inspect.Config == nil {
		return "", fmt.Errorf("failed to get env var, image '%s' does not exist", l.RepoName)
	}
	for _, envVar := range l.Inspect.Config.Env {
		parts := strings.Split(envVar, "=")
		if parts[0] == key {
			return parts[1], nil
		}
	}
	return "", nil
}

func (l *local) Rename(name string) {
	l.easyAddLayers = nil
	if prevInspect, _, err := l.Docker.ImageInspectWithRaw(context.TODO(), name); err == nil {
		if l.sameBase(prevInspect) {
			l.easyAddLayers = prevInspect.RootFS.Layers[len(l.Inspect.RootFS.Layers):]
		}
	}

	l.RepoName = name
}

func (l *local) sameBase(prevInspect types.ImageInspect) bool {
	if len(prevInspect.RootFS.Layers) < len(l.Inspect.RootFS.Layers) {
		return false
	}
	for i, baseLayer := range l.Inspect.RootFS.Layers {
		if baseLayer != prevInspect.RootFS.Layers[i] {
			return false
		}
	}
	return true
}

func (l *local) Name() string {
	return l.RepoName
}

func (l *local) Found() (bool, error) {
	return l.Inspect.Config != nil, nil
}

func (l *local) Digest() (string, error) {
	if found, err := l.Found(); err != nil {
		return "", errors.Wrap(err, "determining image existence")
	} else if !found {
		return "", fmt.Errorf("failed to get digest, image '%s' does not exist", l.RepoName)
	}
	if len(l.Inspect.RepoDigests) == 0 {
		return "", nil
	}
	parts := strings.Split(l.Inspect.RepoDigests[0], "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("failed to get digest, image '%s' has malformed digest '%s'", l.RepoName, l.Inspect.RepoDigests[0])
	}
	return parts[1], nil
}

func (l *local) Rebase(baseTopLayer string, newBase Image) error {
	ctx := context.Background()

	// FIND TOP LAYER
	keepLayers := -1
	for i, diffID := range l.Inspect.RootFS.Layers {
		if diffID == baseTopLayer {
			keepLayers = len(l.Inspect.RootFS.Layers) - i - 1
			break
		}
	}
	if keepLayers == -1 {
		return fmt.Errorf("'%s' not found in '%s' during rebase", baseTopLayer, l.RepoName)
	}

	// SWITCH BASE LAYERS
	newBaseInspect, _, err := l.Docker.ImageInspectWithRaw(ctx, newBase.Name())
	if err != nil {
		return errors.Wrap(err, "analyze read previous image config")
	}
	l.Inspect.RootFS.Layers = newBaseInspect.RootFS.Layers
	l.layerPaths = make([]string, len(l.Inspect.RootFS.Layers))

	// SAVE CURRENT IMAGE TO DISK
	if err := l.prevDownload(); err != nil {
		return err
	}

	// READ MANIFEST.JSON
	b, err := ioutil.ReadFile(filepath.Join(l.prevDir, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest []struct{ Layers []string }
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	if len(manifest) != 1 {
		return fmt.Errorf("expected 1 image received %d", len(manifest))
	}

	// ADD EXISTING LAYERS
	for _, filename := range manifest[0].Layers[(len(manifest[0].Layers) - keepLayers):] {
		if err := l.AddLayer(filepath.Join(l.prevDir, filename)); err != nil {
			return err
		}
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

func (l *local) SetEnv(key, val string) error {
	if l.Inspect.Config == nil {
		return fmt.Errorf("failed to set env var, image '%s' does not exist", l.RepoName)
	}
	l.Inspect.Config.Env = append(l.Inspect.Config.Env, fmt.Sprintf("%s=%s", key, val))
	return nil
}

func (l *local) SetEntrypoint(ep ...string) error {
	if l.Inspect.Config == nil {
		return fmt.Errorf("failed to set entrypoint, image '%s' does not exist", l.RepoName)
	}
	l.Inspect.Config.Entrypoint = ep
	return nil
}

func (l *local) SetCmd(cmd ...string) error {
	if l.Inspect.Config == nil {
		return fmt.Errorf("failed to set cmd, image '%s' does not exist", l.RepoName)
	}
	l.Inspect.Config.Cmd = cmd
	return nil
}

func (l *local) TopLayer() (string, error) {
	all := l.Inspect.RootFS.Layers
	topLayer := all[len(all)-1]
	return topLayer, nil
}

func (l *local) GetLayer(sha string) (io.ReadCloser, error) {
	l.prevDownload()
	layerID, ok := l.prevMap[sha]
	if !ok {
		return nil, fmt.Errorf("image '%s' does not contain layer with diff ID '%s'", l.RepoName, sha)
	}
	return os.Open(filepath.Join(l.prevDir, layerID))
}

func (l *local) AddLayer(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "AddLayer: open layer: %s", path)
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return errors.Wrapf(err, "AddLayer: calculate checksum: %s", path)
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	l.Inspect.RootFS.Layers = append(l.Inspect.RootFS.Layers, "sha256:"+sha)
	l.layerPaths = append(l.layerPaths, path)
	l.easyAddLayers = nil

	return nil
}

func (l *local) ReuseLayer(sha string) error {
	if len(l.easyAddLayers) > 0 && l.easyAddLayers[0] == sha {
		l.Inspect.RootFS.Layers = append(l.Inspect.RootFS.Layers, sha)
		l.layerPaths = append(l.layerPaths, "")
		l.easyAddLayers = l.easyAddLayers[1:]
		return nil
	}

	if err := l.prevDownload(); err != nil {
		return err
	}

	reuseLayer, ok := l.prevMap[sha]
	if !ok {
		return fmt.Errorf("SHA %s was not found in %s", sha, l.RepoName)
	}

	return l.AddLayer(filepath.Join(l.prevDir, reuseLayer))
}

func (l *local) Save() (string, error) {
	ctx := context.Background()
	done := make(chan error)

	t, err := name.NewTag(l.RepoName, name.WeakValidation)
	if err != nil {
		return "", err
	}
	repoName := t.String()

	pr, pw := io.Pipe()
	defer pw.Close()
	go func() {
		res, err := l.Docker.ImageLoad(ctx, pr, true)
		if err != nil {
			done <- err
			return
		}
		defer res.Body.Close()
		io.Copy(ioutil.Discard, res.Body)

		done <- nil
	}()

	tw := tar.NewWriter(pw)
	defer tw.Close()

	configFile, err := l.configFile()
	if err != nil {
		return "", errors.Wrap(err, "generate config file")
	}

	imgID := fmt.Sprintf("%x", sha256.Sum256(configFile))
	if err := archive.AddTextToTar(tw, imgID+".json", configFile); err != nil {
		return "", err
	}

	var layerPaths []string
	for _, path := range l.layerPaths {
		if path == "" {
			layerPaths = append(layerPaths, "")
			continue
		}
		layerName := fmt.Sprintf("/%x.tar", sha256.Sum256([]byte(path)))
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if err := archive.AddFileToTar(tw, layerName, f); err != nil {
			return "", err
		}
		f.Close()
		layerPaths = append(layerPaths, layerName)

	}

	manifest, err := json.Marshal([]map[string]interface{}{
		{
			"Config":   imgID + ".json",
			"RepoTags": []string{repoName},
			"Layers":   layerPaths,
		},
	})
	if err != nil {
		return "", err
	}

	if err := archive.AddTextToTar(tw, "manifest.json", manifest); err != nil {
		return "", err
	}

	tw.Close()
	pw.Close()
	err = <-done

	if l.prevDir != "" {
		os.RemoveAll(l.prevDir)
		l.prevDir = ""
		l.prevMap = nil
		l.prevOnce = &sync.Once{}
	}

	if _, _, err = l.Docker.ImageInspectWithRaw(context.Background(), imgID); err != nil {
		if dockerclient.IsErrNotFound(err) {
			return "", fmt.Errorf("save image '%s'", l.RepoName)
		}
		return "", err
	}

	return imgID, err
}

func (l *local) configFile() ([]byte, error) {
	imgConfig := map[string]interface{}{
		"os":      "linux",
		"created": time.Now().Format(time.RFC3339),
		"config":  l.Inspect.Config,
		"rootfs": map[string][]string{
			"diff_ids": l.Inspect.RootFS.Layers,
		},
		"history": make([]struct{}, len(l.Inspect.RootFS.Layers)),
	}
	return json.Marshal(imgConfig)
}

func (l *local) Delete() error {
	if found, err := l.Found(); err != nil {
		return errors.Wrap(err, "determining image existence")
	} else if found {
		options := dockertypes.ImageRemoveOptions{
			Force:         true,
			PruneChildren: true,
		}
		_, err := l.Docker.ImageRemove(context.Background(), l.Inspect.ID, options)
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *local) prevDownload() error {
	var outerErr error
	l.prevOnce.Do(func() {
		ctx := context.Background()

		tarFile, err := l.Docker.ImageSave(ctx, []string{l.RepoName})
		if err != nil {
			outerErr = err
			return
		}
		defer tarFile.Close()

		l.prevDir, err = ioutil.TempDir("", "packs.local.reuse-layer.")
		if err != nil {
			outerErr = errors.Wrap(err, "local reuse-layer create temp dir")
			return
		}

		err = archive.Untar(tarFile, l.prevDir)
		if err != nil {
			outerErr = err
			return
		}

		mf, err := os.Open(filepath.Join(l.prevDir, "manifest.json"))
		if err != nil {
			outerErr = err
			return
		}
		defer mf.Close()

		var manifest []struct {
			Config string
			Layers []string
		}
		if err := json.NewDecoder(mf).Decode(&manifest); err != nil {
			outerErr = err
			return
		}

		if len(manifest) != 1 {
			outerErr = fmt.Errorf("manifest.json had unexpected number of entries: %d", len(manifest))
			return
		}

		df, err := os.Open(filepath.Join(l.prevDir, manifest[0].Config))
		if err != nil {
			outerErr = err
			return
		}
		defer df.Close()

		var details struct {
			RootFS struct {
				DiffIDs []string `json:"diff_ids"`
			} `json:"rootfs"`
		}

		if err = json.NewDecoder(df).Decode(&details); err != nil {
			outerErr = err
			return
		}

		if len(manifest[0].Layers) != len(details.RootFS.DiffIDs) {
			outerErr = fmt.Errorf("layers and diff IDs do not match, there are %d layers and %d diffIDs", len(manifest[0].Layers), len(details.RootFS.DiffIDs))
			return
		}

		l.prevMap = make(map[string]string, len(manifest[0].Layers))
		for i, diffID := range details.RootFS.DiffIDs {
			layerID := manifest[0].Layers[i]
			l.prevMap[diffID] = layerID
		}
	})
	return outerErr
}
