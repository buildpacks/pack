package pack

import (
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/image"
)

type BuilderTOML struct {
	Buildpacks []struct {
		ID     string `toml:"id"`
		URI    string `toml:"uri"`
		Latest bool   `toml:"latest"`
	} `toml:"buildpacks"`
	Groups []lifecycle.BuildpackGroup `toml:"groups"`
}

type BuilderConfig struct {
	Buildpacks []Buildpack
	Groups     []lifecycle.BuildpackGroup
	Repo       image.Image
	BuilderDir string //original location of builder.toml, used for interpreting relative paths in buildpack URIs
}
type Buildpack struct {
	ID     string
	Dir    string
	Latest bool
}

type BuilderFactory struct {
	Log          *log.Logger
	FS           FS
	Config       *config.Config
	ImageFactory ImageFactory
}

type CreateBuilderFlags struct {
	RepoName        string
	BuilderTomlPath string
	StackID         string
	Publish         bool
	NoPull          bool
}

func (f *BuilderFactory) BuilderConfigFromFlags(flags CreateBuilderFlags) (BuilderConfig, error) {
	baseImage, err := f.baseImageName(flags.StackID, flags.RepoName)
	if err != nil {
		return BuilderConfig{}, err
	}

	builderConfig := BuilderConfig{}
	builderConfig.BuilderDir = filepath.Dir(flags.BuilderTomlPath)
	if flags.Publish {
		builderConfig.Repo, err = f.ImageFactory.NewRemote(baseImage)
	} else {
		builderConfig.Repo, err = f.ImageFactory.NewLocal(baseImage, !flags.NoPull)
	}
	if err != nil {
		return BuilderConfig{}, errors.Wrapf(err, "opening base image: %s", baseImage)
	}
	builderConfig.Repo.Rename(flags.RepoName)

	builderTOML := &BuilderTOML{}
	_, err = toml.DecodeFile(flags.BuilderTomlPath, &builderTOML)
	if err != nil {
		return BuilderConfig{}, fmt.Errorf(`failed to decode builder config from file "%s": %s`, flags.BuilderTomlPath, err)
	}
	builderConfig.Groups = builderTOML.Groups

	for _, b := range builderTOML.Buildpacks {
		bp, err := f.resolveBuildpackURI(builderConfig.BuilderDir, b)
		if err != nil {
			return BuilderConfig{}, err
		}
		builderConfig.Buildpacks = append(builderConfig.Buildpacks, bp)
	}
	return builderConfig, nil
}

func (f *BuilderFactory) resolveBuildpackURI(builderDir string, b struct {
	ID     string `toml:"id"`
	URI    string `toml:"uri"`
	Latest bool   `toml:"latest"`
}) (Buildpack, error) {

	var dir string

	asurl, err := url.Parse(b.URI)
	if err != nil {
		return Buildpack{}, err
	}
	switch asurl.Scheme {
	case "", // This is the only way to support relative filepaths
		"file": // URIs with file:// protocol force the use of absolute paths. Host=localhost may be implied with file:///

		path := asurl.Path

		if !asurl.IsAbs() && !filepath.IsAbs(path) {
			path = filepath.Join(builderDir, path)
		}

		if filepath.Ext(path) == ".tgz" {
			file, err := os.Open(path)
			if err != nil {
				return Buildpack{}, errors.Wrapf(err, "could not open file to untar: %q", path)
			}
			defer file.Close()
			tmpDir, err := ioutil.TempDir("", fmt.Sprintf("create-builder-%s-", b.ID))
			if err != nil {
				return Buildpack{}, fmt.Errorf(`failed to create temporary directory: %s`, err)
			}
			if err = f.untarZ(file, tmpDir); err != nil {
				return Buildpack{}, err
			}
			dir = tmpDir
		} else {
			dir = path
		}
	case "http", "https":
		uriDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(b.URI)))
		cachedDir := filepath.Join(f.Config.Path(), "dl-cache", uriDigest)
		_, err := os.Stat(cachedDir)
		if os.IsNotExist(err) {
			if err = os.MkdirAll(cachedDir, 0744); err != nil {
				return Buildpack{}, err
			}
		}
		etagFile := cachedDir + ".etag"
		bytes, err := ioutil.ReadFile(etagFile)
		etag := ""
		if err == nil {
			etag = string(bytes)
		}

		reader, etag, err := f.downloadAsStream(b.URI, etag)
		if err != nil {
			return Buildpack{}, errors.Wrapf(err, "failed to download from %q", b.URI)
		} else if reader == nil {
			// can use cached content
			dir = cachedDir
			break
		} else {
			if err = f.untarZ(reader, cachedDir); err != nil {
				return Buildpack{}, err
			}
			if err = ioutil.WriteFile(etagFile, []byte(etag), 0744); err != nil {
				return Buildpack{}, err
			}
		}

		dir = cachedDir
	default:
		return Buildpack{}, fmt.Errorf("unsupported protocol in uri %q", b.URI)
	}

	return Buildpack{
		ID:     b.ID,
		Latest: b.Latest,
		Dir:    dir,
	}, nil
}

