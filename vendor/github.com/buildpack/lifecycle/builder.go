package lifecycle

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

type Builder struct {
	PlatformDir string
	LayersDir   string
	AppDir      string
	Env         BuildEnv
	Buildpacks  []*Buildpack
	Plan        Plan
	Out, Err    io.Writer
}

type BuildEnv interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string) error
	List() []string
}

type Process struct {
	Type    string `toml:"type"`
	Command string `toml:"command"`
}

type LaunchTOML struct {
	Processes []Process `toml:"processes"`
}

type Plan map[string]map[string]interface{}

type BuildMetadata struct {
	Processes  []Process `toml:"processes"`
	Buildpacks []string  `toml:"buildpacks"`
	BOM        Plan      `toml:"bom"`
}

func (b *Builder) Build() (*BuildMetadata, error) {
	platformDir, err := filepath.Abs(b.PlatformDir)
	if err != nil {
		return nil, err
	}
	layersDir, err := filepath.Abs(b.LayersDir)
	if err != nil {
		return nil, err
	}
	appDir, err := filepath.Abs(b.AppDir)
	if err != nil {
		return nil, err
	}
	planDir, err := ioutil.TempDir("", "plan.")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(planDir)

	procMap := processMap{}
	plan := copyPlan(b.Plan)
	bom := copyPlan(b.Plan)
	var buildpackIDs []string
	for _, bp := range b.Buildpacks {
		bpDirName := bp.EscapedID()
		bpLayersDir := filepath.Join(layersDir, bpDirName)
		bpPlanDir := filepath.Join(planDir, bpDirName)
		buildpackIDs = append(buildpackIDs, bpDirName)
		if err := os.MkdirAll(bpLayersDir, 0777); err != nil {
			return nil, err
		}

		if err := os.MkdirAll(bpPlanDir, 0777); err != nil {
			return nil, err
		}
		bpPlanPath := filepath.Join(bpPlanDir, "plan.toml")
		if ioutil.WriteFile(bpPlanPath, nil, 0777); err != nil {
			return nil, err
		}
		planIn := &bytes.Buffer{}
		if err := toml.NewEncoder(planIn).Encode(plan); err != nil {
			return nil, err
		}
		buildPath, err := filepath.Abs(filepath.Join(bp.Dir, "bin", "build"))
		if err != nil {
			return nil, err
		}
		cmd := exec.Command(buildPath, bpLayersDir, platformDir, bpPlanPath)
		cmd.Env = b.Env.List()
		cmd.Dir = appDir
		cmd.Stdin = planIn
		cmd.Stdout = b.Out
		cmd.Stderr = b.Err
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		if err := setupEnv(b.Env, bpLayersDir); err != nil {
			return nil, err
		}
		if err := consumePlan(bpPlanPath, plan, bom); err != nil {
			return nil, err
		}
		var launch LaunchTOML
		tomlPath := filepath.Join(bpLayersDir, "launch.toml")
		if _, err := toml.DecodeFile(tomlPath, &launch); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		procMap.add(launch.Processes)
	}

	return &BuildMetadata{
		Processes:  procMap.list(),
		Buildpacks: buildpackIDs,
		BOM:        bom,
	}, nil
}

func setupEnv(env BuildEnv, layersDir string) error {
	if err := eachDir(layersDir, func(path string) error {
		if !isBuild(path + ".toml") {
			return nil
		}
		return env.AddRootDir(path)
	}); err != nil {
		return err
	}

	return eachDir(layersDir, func(path string) error {
		if !isBuild(path + ".toml") {
			return nil
		}
		if err := env.AddEnvDir(filepath.Join(path, "env")); err != nil {
			return err
		}
		return env.AddEnvDir(filepath.Join(path, "env.build"))
	})
}

func isBuild(path string) bool {
	var layerTOML struct {
		Build bool `toml:"build"`
	}
	_, err := toml.DecodeFile(path, &layerTOML)
	return err == nil && layerTOML.Build
}

func consumePlan(path string, plan, bom Plan) error {
	var input map[string]map[string]interface{}
	if _, err := toml.DecodeFile(path, &input); err != nil {
		return err
	}
	for k, v := range input {
		delete(plan, k)
		if len(v) > 0 {
			bom[k] = v
		}
	}
	return nil
}

type processMap map[string]Process

func (m processMap) add(l []Process) {
	for _, proc := range l {
		m[proc.Type] = proc
	}
}

func (m processMap) list() []Process {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	procs := []Process{}
	for _, key := range keys {
		procs = append(procs, m[key])
	}
	return procs
}

func copyPlan(m Plan) Plan {
	out := Plan{}
	for k, v := range m {
		out[k] = v
	}
	return out
}
