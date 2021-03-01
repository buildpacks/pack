package dist

import (
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/buildpacks/imgutil/layer"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
)

type Labeled interface {
	Label(name string) (value string, err error)
}

type Labelable interface {
	SetLabel(name string, value string) error
}

func SetLabel(labelable Labelable, label string, data interface{}) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return errors.Wrapf(err, "marshalling data to JSON for label %s", style.Symbol(label))
	}
	if err := labelable.SetLabel(label, string(dataBytes)); err != nil {
		return errors.Wrapf(err, "setting label %s", style.Symbol(label))
	}
	return nil
}

func GetLabel(labeled Labeled, label string, obj interface{}) (ok bool, err error) {
	labelData, err := labeled.Label(label)
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

type WorkableImage interface {
	SetOS(string) error
	SetLabel(string, string) error
	AddLayerWithDiffID(path, diffID string) error
}

func AddWindowsShimBaseLayer(image WorkableImage, tmpDir string) error {
	baseLayerFile, err := ioutil.TempFile(tmpDir, "windows-baselayer")
	if err != nil {
		return err
	}
	defer baseLayerFile.Close()

	baseLayer, err := layer.WindowsBaseLayer()
	if err != nil {
		return err
	}

	if _, err := io.Copy(baseLayerFile, baseLayer); err != nil {
		return err
	}

	if err := baseLayerFile.Close(); err != nil {
		return err
	}

	baseLayerPath := baseLayerFile.Name()
	diffID, err := LayerDiffID(baseLayerPath)
	if err != nil {
		return err
	}

	if err := image.AddLayerWithDiffID(baseLayerPath, diffID.String()); err != nil {
		return err
	}

	return nil
}