func (f *BuilderFactory) baseImageName(stackID, repoName string) (string, error) {
	stack, err := f.Config.Get(stackID)
	if err != nil {
		return "", err
	}
	if len(stack.BuildImages) == 0 {
		return "", fmt.Errorf(`Invalid stack: stack "%s" requires at least one build image`, stack.ID)
	}
	registry, err := config.Registry(repoName)
	if err != nil {
		return "", err
	}
	return config.ImageByRegistry(registry, stack.BuildImages)
}

func (f *BuilderFactory) Create(config BuilderConfig) error {
	tmpDir, err := ioutil.TempDir("", "create-builder") // TODO
	if err != nil {
		return fmt.Errorf(`failed to create temporary directory: %s`, err)
	}
	defer os.RemoveAll(tmpDir)

	orderTar, err := f.orderLayer(tmpDir, config.Groups)
	if err != nil {
		return fmt.Errorf(`failed generate order.toml layer: %s`, err)
	}
	if err := config.Repo.AddLayer(orderTar); err != nil {
		return fmt.Errorf(`failed append order.toml layer to image: %s`, err)
	}
	for _, buildpack := range config.Buildpacks {
		tarFile, err := f.buildpackLayer(tmpDir, buildpack, config.BuilderDir)
		if err != nil {
			return fmt.Errorf(`failed generate layer for buildpack "%s": %s`, buildpack.ID, err)
		}
		if err := config.Repo.AddLayer(tarFile); err != nil {
			return fmt.Errorf(`failed append buildpack layer to image: %s`, err)
		}
	}
	tarFile, err := f.latestLayer(config.Buildpacks, tmpDir, config.BuilderDir)
	if err != nil {
		return fmt.Errorf(`failed generate layer for latest links: %s`, err)
	}
	if err := config.Repo.AddLayer(tarFile); err != nil {
		return fmt.Errorf(`failed append latest link layer to image: %s`, err)
	}

	if _, err := config.Repo.Save(); err != nil {
		return err
	}

	f.Log.Println("Successfully created builder image:", config.Repo.Name())
	f.Log.Println("")
	f.Log.Println(`Tip: Run "pack build <image name> --builder <builder image> --path <app source code>" to use this builder`)

	return nil
}

type order struct {
	Groups []lifecycle.BuildpackGroup `toml:"groups"`
}

func (f *BuilderFactory) orderLayer(dest string, groups []lifecycle.BuildpackGroup) (layerTar string, err error) {
	buildpackDir := filepath.Join(dest, "buildpack")
	err = os.Mkdir(buildpackDir, 0755)
	if err != nil {
		return "", err
	}

	orderFile, err := os.Create(filepath.Join(buildpackDir, "order.toml"))
	if err != nil {
		return "", err
	}
	defer orderFile.Close()
	err = toml.NewEncoder(orderFile).Encode(order{Groups: groups})
	if err != nil {
		return "", err
	}
	layerTar = filepath.Join(dest, "order.tar")
	if err := f.FS.CreateTarFile(layerTar, buildpackDir, "/buildpacks", 0, 0); err != nil {
		return "", err
	}
	return layerTar, nil
}

