package image

import (
	"encoding/json"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/style"
)

func UnmarshalLabel(img imgutil.Image, label string, obj interface{}) (ok bool, err error) {
	labelData, err := img.Label(label)
	if err != nil {
		return false, labelRetrievalError(err, label, img.Name())
	}
	if labelData != "" {
		if err := json.Unmarshal([]byte(labelData), obj); err != nil {
			return false, errors.Wrapf(err, "unmarshalling label %s from image %s", style.Symbol(label), style.Symbol(img.Name()))
		}
		return true, nil
	}
	return false, nil
}

func ReadLabel(img imgutil.Image, label string) (string, bool, error) {
	value, err := img.Label(label)
	if err != nil {
		return "", false, labelRetrievalError(err, label, img.Name())
	}
	return value, value != "", nil
}

func MarshalToLabel(img imgutil.Image, label string, data interface{}) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return errors.Wrapf(err, "marshalling data to JSON for label %s", style.Symbol(label))
	}
	if err := img.SetLabel(label, string(dataBytes)); err != nil {
		return errors.Wrapf(err, "setting label %s on image %s", style.Symbol(label), style.Symbol(img.Name()))
	}
	return nil
}

func labelRetrievalError(err error, label, imageName string) error {
	return errors.Wrapf(err, "retrieving label %s from image %s", style.Symbol(label), style.Symbol(imageName))
}
