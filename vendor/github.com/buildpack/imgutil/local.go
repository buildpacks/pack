package imgutil

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
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
)

type localImage struct {
	repoName         string
	docker           *client.Client
	inspect          types.ImageInspect
	layerPaths       []string
	currentTempImage string
	prevDir          string
	prevMap          map[string]string
	prevOnce         *sync.Once
	easyAddLayers    []string
}

func EmptyLocalImage(repoName string, dockerClient *client.Client) Image {
	inspect := types.ImageInspect{}
	inspect.Config = &container.Config{
		Labels: map[string]string{},
	}
	return &localImage{
		repoName: repoName,
		docker:   dockerClient,
		inspect:  inspect,
		prevOnce: &sync.Once{},
	}
}

func NewLocalImage(repoName string, dockerClient *client.Client) (Image, error) {
	inspect, _, err := dockerClient.ImageInspectWithRaw(context.Background(), repoName)
	if err != nil && !client.IsErrNotFound(err) {
		return nil, err
	}

	return &localImage{
		docker:     dockerClient,
		repoName:   repoName,
		inspect:    inspect,
		layerPaths: make([]string, len(inspect.RootFS.Layers)),
		prevOnce:   &sync.Once{},
	}, nil
}

func (l *localImage) Label(key string) (string, error) {
	if l.inspect.Config == nil {
		return "", fmt.Errorf("failed to get label, image '%s' does not exist", l.repoName)
	}
	labels := l.inspect.Config.Labels
	return labels[key], nil
}

func (l *localImage) Env(key string) (string, error) {
	if l.inspect.Config == nil {
		return "", fmt.Errorf("failed to get env var, image '%s' does not exist", l.repoName)
	}
	for _, envVar := range l.inspect.Config.Env {
		parts := strings.Split(envVar, "=")
		if parts[0] == key {
			return parts[1], nil
		}
	}
	return "", nil
}

func (l *localImage) Rename(name string) {
	l.easyAddLayers = nil
	if prevInspect, _, err := l.docker.ImageInspectWithRaw(context.TODO(), name); err == nil {
		if l.sameBase(prevInspect) {
			l.easyAddLayers = prevInspect.RootFS.Layers[len(l.inspect.RootFS.Layers):]
		}
	}

	l.repoName = name
}

func (l *localImage) sameBase(prevInspect types.ImageInspect) bool {
	if len(prevInspect.RootFS.Layers) < len(l.inspect.RootFS.Layers) {
		return false
	}
	for i, baseLayer := range l.inspect.RootFS.Layers {
		if baseLayer != prevInspect.RootFS.Layers[i] {
			return false
		}
	}
	return true
}

func (l *localImage) Name() string {
	return l.repoName
}

func (l *localImage) Found() (bool, error) {
	return l.inspect.Config != nil, nil
}

func (l *localImage) Digest() (string, error) {
	if found, err := l.Found(); err != nil {
		return "", errors.Wrap(err, "determining image existence")
	} else if !found {
		return "", fmt.Errorf("failed to get digest, image '%s' does not exist", l.repoName)
	}
	if len(l.inspect.RepoDigests) == 0 {
		return "", nil
	}
	parts := strings.Split(l.inspect.RepoDigests[0], "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("failed to get digest, image '%s' has malformed digest '%s'", l.repoName, l.inspect.RepoDigests[0])
	}
	return parts[1], nil
}

func (l *localImage) CreatedAt() (time.Time, error) {
	createdAtTime := l.inspect.Created
	createdTime, err := time.Parse(time.RFC3339Nano, createdAtTime)

	if err != nil {
		return time.Time{}, err
	}
	return createdTime, nil
}