type BuildpackData struct {
	BP struct {
		ID      string `toml:"id"`
		Version string `toml:"version"`
	} `toml:"buildpack"`
}

// buildpackLayer creates and returns the location of a tgz file for a buildpack layer. That file will reside in the `dest` directory.
// The tgz file is either created from an initially local directory, or it is downloaded (and validated) from
// a remote location if the buildpack uri uses the http(s) protocol.
func (f *BuilderFactory) buildpackLayer(dest string, buildpack Buildpack, builderDir string) (layerTar string, err error) {
	dir := buildpack.Dir

	data, err := f.buildpackData(buildpack, dir)
	if err != nil {
		return "", err
	}
	bp := data.BP
	if buildpack.ID != bp.ID {
		return "", fmt.Errorf("buildpack ids did not match: %s != %s", buildpack.ID, bp.ID)
	}
	if bp.Version == "" {
		return "", fmt.Errorf("buildpack.toml must provide version: %s", filepath.Join(buildpack.Dir, "!/buildpack.toml"))
	}

	tarFile := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", buildpack.ID, bp.Version))
	if err := f.FS.CreateTarFile(tarFile, dir, filepath.Join("/buildpacks", buildpack.ID, bp.Version), 0, 0); err != nil {
		return "", err
	}
	return tarFile, err
}

func (f *BuilderFactory) buildpackData(buildpack Buildpack, dir string) (*BuildpackData, error) {
	data := &BuildpackData{}
	_, err := toml.DecodeFile(filepath.Join(dir, "buildpack.toml"), &data)
	if err != nil {
		return nil, errors.Wrapf(err, "reading buildpack.toml from buildpack: %s", dir)
	}
	return data, nil
}

func (f *BuilderFactory) latestLayer(buildpacks []Buildpack, dest, builderDir string) (string, error) {
	tmpDir, err := ioutil.TempDir(dest, "create-builder-latest")
	if err != nil {
		return "", err
	}
	for _, bp := range buildpacks {
		if bp.Latest {
			data, err := f.buildpackData(bp, bp.Dir)
			if err != nil {
				return "", err
			}
			err = os.Mkdir(filepath.Join(tmpDir, bp.ID), 0755)
			if err != nil {
				return "", err
			}
			err = os.Symlink(filepath.Join("/", "buildpacks", bp.ID, data.BP.Version), filepath.Join(tmpDir, bp.ID, "latest"))
			if err != nil {
				fmt.Println("E")
			}
		}
	}
	tarFile := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", "latest", "buildpacks"))
	if err := f.FS.CreateTarFile(tarFile, tmpDir, filepath.Join("/buildpacks"), 0, 0); err != nil {
		return "", err
	}
	return tarFile, err
}

func (f *BuilderFactory) downloadAsStream(uri string, etag string) (io.Reader, string, error) {
	c := http.Client{}
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, "", err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if resp, err := c.Do(req); err != nil {
		return nil, "", err
	} else {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			f.Log.Printf("Downloading from %q\n", uri)
			return resp.Body, resp.Header.Get("Etag"), nil
		} else if resp.StatusCode == 304 {
			f.Log.Printf("Using cached version of %q\n", uri)
			return nil, etag, nil
		} else {
			return nil, "", fmt.Errorf("could not download from %q, code http status %d", uri, resp.StatusCode)
		}
	}
}

func (f *BuilderFactory) untarZ(r io.Reader, dir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return errors.Wrapf(err, "could not unzip")
	}
	defer gzr.Close()
	return f.FS.Untar(gzr, dir)
}
