package dist

import (
	"encoding/json"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/style"
)

func SetLabel(image imgutil.Image, label string, data interface{}) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return errors.Wrapf(err, "marshalling data to JSON for label %s", style.Symbol(label))
	}
	if err := image.SetLabel(label, string(dataBytes)); err != nil {
		return errors.Wrapf(err, "setting label %s", style.Symbol(label))
	}
	return nil
}

func GetLabel(image imgutil.Image, label string, obj interface{}) (ok bool, err error) {
	labelData, err := image.Label(label)
	if err != nil {
		return false, errors.Wrapf(err, "retrieving label %s", style.Symbol(label))
	}
	if labelData != "" {
		if err := json.Unmarshal([]byte(labelData), obj); err != nil {
			return false, errors.Wrapf(err, "unmarshalling label %s", style.Symbol(label))
		}
		return true, nil
	}
	return false, nil
}
