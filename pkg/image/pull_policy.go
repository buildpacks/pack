package image

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"github.com/buildpacks/imgutil/local"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/logging"
)

// PullPolicy defines a policy for how to manage images
type PullPolicy int

const (
	PRUNE_TIME = 7 * 24 * 60 * time.Minute
	HOURLY     = 1 * 60 * time.Minute
	DAILY      = 1 * 24 * 60 * time.Minute
	WEEKLY     = 7 * 24 * 60 * time.Minute
)

type ImagePullPolicyManager struct {
	Logger logging.Logger
}

const (
	// Always pull images, even if they are present
	PullAlways PullPolicy = iota
	// Pull images hourly
	PullHourly
	// Pull images daily
	PullDaily
	// Pull images weekly
	PullWeekly
	// Never pull images, even if they are not present
	PullNever
	// PullIfNotPresent pulls images if they aren't present
	PullIfNotPresent
)

type Interval struct {
	LastPrune string `json:"last_prune"`
}

type ImageData struct {
	ImageIDtoTIME map[string]string
}

type ImageJSON struct {
	Interval *Interval  `json:"interval"`
	Image    *ImageData `json:"image"`
}

func DefaultImageJSONPath() (string, error) {
	home, err := config.PackHome()
	if err != nil {
		return "", errors.Wrap(err, "getting pack home")
	}
	return filepath.Join(home, "image.json"), nil
}

var nameMap = map[string]PullPolicy{"always": PullAlways, "hourly": PullHourly, "daily": PullDaily, "weekly": PullWeekly, "never": PullNever, "if-not-present": PullIfNotPresent, "": PullAlways}

// ParsePullPolicy from string with support for interval formats
func (i *ImagePullPolicyManager) ParsePullPolicy(policy string) (PullPolicy, error) {
	if val, ok := nameMap[policy]; ok {
		return val, nil
	}

	return PullAlways, errors.Errorf("invalid pull policy %s", policy)
}

func (p PullPolicy) String() string {
	switch p {
	case PullAlways:
		return "always"
	case PullHourly:
		return "hourly"
	case PullDaily:
		return "daily"
	case PullWeekly:
		return "weekly"
	case PullNever:
		return "never"
	case PullIfNotPresent:
		return "if-not-present"
	}

	return ""
}

func (i *ImagePullPolicyManager) GetDuration(p PullPolicy) time.Duration {
	switch p {
	case PullHourly:
		return HOURLY
	case PullDaily:
		return DAILY
	case PullWeekly:
		return WEEKLY
	}

	return 0
}

func (i *ImagePullPolicyManager) PruneOldImages(docker DockerClient) error {
	path, err := DefaultImageJSONPath()
	if err != nil {
		return err
	}
	imageJSON, err := i.Read(path)
	if err != nil {
		return err
	}

	if imageJSON.Interval.LastPrune != "" {
		lastPruneTime, err := time.Parse(time.RFC3339, imageJSON.Interval.LastPrune)
		if err != nil {
			return errors.Wrap(err, "failed to parse last prune timestamp from JSON")
		}

		if time.Since(lastPruneTime) < PRUNE_TIME {
			// not enough time has passed since the last prune
			return nil
		}
	}

	pruningThreshold := time.Now().Add(-PRUNE_TIME)

	for imageID, timestamp := range imageJSON.Image.ImageIDtoTIME {
		imageTimestamp, err := time.Parse(time.RFC3339, timestamp)
		if err != nil {
			return errors.Wrap(err, "failed to parse image timestamp fron JSON")
		}

		image, err := local.NewImage(imageID, docker, local.FromBaseImage(imageID))
		if err != nil {
			return err
		}
		if !image.Found() {
			delete(imageJSON.Image.ImageIDtoTIME, imageID)
		}

		if imageTimestamp.Before(pruningThreshold) {
			delete(imageJSON.Image.ImageIDtoTIME, imageID)
		}
	}

	imageJSON.Interval.LastPrune = time.Now().Format(time.RFC3339)

	if err := i.Write(imageJSON, path); err != nil {
		return errors.Wrap(err, "failed to write updated image.json")
	}

	return nil
}

func (i *ImagePullPolicyManager) Read(path string) (*ImageJSON, error) {
	// Check if the file exists, if not, return default values
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &ImageJSON{
			Interval: &Interval{
				LastPrune: "",
			},
			Image: &ImageData{},
		}, nil
	}

	jsonData, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "failed to read image.json")
	}
	var imageJSON *ImageJSON
	if err := json.Unmarshal(jsonData, &imageJSON); err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "failed to unmarshal image.json")
	}
	return imageJSON, nil
}

func (i *ImagePullPolicyManager) Write(imageJSON *ImageJSON, path string) error {
	updatedJSON, err := json.MarshalIndent(imageJSON, "", "    ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal updated records")
	}

	return WriteFile(updatedJSON, path)
}

func WriteFile(data []byte, path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return errors.New("failed to open file: " + err.Error())
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return errors.New("failed to write data to file: " + err.Error())
	}
	return nil
}
