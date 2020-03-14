package registry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"

	"github.com/buildpacks/pack/internal/buildpack"
)

const defaultRegistryURL = "https://github.com/jkutner/buildpack-registry"

const defaultRegistyDir = "registry"

type Buildpack struct {
	Namespace string `json:"ns"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Yanked    bool   `json:"yanked"`
	Digest    string `json:"digest"`
	Address   string `json:"addr"`
}

type Entry struct {
	Buildpacks []Buildpack `json:"buildpacks"`
}

type RegistryCache struct {
	URL  string
	Root string
}

func NewRegistryCache(home, registryURL string) (RegistryCache, error) {
	r := RegistryCache{
		URL:  registryURL,
		Root: filepath.Join(home, defaultRegistyDir),
	}
	return r, r.Initialize()
}

func NewDefaultRegistryCache(home string) (RegistryCache, error) {
	return NewRegistryCache(home, defaultRegistryURL)
}

func (r *RegistryCache) Initialize() error {
	_, err := os.Stat(r.Root)
	if err != nil {
		if os.IsNotExist(err) {
			root, err := ioutil.TempDir("", "registry")
			if err != nil {
				return err
			}

			repository, err := git.PlainClone(root, false, &git.CloneOptions{
				URL: r.URL,
			})

			w, err := repository.Worktree()
			if err != nil {
				return err
			}

			return os.Rename(w.Filesystem.Root(), r.Root)
		}
	}
	// TODO validate the existing registry
	// TODO if the remote origin is != URL, reset the repo
	return err
}

func (r *RegistryCache) Refresh() error {
	repository, err := git.PlainOpen(r.Root)
	if err != nil {
		return err
	}

	w, err := repository.Worktree()
	if err != nil {
		return err
	}

	err = w.Pull(&git.PullOptions{RemoteName: "origin"})
	if err == git.NoErrAlreadyUpToDate {
		return nil
	} else {
		return err
	}
}

func (r *RegistryCache) readEntry(ns, name, version string) (Entry, error) {
	index := filepath.Join(r.Root, ns[:2], ns[2:4], fmt.Sprintf("%s_%s", ns, name))

	if _, err := os.Stat(index); err != nil {
		return Entry{}, errors.Wrapf(err, "could not find buildpack: %s/%s", ns, name)
	}

	file, err := os.Open(index)
	if err != nil {
		return Entry{}, errors.Wrapf(err, "could not open index for buildpack: %s/%s", ns, name)
	}
	defer file.Close()

	entry := Entry{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var bp Buildpack
		err = json.Unmarshal([]byte(scanner.Text()), &bp)
		if err != nil {
			return Entry{}, errors.Wrapf(err, "could not parse index for buildpack: %s/%s", ns, name)
		}

		entry.Buildpacks = append(entry.Buildpacks, bp)
	}

	if err := scanner.Err(); err != nil {
		return entry, errors.Wrapf(err, "could not read index for buildpack: %s/%s", ns, name)
	}

	return entry, nil
}

func (r *RegistryCache) LocateBuildpack(bp string) (Buildpack, error) {
	err := r.Refresh()
	if err != nil {
		return Buildpack{}, errors.Wrap(err, "refreshing cache")
	}

	ns, name, version, err := buildpack.ParseRegistryID(bp)
	if err != nil {
		return Buildpack{}, err
	}

	entry, err := r.readEntry(ns, name, version)
	if err != nil {
		return Buildpack{}, errors.Wrap(err, "reading entry")
	}

	if len(entry.Buildpacks) > 0 {
		if version == "" {
			// TODO check highest version?
			return entry.Buildpacks[0], nil
		}

		for _, bpIndex := range entry.Buildpacks {
			if bpIndex.Version == version {
				return bpIndex, nil
			}
		}
		return Buildpack{}, fmt.Errorf("could not find version for buildpack: %s", bp)
	}

	return Buildpack{}, fmt.Errorf("no entries for buildpack: %s", bp)
}
