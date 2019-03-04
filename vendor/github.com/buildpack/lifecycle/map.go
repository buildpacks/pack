package lifecycle

import (
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

type BuildpackMap map[string]*Buildpack

type buildpackTOML struct {
	Buildpack struct {
		ID      string `toml:"id"`
		Version string `toml:"version"`
		Name    string `toml:"name"`
	} `toml:"buildpack"`
}

func NewBuildpackMap(dir string) (BuildpackMap, error) {
	buildpacks := BuildpackMap{}
	glob := filepath.Join(dir, "*", "*", "buildpack.toml")
	files, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		buildpackDir := filepath.Dir(file)
		_, version := filepath.Split(buildpackDir)
		var bpTOML buildpackTOML
		if _, err := toml.DecodeFile(file, &bpTOML); err != nil {
			return nil, err
		}
		buildpacks[bpTOML.Buildpack.ID+"@"+version] = &Buildpack{
			ID:      bpTOML.Buildpack.ID,
			Version: bpTOML.Buildpack.Version,
			Name:    bpTOML.Buildpack.Name,
			Dir:     buildpackDir,
		}
	}
	return buildpacks, nil
}

func (m BuildpackMap) lookup(l []*Buildpack) ([]*Buildpack, error) {
	out := make([]*Buildpack, 0, len(l))
	for _, b := range l {
		ref := b.ID + "@" + b.Version
		if b.Version == "" {
			ref += "latest"
		}
		if bp, ok := m[ref]; ok {
			bp := *bp
			bp.Optional = b.Optional
			out = append(out, &bp)
		} else {
			return nil, fmt.Errorf("buildpack '%s' missing from image", ref)
		}
	}
	return out, nil
}

func (m BuildpackMap) ReadOrder(orderPath string) (BuildpackOrder, error) {
	var order struct {
		Groups BuildpackOrder `toml:"groups"`
	}
	if _, err := toml.DecodeFile(orderPath, &order); err != nil {
		return nil, err
	}

	var groups BuildpackOrder
	for _, g := range order.Groups {
		group, err := m.lookup(g.Buildpacks)
		if err != nil {
			return nil, errors.Wrap(err, "lookup buildpacks")
		}
		groups = append(groups, BuildpackGroup{
			Buildpacks: group,
		})
	}
	return groups, nil
}

func (g *BuildpackGroup) Write(path string) error {
	data := struct {
		Buildpacks []*Buildpack `toml:"buildpacks"`
	}{
		Buildpacks: g.Buildpacks,
	}
	return WriteTOML(path, data)
}

func (m BuildpackMap) ReadGroup(path string) (*BuildpackGroup, error) {
	var group BuildpackGroup
	var err error
	if _, err := toml.DecodeFile(path, &group); err != nil {
		return nil, err
	}
	group.Buildpacks, err = m.lookup(group.Buildpacks)
	if err != nil {
		return nil, errors.Wrap(err, "lookup buildpacks")
	}
	return &group, nil
}
