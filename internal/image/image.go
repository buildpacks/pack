package image

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/style"
)

type Labeled interface {
	Label(string) (string, error)
}

type Labelable interface {
	SetLabel(string, string) error
}

func UnmarshalLabel(labelable Labeled, label string, obj interface{}) (ok bool, err error) {
	labelData, err := labelable.Label(label)
	if err != nil {
		return false, labelRetrievalError(err, label)
	}
	if labelData != "" {
		if err := json.Unmarshal([]byte(labelData), obj); err != nil {
			return false, errors.Wrapf(err, "unmarshalling label %s", style.Symbol(label))
		}
		return true, nil
	}
	return false, nil
}

func ReadLabel(labelable Labeled, label string) (string, bool, error) {
	value, err := labelable.Label(label)
	if err != nil {
		return "", false, labelRetrievalError(err, label)
	}
	return value, value != "", nil
}

func MarshalToLabel(labelable Labelable, label string, data interface{}) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return errors.Wrapf(err, "marshalling data to JSON for label %s", style.Symbol(label))
	}
	if err := labelable.SetLabel(label, string(dataBytes)); err != nil {
		return errors.Wrapf(err, "setting label %s", style.Symbol(label))
	}
	return nil
}

func labelRetrievalError(err error, label string) error {
	return errors.Wrapf(err, "retrieving label %s", style.Symbol(label))
}
