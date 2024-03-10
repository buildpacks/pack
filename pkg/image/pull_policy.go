package image

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/logging"
)

// PullPolicy defines a policy for how to manage images
type PullPolicy int

var interval string

type ImagePullPolicyManager struct {
	Logger logging.Logger
}

var (
	hourly        = "1h"
	daily         = "1d"
	weekly        = "7d"
	intervalRegex = regexp.MustCompile(`^(\d+d)?(\d+h)?(\d+m)?$`)
)

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
	// PullWithInterval pulls images with specified intervals
	PullWithInterval
)

type Interval struct {
	PullingInterval string `json:"pulling_interval"`
	PruningInterval string `json:"pruning_interval"`
	LastPrune       string `json:"last_prune"`
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
func ParsePullPolicy(policy string, logger logging.Logger) (PullPolicy, error) {
	pullPolicyManager := NewPullPolicyManager(logger)

	if val, ok := nameMap[policy]; ok {
		if val == PullHourly {
			err := pullPolicyManager.updateImageJSONDuration(hourly)
			if err != nil {
				return PullAlways, err
			}
		}
		if val == PullDaily {
			err := pullPolicyManager.updateImageJSONDuration(daily)
			if err != nil {
				return PullAlways, err
			}
		}
		if val == PullWeekly {
			err := pullPolicyManager.updateImageJSONDuration(weekly)
			if err != nil {
				return PullAlways, err
			}
		}

		return val, nil
	}

	if strings.HasPrefix(policy, "interval=") {
		interval = policy
		intervalStr := strings.TrimPrefix(policy, "interval=")
		matches := intervalRegex.FindStringSubmatch(intervalStr)
		if len(matches) == 0 {
			return PullAlways, errors.Errorf("invalid interval format: %s", intervalStr)
		}

		err := pullPolicyManager.updateImageJSONDuration(intervalStr)
		if err != nil {
			return PullAlways, err
		}

		return PullWithInterval, nil
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
	case PullWithInterval:
		return fmt.Sprintf("%v", interval)
	}

	return ""
}

func (i *ImagePullPolicyManager) updateImageJSONDuration(intervalStr string) error {
	path, err := DefaultImageJSONPath()
	if err != nil {
		return err
	}

	imageJSON, err := i.Read(path)
	if err != nil {
		return err
	}

	imageJSON.Interval.PullingInterval = intervalStr

	return i.Write(imageJSON, path)
}

func parseDurationString(durationStr string) (time.Duration, error) {
	var totalMinutes int
	for i := 0; i < len(durationStr); {
		endIndex := i + 1
		for endIndex < len(durationStr) && durationStr[endIndex] >= '0' && durationStr[endIndex] <= '9' {
			endIndex++
		}

		value, err := strconv.Atoi(durationStr[i:endIndex])
		if err != nil {
			return 0, errors.Wrapf(err, "invalid interval format: %s", durationStr)
		}
		unit := durationStr[endIndex]

		switch unit {
		case 'd':
			totalMinutes += value * 24 * 60
		case 'h':
			totalMinutes += value * 60
		case 'm':
			totalMinutes += value
		default:
			return 0, errors.Errorf("invalid interval uniit: %s", string(unit))
		}

		i = endIndex + 1
	}

	return time.Duration(totalMinutes) * time.Minute, nil
}

func (i *ImagePullPolicyManager) PruneOldImages(f *Fetcher) error {
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

		pruningInterval, err := parseDurationString(imageJSON.Interval.PruningInterval)
		if err != nil {
			return errors.Wrap(err, "failed to parse pruning interval from JSON")
		}

		if time.Since(lastPruneTime) < pruningInterval {
			// not enough time has passed since the last prune
			return nil
		}
	}

	// prune images older than the pruning interval
	pruningDuration, err := parseDurationString(imageJSON.Interval.PruningInterval)
	if err != nil {
		return errors.Wrap(err, "failed to parse pruning interval from JSON")
	}

	pruningThreshold := time.Now().Add(-pruningDuration)

	for imageID, timestamp := range imageJSON.Image.ImageIDtoTIME {
		imageTimestamp, err := time.Parse(time.RFC3339, timestamp)
		if err != nil {
			return errors.Wrap(err, "failed to parse image timestamp fron JSON")
		}

		_, err = f.fetchDaemonImage(imageID)
		if !errors.Is(err, ErrNotFound) {
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
				PullingInterval: "7d",
				PruningInterval: "7d",
				LastPrune:       "",
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
