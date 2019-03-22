package lifecycle

import (
	"encoding/json"
	"github.com/google/go-containerregistry/pkg/name"
	"os"
	"path/filepath"
	"regexp"
)

func SetupCredHelpers(refs ...string) error {
	dockerPath := filepath.Join(os.Getenv("HOME"), ".docker")
	configPath := filepath.Join(dockerPath, "config.json")
	config := map[string]interface{}{}
	credHelpers := map[string]string{}
	config["credHelpers"] = credHelpers
	if f, err := os.Open(configPath); err == nil {
		err := json.NewDecoder(f).Decode(&config)
		if f.Close(); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	added := false
	for _, refStr := range refs {
		ref, err := name.ParseReference(refStr, name.WeakValidation)
		if err != nil {
			return err
		}
		registry := ref.Context().RegistryStr()
		for _, ch := range []struct {
			domain string
			helper string
		}{
			{"([.]|^)gcr[.]io$", "gcr"},
			{"[.]amazonaws[.]", "ecr-login"},
			{"([.]|^)azurecr[.]io$", "acr"},
		} {
			match, err := regexp.MatchString("(?i)"+ch.domain, registry)
			if err != nil || !match {
				continue
			}
			credHelpers[registry] = ch.helper
			added = true
		}
	}
	if !added {
		return nil
	}
	if err := os.MkdirAll(dockerPath, 0777); err != nil {
		return err
	}
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(config)
}
