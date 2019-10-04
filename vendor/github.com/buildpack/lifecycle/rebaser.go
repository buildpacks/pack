package lifecycle

import (
	"encoding/json"
	"fmt"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/logging"
	"github.com/buildpack/lifecycle/metadata"
)

type Rebaser struct {
	Logger logging.Logger
}

func (r *Rebaser) Rebase(
	workingImage imgutil.Image,
	newBaseImage imgutil.Image,
	additionalNames []string,
) error {
	origMetadata, err := metadata.GetLayersMetadata(workingImage)
	if err != nil {
		return errors.Wrap(err, "get image metadata")
	}

	workingImageStack, err := metadata.GetStackMetadata(workingImage)
	if err != nil {
		return errors.Wrap(err, "get working image stack")
	}

	newBaseImageStack, err := metadata.GetStackMetadata(newBaseImage)
	if err != nil {
		return errors.Wrap(err, "get new base image stack")
	}

	if workingImageStack.Id == "" {
		return errors.New("stack not defined on working image")
	}

	if newBaseImageStack.Id == "" {
		return errors.New("stack not defined on new base image")
	}

	if workingImageStack.Id != newBaseImageStack.Id {
		return errors.New(fmt.Sprintf("incompatible stack: '%s' is not compatible with '%s'", newBaseImageStack.Id, workingImageStack.Id))
	}

	err = workingImage.Rebase(origMetadata.RunImage.TopLayer, newBaseImage)
	if err != nil {
		return errors.Wrap(err, "rebase working image")
	}

	origMetadata.RunImage.TopLayer, err = newBaseImage.TopLayer()
	if err != nil {
		return errors.Wrap(err, "get rebase run image top layer SHA")
	}

	identifier, err := newBaseImage.Identifier()
	if err != nil {
		return errors.Wrap(err, "get run image id or digest")
	}
	origMetadata.RunImage.Reference = identifier.String()

	data, err := json.Marshal(origMetadata)
	if err != nil {
		return errors.Wrap(err, "marshall metadata")
	}

	if err := workingImage.SetLabel(metadata.LayerMetadataLabel, string(data)); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}

	return saveImage(workingImage, additionalNames, r.Logger)
}
