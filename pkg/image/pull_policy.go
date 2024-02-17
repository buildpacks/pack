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
)

// PullPolicy defines a policy for how to manage images
type PullPolicy int

var interval time.Duration

var (
	intervalRegex = regexp.MustCompile(`^(\d+d)?(\d+h)?(\d+m)?$`)
	imagePath     string
)

const (
	// PullAlways images, even if they are present
	PullAlways PullPolicy = iota
	// PullNever images, even if they are not present
	PullNever
	// PullIfNotPresent pulls images if they aren't present
	PullIfNotPresent
	// PullWithInterval pulls images with specified intervals
	PullWithInterval
)

var nameMap = map[string]PullPolicy{"always": PullAlways, "never": PullNever, "if-not-present": PullIfNotPresent, "": PullAlways}

// ParsePullPolicy from string with support for interval formats
func ParsePullPolicy(policy string) (PullPolicy, error) {
	if val, ok := nameMap[policy]; ok {
		return val, nil
	}

	if strings.HasPrefix(policy, "interval=") {
		intervalStr := strings.TrimPrefix(policy, "interval=")
		matches := intervalRegex.FindStringSubmatch(intervalStr)
		if len(matches) == 0 {
			return PullAlways, errors.Errorf("invalid interval format: %s", intervalStr)
		}

		updateImageJSONDuration(intervalStr)

		return PullWithInterval, nil
	}

	return PullAlways, errors.Errorf("invalid pull policy %s", policy)
}

func (p PullPolicy) String() string {
	switch p {
	case PullAlways:
		return "always"
	case PullNever:
		return "never"
	case PullIfNotPresent:
		return "if-not-present"
	case PullWithInterval:
		return fmt.Sprintf("interval=%v", interval)
	}

	return ""
}

func updateImageJSONDuration(intervalStr string) error {
	imageJSON, err := readImageJSON()
	if err != nil {
		return err
	}

	imageJSON.Interval.Duration = intervalStr

	updatedJSON, err := json.MarshalIndent(imageJSON, "", "    ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal updated records")
	}

	return os.WriteFile(imagePath, updatedJSON, 0644)
}

func readImageJSON() (ImageJSON, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ImageJSON{}, errors.Wrap(err, "failed to get home directory")
	}
	imagePath = filepath.Join(homeDir, ".pack", "image.json")

	jsonData, err := os.ReadFile(imagePath)
	if err != nil && !os.IsNotExist(err) {
		return ImageJSON{}, errors.Wrap(err, "failed to read image.json")
	}

	var imageJSON ImageJSON
	if err := json.Unmarshal(jsonData, &imageJSON); err != nil && !os.IsNotExist(err) {
		return ImageJSON{}, errors.Wrap(err, "failed to unmarshal image.json")
	}

	return imageJSON, nil
}

func CheckImagePullInterval(imageID string) (bool, error) {
	imageJSON, err := readImageJSON()
	if err != nil {
		return false, err
	}

	timestamp, ok := imageJSON.Image.ImageIDtoTIME[imageID]
	if !ok {
		// If the image ID is not present, return true
		return true, nil
	}

	imageTimestamp, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse image timestamp from JSON")
	}

	durationStr := imageJSON.Interval.Duration

	duration, err := parseDurationString(durationStr)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse duration from JSON")
	}

	timeThreshold := time.Now().Add(-duration)

	return imageTimestamp.Before(timeThreshold), nil
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
			return 0, errors.Errorf("invalid interval unit: %s", string(unit))
		}

		i = endIndex + 1
	}

	return time.Duration(totalMinutes) * time.Minute, nil
}
