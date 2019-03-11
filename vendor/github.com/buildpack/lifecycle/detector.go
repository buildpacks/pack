package lifecycle

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/BurntSushi/toml"
)

const (
	CodeDetectPass = iota
	CodeDetectError
	CodeDetectFail = 100
)

type Buildpack struct {
	ID       string `toml:"id"`
	Version  string `toml:"version"`
	Optional bool   `toml:"optional,omitempty"`
	Name     string `toml:"-"`
	Dir      string `toml:"-"`
}

type DetectConfig struct {
	AppDir      string
	PlatformDir string
	Out, Err    *log.Logger
}

func (bp *Buildpack) EscapedID() string {
	return escapeIdentifier(bp.ID)
}

func (bp *Buildpack) Detect(c *DetectConfig, in io.Reader, out io.Writer) int {
	detectPath, err := filepath.Abs(filepath.Join(bp.Dir, "bin", "detect"))
	if err != nil {
		c.Err.Print("Error: ", err)
		return CodeDetectError
	}
	appDir, err := filepath.Abs(c.AppDir)
	if err != nil {
		c.Err.Print("Error: ", err)
		return CodeDetectError
	}
	platformDir, err := filepath.Abs(c.PlatformDir)
	if err != nil {
		c.Err.Print("Error: ", err)
		return CodeDetectError
	}
	planDir, err := ioutil.TempDir("", filepath.Base(bp.Dir)+".plan.")
	if err != nil {
		c.Err.Print("Error: ", err)
		return CodeDetectError
	}
	defer os.RemoveAll(planDir)
	planPath := filepath.Join(planDir, "plan.toml")
	if ioutil.WriteFile(planPath, nil, 0777); err != nil {
		c.Err.Print("Error: ", err)
		return CodeDetectError
	}
	log := &bytes.Buffer{}
	defer func() {
		if log.Len() > 0 {
			c.Out.Printf("======== Output: %s ========\n%s", bp.Name, log)
		}
	}()
	cmd := exec.Command(detectPath, platformDir, planPath)
	cmd.Dir = appDir
	cmd.Stdin = in
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				return status.ExitStatus()
			}
		}
		c.Err.Print("Error: ", err)
		return CodeDetectError
	}
	if err := parsePlan(out, planPath); err != nil {
		c.Err.Print("Error: ", err)
		return CodeDetectError
	}
	return CodeDetectPass
}

func parsePlan(out io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(out, f)
	return err
}

type BuildpackGroup struct {
	Buildpacks []*Buildpack `toml:"buildpacks"`
}

func (bg *BuildpackGroup) Detect(c *DetectConfig) (plan []byte, group *BuildpackGroup, ok bool) {
	group = &BuildpackGroup{}
	detected := true
	c.Out.Printf("Trying group of %d...", len(bg.Buildpacks))
	plan, codes := bg.pDetect(c)
	c.Out.Printf("======== Results ========")
	for i, code := range codes {
		name := bg.Buildpacks[i].Name
		optional := bg.Buildpacks[i].Optional
		switch code {
		case CodeDetectPass:
			c.Out.Printf("%s: pass", name)
			group.Buildpacks = append(group.Buildpacks, bg.Buildpacks[i])
		case CodeDetectFail:
			if optional {
				c.Out.Printf("%s: skip", name)
			} else {
				c.Out.Printf("%s: fail", name)
			}
			detected = detected && optional
		default:
			c.Out.Printf("%s: error (%d)", name, code)
			detected = detected && optional
		}
	}
	detected = detected && len(group.Buildpacks) > 0
	return plan, group, detected
}

func (bg *BuildpackGroup) pDetect(c *DetectConfig) (plan []byte, codes []int) {
	codes = make([]int, len(bg.Buildpacks))
	wg := sync.WaitGroup{}
	defer wg.Wait()
	wg.Add(len(bg.Buildpacks))
	var lastIn io.ReadCloser
	for i := range bg.Buildpacks {
		in, out := io.Pipe()
		go func(i int, last io.ReadCloser) {
			defer wg.Done()
			defer out.Close()
			add := &bytes.Buffer{}
			if last != nil {
				defer last.Close()
				orig := &bytes.Buffer{}
				last := io.TeeReader(last, orig)
				codes[i] = bg.Buildpacks[i].Detect(c, last, add)
				io.Copy(ioutil.Discard, last)
				if codes[i] == CodeDetectPass {
					mergeTOML(c.Err, out, orig, add)
				} else {
					mergeTOML(c.Err, out, orig)
				}
			} else {
				codes[i] = bg.Buildpacks[i].Detect(c, nil, add)
				if codes[i] == CodeDetectPass {
					mergeTOML(c.Err, out, add)
				}
			}
		}(i, lastIn)
		lastIn = in
	}
	if lastIn != nil {
		defer lastIn.Close()
		if p, err := ioutil.ReadAll(lastIn); err != nil {
			c.Err.Print("Warning: ", err)
		} else {
			plan = p
		}
	}
	return plan, codes
}

func mergeTOML(l *log.Logger, out io.Writer, in ...io.Reader) {
	result := map[string]interface{}{}
	for _, r := range in {
		var m map[string]interface{}
		if _, err := toml.DecodeReader(r, &m); err != nil {
			l.Print("Warning: ", err)
			continue
		}
		for k, v := range m {
			result[k] = v
		}
	}
	if err := toml.NewEncoder(out).Encode(result); err != nil {
		l.Print("Warning: ", err)
	}
}

type BuildpackOrder []BuildpackGroup

func (bo BuildpackOrder) Detect(c *DetectConfig) (plan []byte, group *BuildpackGroup) {
	for i := range bo {
		if p, g, ok := bo[i].Detect(c); ok {
			return p, g
		}
	}
	return nil, nil
}
