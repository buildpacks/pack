package dist

import (
	"encoding/json"

	"github.com/buildpacks/pack/internal/style"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	"github.com/moby/buildkit/frontend/gateway/client"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

func GetBuildkitLabel(res *client.Result, label string, obj any) (ok bool, err error) {
	var configFile = &ocispecs.Image{}
	configFileBytes := res.Metadata[exptypes.ExporterImageConfigKey]
	if err := json.Unmarshal(configFileBytes, configFile); err != nil {
		return false, err
	}

	labelData := configFile.Config.Labels[label]
	if labelData != "" {
		if err := json.Unmarshal([]byte(labelData), obj); err != nil {
			return false, errors.Wrapf(err, "unmarshalling label %s", style.Symbol(label))
		}
		return true, nil
	}
	return false, nil
}