func (l *localImage) Rebase(baseTopLayer string, newBase Image) error {
	ctx := context.Background()

	// FIND TOP LAYER
	keepLayers := -1
	for i, diffID := range l.inspect.RootFS.Layers {
		if diffID == baseTopLayer {
			keepLayers = len(l.inspect.RootFS.Layers) - i - 1
			break
		}
	}
	if keepLayers == -1 {
		return fmt.Errorf("'%s' not found in '%s' during rebase", baseTopLayer, l.repoName)
	}

	// SWITCH BASE LAYERS
	newBaseInspect, _, err := l.docker.ImageInspectWithRaw(ctx, newBase.Name())
	if err != nil {
		return errors.Wrap(err, "analyze read previous image config")
	}
	l.inspect.RootFS.Layers = newBaseInspect.RootFS.Layers
	l.layerPaths = make([]string, len(l.inspect.RootFS.Layers))

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

func (l *localImage) SetLabel(key, val string) error {
	if l.inspect.Config == nil {
		return fmt.Errorf("failed to set label, image '%s' does not exist", l.repoName)
	}
	l.inspect.Config.Labels[key] = val
	return nil
}

func (l *localImage) SetEnv(key, val string) error {
	if l.inspect.Config == nil {
		return fmt.Errorf("failed to set env var, image '%s' does not exist", l.repoName)
	}
	l.inspect.Config.Env = append(l.inspect.Config.Env, fmt.Sprintf("%s=%s", key, val))
	return nil
}

func (l *localImage) SetWorkingDir(dir string) error {
	if l.inspect.Config == nil {
		return fmt.Errorf("failed to set working dir, image '%s' does not exist", l.repoName)
	}
	l.inspect.Config.WorkingDir = dir
	return nil
}

func (l *localImage) SetEntrypoint(ep ...string) error {
	if l.inspect.Config == nil {
		return fmt.Errorf("failed to set entrypoint, image '%s' does not exist", l.repoName)
	}
	l.inspect.Config.Entrypoint = ep
	return nil
}

func (l *localImage) SetCmd(cmd ...string) error {
	if l.inspect.Config == nil {
		return fmt.Errorf("failed to set cmd, image '%s' does not exist", l.repoName)
	}
	l.inspect.Config.Cmd = cmd
	return nil
}

func (l *localImage) TopLayer() (string, error) {
	all := l.inspect.RootFS.Layers
	topLayer := all[len(all)-1]
	return topLayer, nil
}

func (l *localImage) GetLayer(sha string) (io.ReadCloser, error) {
	if err := l.prevDownload(); err != nil {
		return nil, err
	}

	layerID, ok := l.prevMap[sha]
	if !ok {
		return nil, fmt.Errorf("image '%s' does not contain layer with diff ID '%s'", l.repoName, sha)
	}
	return os.Open(filepath.Join(l.prevDir, layerID))
}

func (l *localImage) AddLayer(path string) error {
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

	l.inspect.RootFS.Layers = append(l.inspect.RootFS.Layers, "sha256:"+sha)
	l.layerPaths = append(l.layerPaths, path)
	l.easyAddLayers = nil

	return nil
}

func (l *localImage) ReuseLayer(sha string) error {
	if len(l.easyAddLayers) > 0 && l.easyAddLayers[0] == sha {
		l.inspect.RootFS.Layers = append(l.inspect.RootFS.Layers, sha)
		l.layerPaths = append(l.layerPaths, "")
		l.easyAddLayers = l.easyAddLayers[1:]
		return nil
	}

	if err := l.prevDownload(); err != nil {
		return err
	}

	reuseLayer, ok := l.prevMap[sha]
	if !ok {
		return fmt.Errorf("SHA %s was not found in %s", sha, l.repoName)
	}

	return l.AddLayer(filepath.Join(l.prevDir, reuseLayer))
}

func (l *localImage) Save() (string, error) {
	ctx := context.Background()
	done := make(chan error)

	t, err := name.NewTag(l.repoName, name.WeakValidation)
	if err != nil {
		return "", err
	}
	repoName := t.String()

	pr, pw := io.Pipe()
	defer pw.Close()
	go func() {
		res, err := l.docker.ImageLoad(ctx, pr, true)
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
	if err := addTextToTar(tw, imgID+".json", configFile); err != nil {
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
		if err := addFileToTar(tw, layerName, f); err != nil {
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

	if err := addTextToTar(tw, "manifest.json", manifest); err != nil {
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

	if _, _, err = l.docker.ImageInspectWithRaw(context.Background(), imgID); err != nil {
		if client.IsErrNotFound(err) {
			return "", fmt.Errorf("save image '%s'", l.repoName)
		}
		return "", err
	}

	return imgID, err
}

func (l *localImage) configFile() ([]byte, error) {
	imgConfig := map[string]interface{}{
		"os":      "linux",
		"created": time.Now().Format(time.RFC3339),
		"config":  l.inspect.Config,
		"rootfs": map[string][]string{
			"diff_ids": l.inspect.RootFS.Layers,
		},
		"history": make([]struct{}, len(l.inspect.RootFS.Layers)),
	}
	return json.Marshal(imgConfig)
}

func (l *localImage) Delete() error {
	if found, err := l.Found(); err != nil {
		return errors.Wrap(err, "determining image existence")
	} else if found {
		options := types.ImageRemoveOptions{
			Force:         true,
			PruneChildren: true,
		}
		_, err := l.docker.ImageRemove(context.Background(), l.inspect.ID, options)
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *localImage) prevDownload() error {
	var outerErr error
	l.prevOnce.Do(func() {
		ctx := context.Background()

		tarFile, err := l.docker.ImageSave(ctx, []string{l.repoName})
		if err != nil {
			outerErr = err
			return
		}
		defer tarFile.Close()

		l.prevDir, err = ioutil.TempDir("", "imgutil.local.reuse-layer.")
		if err != nil {
			outerErr = errors.Wrap(err, "local reuse-layer create temp dir")
			return
		}

		err = untar(tarFile, l.prevDir)
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

func addTextToTar(tw *tar.Writer, name string, contents []byte) error {
	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(contents))}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(contents)
	return err
}

func addFileToTar(tw *tar.Writer, name string, contents *os.File) error {
	fi, err := contents.Stat()
	if err != nil {
		return err
	}
	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(fi.Size())}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, contents)
	return err
}

func untar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			return nil
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dest, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			_, err := os.Stat(filepath.Dir(path))
			if os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return err
				}
			}

			fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, hdr.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(fh, tr); err != nil {
				fh.Close()
				return err
			}
			fh.Close()
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, path); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown file type in tar %d", hdr.Typeflag)
		}
	}
}
