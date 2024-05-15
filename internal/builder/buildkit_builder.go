package builder

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/buildpacks/pack/internal/stack"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/containerd/containerd/platforms"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	"github.com/moby/buildkit/frontend/gateway/client"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

type BuildkitBuilder struct {
	name string // name of the image
	res *client.Result
	platform ocispecs.Platform
	buildEnvs map[string]string
	prevImage *client.Result
	lifecycle            Lifecycle
	lifecycleDescriptor  LifecycleDescriptor
	additionalBuildpacks buildpack.ManagedCollection
	additionalExtensions buildpack.ManagedCollection
	metadata             Metadata
	mixins               []string
	env                  map[string]string
	uid, gid             int
	StackID              string
	replaceOrder         bool
	order                dist.Order
	orderExtensions      dist.Order
	validateMixins       bool
}

func NewBuildkitBuilder(res *client.Result, ref string, platform ocispecs.Platform) (*BuildkitBuilder, error) {
	return constructBuildkitBuilder(res, ref, platform, true)
}

func constructBuildkitBuilder(res *client.Result, name string, platform ocispecs.Platform, errOnMissingLabel bool, ops ...BuilderOption) (*BuildkitBuilder, error) {
	var metadata Metadata
	configFile, err := buildkitBuidlerConfigFile(res)
	if err != nil {
		return nil, err
	}

	if ok, err := dist.GetBuildkitLabel(res, metadataLabel, &metadata); err != nil {
		return nil, errors.Wrapf(err, "getting label %s", metadataLabel)
	} else if !ok && errOnMissingLabel {
		return nil, fmt.Errorf("builder %s missing label %s -- try recreating builder", style.Symbol(name), style.Symbol(metadataLabel))
	}

	opts := &options{}
	for _, op := range ops {
		if err := op(opts); err != nil {
			return nil, err
		}
	}

	if opts.runImage != "" {
		// Do we need to look for available mirrors? for now the mirrors are gone if you override the run-image
		// create an issue if you want to preserve the mirrors
		metadata.RunImages = []RunImageMetadata{{Image: opts.runImage}}
		metadata.Stack.RunImage = RunImageMetadata{Image: opts.runImage}
	}

	for labelKey, labelValue := range opts.labels {
		configFile.Config.Labels[labelKey] = labelValue
	}

	p := platforms.Format(platform)
	configBytes, err := json.Marshal(configFile)
	if err != nil {
		return nil, err
	}
	res.AddMeta(fmt.Sprintf("%s/%s", exptypes.ExporterImageConfigKey, p), configBytes)

	bldr := &BuildkitBuilder{
		name: name,
		res: res,
		metadata:             metadata,
		lifecycleDescriptor:  constructLifecycleDescriptor(metadata),
		env:                  map[string]string{},
		buildEnvs:       map[string]string{},
		validateMixins:       true,
		additionalBuildpacks: buildpack.NewManagedCollectionV2(opts.toFlatten),
		additionalExtensions: buildpack.NewManagedCollectionV2(opts.toFlatten),
	}

	if err := addLabelsToBuildkitBuilder(bldr); err != nil {
		return nil, errors.Wrap(err, "adding image labels to builder")
	}

	return bldr, nil
}

func buildkitBuidlerConfigFile(res *client.Result) (*ocispecs.Image, error) {
	var configFile *ocispecs.Image
	configFileBytes := res.Metadata[exptypes.ExporterImageConfigKey]
	if err := json.Unmarshal(configFileBytes, configFile); err != nil {
		return nil, err
	}

	return configFile, nil
}

func addLabelsToBuildkitBuilder(bldr *BuildkitBuilder) error {
	var err error
	config, err := buildkitBuidlerConfigFile(bldr.res)
	if err != nil {
		return err
	}
	bldr.uid, bldr.gid, err = bldr.buildkitUserAndGroupIDs(config)
	if err != nil {
		return err
	}

	bldr.StackID, err = bldr.Label(stackLabel)
	if err != nil {
		return errors.Wrapf(err, "get label %s from image %s", style.Symbol(stackLabel), style.Symbol(bldr.name))
	}

	if _, err = dist.GetBuildkitLabel(bldr.res, stack.MixinsLabel, &bldr.mixins); err != nil {
		return errors.Wrapf(err, "getting label %s", stack.MixinsLabel)
	}

	if _, err = dist.GetBuildkitLabel(bldr.res, OrderLabel, &bldr.order); err != nil {
		return errors.Wrapf(err, "getting label %s", OrderLabel)
	}

	if _, err = dist.GetBuildkitLabel(bldr.res, OrderExtensionsLabel, &bldr.orderExtensions); err != nil {
		return errors.Wrapf(err, "getting label %s", OrderExtensionsLabel)
	}

	return nil
}

// DefaultRunImage returns the default run image metadata
func (b *BuildkitBuilder) DefaultRunImage() RunImageMetadata {
	// run.images are ensured in builder.ValidateConfig()
	// per the spec, we use the first one as the default
	return b.RunImages()[0]
}

// RunImages returns all run image metadata
func (b *BuildkitBuilder) RunImages() []RunImageMetadata {
	return append(b.metadata.RunImages, b.Stack().RunImage)
}

// Stack returns the stack metadata
func (b *BuildkitBuilder) Stack() StackMetadata {
	return b.metadata.Stack
}

func (b *BuildkitBuilder) buildkitUserAndGroupIDs(config *ocispecs.Image) (int, int, error) {
	platformStr := platforms.Format(b.platform)
	sUID := getIteminSlice(config.Config.Env, EnvUID)
	if sUID == "" {
		return 0, 0, fmt.Errorf("image %s(%s) missing required env var %s", style.Symbol(b.name), platformStr, style.Symbol(EnvUID))
	}

	sGID := getIteminSlice(config.Config.Env, EnvGID)
	if sGID == "" {
		return 0, 0, fmt.Errorf("image %s(%s) missing required env var %s", style.Symbol(b.name), platformStr, style.Symbol(EnvGID))
	}

	uid, err := strconv.Atoi(sUID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse %s, value %s should be an integer", style.Symbol(EnvUID), style.Symbol(sUID))
	}

	gid, err := strconv.Atoi(sGID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse %s, value %s should be an integer", style.Symbol(EnvGID), style.Symbol(sGID))
	}

	return uid, gid, nil
}

func (b *BuildkitBuilder) Label(label string) (string, error) {
	config, err := buildkitBuidlerConfigFile(b.res)
	if err != nil {
		return "", err
	}

	return getIteminSlice(config.Config.Env, label), nil
}

func getIteminSlice(slice []string, item string) string {
	for _, s := range slice {
		k, v, _ := strings.Cut(s, "=")
		if k == item {
			return v
		}
	}
	return ""
}
