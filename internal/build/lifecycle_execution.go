package build

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/cache"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/logging"
)

const (
	defaultProcessType = "web"
	overrideGID        = 0
	sourceDateEpochEnv = "SOURCE_DATE_EPOCH"
)

type LifecycleExecution struct {
	logger       logging.Logger
	docker       client.CommonAPIClient
	platformAPI  *api.Version
	layersVolume string
	appVolume    string
	os           string
	mountPaths   mountPaths
	opts         LifecycleOptions
}

func NewLifecycleExecution(logger logging.Logger, docker client.CommonAPIClient, opts LifecycleOptions) (*LifecycleExecution, error) {
	latestSupportedPlatformAPI, err := findLatestSupported(append(
		opts.Builder.LifecycleDescriptor().APIs.Platform.Deprecated,
		opts.Builder.LifecycleDescriptor().APIs.Platform.Supported...,
	))
	if err != nil {
		return nil, err
	}

	osType, err := opts.Builder.Image().OS()
	if err != nil {
		return nil, err
	}

	exec := &LifecycleExecution{
		logger:       logger,
		docker:       docker,
		layersVolume: paths.FilterReservedNames("pack-layers-" + randString(10)),
		appVolume:    paths.FilterReservedNames("pack-app-" + randString(10)),
		platformAPI:  latestSupportedPlatformAPI,
		opts:         opts,
		os:           osType,
		mountPaths:   mountPathsForOS(osType, opts.Workspace),
	}

	if opts.Interactive {
		exec.logger = opts.Termui
	}

	return exec, nil
}

func findLatestSupported(apis []*api.Version) (*api.Version, error) {
	for i := len(SupportedPlatformAPIVersions) - 1; i >= 0; i-- {
		for _, version := range apis {
			if SupportedPlatformAPIVersions[i].Equal(version) {
				return version, nil
			}
		}
	}

	return nil, errors.New("unable to find a supported Platform API version")
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

func (l *LifecycleExecution) Builder() Builder {
	return l.opts.Builder
}

func (l *LifecycleExecution) AppPath() string {
	return l.opts.AppPath
}

func (l LifecycleExecution) AppDir() string {
	return l.mountPaths.appDir()
}

func (l *LifecycleExecution) AppVolume() string {
	return l.appVolume
}

func (l *LifecycleExecution) LayersVolume() string {
	return l.layersVolume
}

func (l *LifecycleExecution) PlatformAPI() *api.Version {
	return l.platformAPI
}

func (l *LifecycleExecution) ImageName() name.Reference {
	return l.opts.Image
}

func (l *LifecycleExecution) PrevImageName() string {
	return l.opts.PreviousImage
}

func (l *LifecycleExecution) Run(ctx context.Context, phaseFactoryCreator PhaseFactoryCreator) error {
	phaseFactory := phaseFactoryCreator(l)
	var buildCache Cache
	if l.opts.CacheImage != "" || (l.opts.Cache.Build.Format == cache.CacheImage) {
		cacheImageName := l.opts.CacheImage
		if cacheImageName == "" {
			cacheImageName = l.opts.Cache.Build.Source
		}
		cacheImage, err := name.ParseReference(cacheImageName, name.WeakValidation)
		if err != nil {
			return fmt.Errorf("invalid cache image name: %s", err)
		}
		buildCache = cache.NewImageCache(cacheImage, l.docker)
	} else {
		switch l.opts.Cache.Build.Format {
		case cache.CacheVolume:
			buildCache = cache.NewVolumeCache(l.opts.Image, l.opts.Cache.Build, "build", l.docker)
			l.logger.Debugf("Using build cache volume %s", style.Symbol(buildCache.Name()))
		case cache.CacheBind:
			buildCache = cache.NewBindCache(l.opts.Cache.Build, l.docker)
			l.logger.Debugf("Using build cache dir %s", style.Symbol(buildCache.Name()))
		}
	}

	if l.opts.ClearCache {
		if err := buildCache.Clear(ctx); err != nil {
			return errors.Wrap(err, "clearing build cache")
		}
		l.logger.Debugf("Build cache %s cleared", style.Symbol(buildCache.Name()))
	}

	launchCache := cache.NewVolumeCache(l.opts.Image, l.opts.Cache.Launch, "launch", l.docker)

	if !l.opts.UseCreator {
		if l.platformAPI.LessThan("0.7") {
			l.logger.Info(style.Step("DETECTING"))
			if err := l.Detect(ctx, phaseFactory); err != nil {
				return err
			}

			l.logger.Info(style.Step("ANALYZING"))
			if err := l.Analyze(ctx, buildCache, launchCache, phaseFactory); err != nil {
				return err
			}
		} else {
			l.logger.Info(style.Step("ANALYZING"))
			if err := l.Analyze(ctx, buildCache, launchCache, phaseFactory); err != nil {
				return err
			}

			l.logger.Info(style.Step("DETECTING"))
			if err := l.Detect(ctx, phaseFactory); err != nil {
				return err
			}
		}

		l.logger.Info(style.Step("RESTORING"))
		if l.opts.ClearCache && l.PlatformAPI().LessThan("0.10") {
			l.logger.Info("Skipping 'restore' due to clearing cache")
		} else if err := l.Restore(ctx, buildCache, phaseFactory); err != nil {
			return err
		}

		if l.platformAPI.AtLeast("0.10") && l.hasExtensions() {
			if l.os == "windows" {
				return fmt.Errorf("builder has an order for extensions which is not supported for Windows builds")
			}
			l.logger.Info(style.Step("EXTENDING"))
			if err := l.Extend(ctx, buildCache, phaseFactory); err != nil {
				return err
			}
		} else {
			l.logger.Info(style.Step("BUILDING"))
			if err := l.Build(ctx, phaseFactory); err != nil {
				return err
			}
		}

		l.logger.Info(style.Step("EXPORTING"))
		return l.Export(ctx, buildCache, launchCache, phaseFactory)
	}

	if l.platformAPI.AtLeast("0.10") && l.hasExtensions() {
		return errors.New("builder has an order for extensions which is not supported when using the creator")
	}
	return l.Create(ctx, buildCache, launchCache, phaseFactory)
}

func (l *LifecycleExecution) Cleanup() error {
	var reterr error
	if err := l.docker.VolumeRemove(context.Background(), l.layersVolume, true); err != nil {
		reterr = errors.Wrapf(err, "failed to clean up layers volume %s", l.layersVolume)
	}
	if err := l.docker.VolumeRemove(context.Background(), l.appVolume, true); err != nil {
		reterr = errors.Wrapf(err, "failed to clean up app volume %s", l.appVolume)
	}
	return reterr
}

func (l *LifecycleExecution) Create(ctx context.Context, buildCache, launchCache Cache, phaseFactory PhaseFactory) error {
	flags := addTags([]string{
		"-app", l.mountPaths.appDir(),
		"-cache-dir", l.mountPaths.cacheDir(),
		"-run-image", l.opts.RunImage,
	}, l.opts.AdditionalTags)

	if l.opts.ClearCache {
		flags = append(flags, "-skip-restore")
	}

	if l.opts.GID >= overrideGID {
		flags = append(flags, "-gid", strconv.Itoa(l.opts.GID))
	}

	if l.opts.PreviousImage != "" {
		if l.opts.Image == nil {
			return errors.New("image can't be nil")
		}

		image, err := name.ParseReference(l.opts.Image.Name(), name.WeakValidation)
		if err != nil {
			return fmt.Errorf("invalid image name: %s", err)
		}

		prevImage, err := name.ParseReference(l.opts.PreviousImage, name.WeakValidation)
		if err != nil {
			return fmt.Errorf("invalid previous image name: %s", err)
		}
		if l.opts.Publish {
			if image.Context().RegistryStr() != prevImage.Context().RegistryStr() {
				return fmt.Errorf(`when --publish is used, <previous-image> must be in the same image registry as <image>
                image registry = %s
                previous-image registry = %s`, image.Context().RegistryStr(), prevImage.Context().RegistryStr())
			}
		}

		flags = append(flags, "-previous-image", l.opts.PreviousImage)
	}

	processType := determineDefaultProcessType(l.platformAPI, l.opts.DefaultProcessType)
	if processType != "" {
		flags = append(flags, "-process-type", processType)
	}

	var cacheBindOp PhaseConfigProviderOperation
	switch buildCache.Type() {
	case cache.Image:
		flags = append(flags, "-cache-image", buildCache.Name())
		cacheBindOp = WithBinds(l.opts.Volumes...)
	case cache.Volume, cache.Bind:
		cacheBindOp = WithBinds(append(l.opts.Volumes, fmt.Sprintf("%s:%s", buildCache.Name(), l.mountPaths.cacheDir()))...)
	}

	withEnv := NullOp()
	if l.opts.CreationTime != nil && l.platformAPI.AtLeast("0.9") {
		withEnv = WithEnv(fmt.Sprintf("%s=%s", sourceDateEpochEnv, strconv.Itoa(int(l.opts.CreationTime.Unix()))))
	}

	opts := []PhaseConfigProviderOperation{
		WithFlags(l.withLogLevel(flags...)...),
		WithArgs(l.opts.Image.String()),
		WithNetwork(l.opts.Network),
		cacheBindOp,
		WithContainerOperations(WriteProjectMetadata(l.mountPaths.projectPath(), l.opts.ProjectMetadata, l.os)),
		WithContainerOperations(CopyDir(l.opts.AppPath, l.mountPaths.appDir(), l.opts.Builder.UID(), l.opts.Builder.GID(), l.os, true, l.opts.FileFilter)),
		If(l.opts.SBOMDestinationDir != "", WithPostContainerRunOperations(
			EnsureVolumeAccess(l.opts.Builder.UID(), l.opts.Builder.GID(), l.os, l.layersVolume, l.appVolume),
			CopyOutTo(l.mountPaths.sbomDir(), l.opts.SBOMDestinationDir))),
		If(l.opts.Interactive, WithPostContainerRunOperations(
			EnsureVolumeAccess(l.opts.Builder.UID(), l.opts.Builder.GID(), l.os, l.layersVolume, l.appVolume),
			CopyOut(l.opts.Termui.ReadLayers, l.mountPaths.layersDir(), l.mountPaths.appDir()))),
		withEnv,
	}

	if l.opts.Publish {
		authConfig, err := auth.BuildEnvVar(authn.DefaultKeychain, l.opts.Image.String(), l.opts.RunImage, l.opts.CacheImage, l.opts.PreviousImage)
		if err != nil {
			return err
		}

		opts = append(opts, WithRoot(), WithRegistryAccess(authConfig))
	} else {
		opts = append(opts,
			WithDaemonAccess(l.opts.DockerHost),
			WithFlags("-daemon", "-launch-cache", l.mountPaths.launchCacheDir()),
			WithBinds(fmt.Sprintf("%s:%s", launchCache.Name(), l.mountPaths.launchCacheDir())),
		)
	}

	create := phaseFactory.New(NewPhaseConfigProvider("creator", l, opts...))
	defer create.Cleanup()
	return create.Run(ctx)
}

func (l *LifecycleExecution) Detect(ctx context.Context, phaseFactory PhaseFactory) error {
	flags := []string{"-app", l.mountPaths.appDir()}

	envOp := NullOp()
	if l.hasExtensions() && l.platformAPI.AtLeast("0.10") {
		envOp = WithEnv("CNB_EXPERIMENTAL_MODE=warn")
	}

	configProvider := NewPhaseConfigProvider(
		"detector",
		l,
		WithLogPrefix("detector"),
		WithArgs(
			l.withLogLevel()...,
		),
		WithNetwork(l.opts.Network),
		WithBinds(l.opts.Volumes...),
		WithContainerOperations(
			EnsureVolumeAccess(l.opts.Builder.UID(), l.opts.Builder.GID(), l.os, l.layersVolume, l.appVolume),
			CopyDir(l.opts.AppPath, l.mountPaths.appDir(), l.opts.Builder.UID(), l.opts.Builder.GID(), l.os, true, l.opts.FileFilter),
		),
		WithFlags(flags...),
		envOp,
	)

	detect := phaseFactory.New(configProvider)
	defer detect.Cleanup()
	return detect.Run(ctx)
}

func (l *LifecycleExecution) Restore(ctx context.Context, buildCache Cache, phaseFactory PhaseFactory) error {
	// build up flags and ops
	var flags []string
	if l.opts.ClearCache {
		flags = append(flags, "-skip-layers")
	}
	var registryImages []string

	// for cache
	cacheBindOp := NullOp()
	switch buildCache.Type() {
	case cache.Image:
		flags = append(flags, "-cache-image", buildCache.Name())
		registryImages = append(registryImages, buildCache.Name())
	case cache.Volume:
		flags = append(flags, "-cache-dir", l.mountPaths.cacheDir())
		cacheBindOp = WithBinds(fmt.Sprintf("%s:%s", buildCache.Name(), l.mountPaths.cacheDir()))
	}

	// for gid
	if l.opts.GID >= overrideGID {
		flags = append(flags, "-gid", strconv.Itoa(l.opts.GID))
	}

	// for kaniko
	kanikoCacheBindOp := NullOp()
	if l.platformAPI.AtLeast("0.10") && l.hasExtensions() {
		flags = append(flags, "-build-image", l.opts.BuilderImage)
		registryImages = append(registryImages, l.opts.BuilderImage)

		switch buildCache.Type() {
		case cache.Volume:
			kanikoCacheBindOp = WithBinds(fmt.Sprintf("%s:%s", buildCache.Name(), l.mountPaths.kanikoCacheDir()))
		default:
			return fmt.Errorf("build cache must be volume cache when building with extensions")
		}
	}

	// for auths
	registryOp := NullOp()
	if len(registryImages) > 0 {
		authConfig, err := auth.BuildEnvVar(authn.DefaultKeychain, registryImages...)
		if err != nil {
			return err
		}
		registryOp = WithRegistryAccess(authConfig)
	}

	flagsOp := WithFlags(flags...)

	configProvider := NewPhaseConfigProvider(
		"restorer",
		l,
		WithLogPrefix("restorer"),
		WithImage(l.opts.LifecycleImage),
		WithEnv(fmt.Sprintf("%s=%d", builder.EnvUID, l.opts.Builder.UID()), fmt.Sprintf("%s=%d", builder.EnvGID, l.opts.Builder.GID())),
		WithRoot(), // remove after platform API 0.2 is no longer supported
		WithArgs(
			l.withLogLevel()...,
		),
		WithNetwork(l.opts.Network),
		flagsOp,
		cacheBindOp,
		registryOp,
		kanikoCacheBindOp,
	)

	restore := phaseFactory.New(configProvider)
	defer restore.Cleanup()
	return restore.Run(ctx)
}

func (l *LifecycleExecution) Analyze(ctx context.Context, buildCache, launchCache Cache, phaseFactory PhaseFactory) error {
	var flags []string
	args := []string{l.opts.Image.String()}
	platformAPILessThan07 := l.platformAPI.LessThan("0.7")

	cacheBindOp := NullOp()
	if l.opts.ClearCache {
		if platformAPILessThan07 || l.platformAPI.AtLeast("0.9") {
			args = prependArg("-skip-layers", args)
		}
	} else {
		switch buildCache.Type() {
		case cache.Image:
			flags = append(flags, "-cache-image", buildCache.Name())
		case cache.Volume:
			if platformAPILessThan07 {
				args = append([]string{"-cache-dir", l.mountPaths.cacheDir()}, args...)
				cacheBindOp = WithBinds(fmt.Sprintf("%s:%s", buildCache.Name(), l.mountPaths.cacheDir()))
			}
		}
	}

	launchCacheBindOp := NullOp()
	if l.platformAPI.AtLeast("0.9") {
		if !l.opts.Publish {
			args = append([]string{"-launch-cache", l.mountPaths.launchCacheDir()}, args...)
			launchCacheBindOp = WithBinds(fmt.Sprintf("%s:%s", launchCache.Name(), l.mountPaths.launchCacheDir()))
		}
	}

	if l.opts.GID >= overrideGID {
		flags = append(flags, "-gid", strconv.Itoa(l.opts.GID))
	}

	if l.opts.PreviousImage != "" {
		if l.opts.Image == nil {
			return errors.New("image can't be nil")
		}

		image, err := name.ParseReference(l.opts.Image.Name(), name.WeakValidation)
		if err != nil {
			return fmt.Errorf("invalid image name: %s", err)
		}

		prevImage, err := name.ParseReference(l.opts.PreviousImage, name.WeakValidation)
		if err != nil {
			return fmt.Errorf("invalid previous image name: %s", err)
		}
		if l.opts.Publish {
			if image.Context().RegistryStr() != prevImage.Context().RegistryStr() {
				return fmt.Errorf(`when --publish is used, <previous-image> must be in the same image registry as <image>
	            image registry = %s
	            previous-image registry = %s`, image.Context().RegistryStr(), prevImage.Context().RegistryStr())
			}
		}
		if platformAPILessThan07 {
			l.opts.Image = prevImage
		} else {
			args = append([]string{"-previous-image", l.opts.PreviousImage}, args...)
		}
	}

	stackOp := NullOp()
	if !platformAPILessThan07 {
		for _, tag := range l.opts.AdditionalTags {
			args = append([]string{"-tag", tag}, args...)
		}
		if l.opts.RunImage != "" {
			args = append([]string{"-run-image", l.opts.RunImage}, args...)
		}
		args = append([]string{"-stack", l.mountPaths.stackPath()}, args...)
		stackOp = WithContainerOperations(WriteStackToml(l.mountPaths.stackPath(), l.opts.Builder.Stack(), l.os))
	}

	flagsOp := WithFlags(flags...)

	var analyze RunnerCleaner
	if l.opts.Publish {
		authConfig, err := auth.BuildEnvVar(authn.DefaultKeychain, l.opts.Image.String(), l.opts.RunImage, l.opts.CacheImage, l.opts.PreviousImage)
		if err != nil {
			return err
		}

		configProvider := NewPhaseConfigProvider(
			"analyzer",
			l,
			WithLogPrefix("analyzer"),
			WithImage(l.opts.LifecycleImage),
			WithEnv(fmt.Sprintf("%s=%d", builder.EnvUID, l.opts.Builder.UID()), fmt.Sprintf("%s=%d", builder.EnvGID, l.opts.Builder.GID())),
			WithRegistryAccess(authConfig),
			WithRoot(),
			WithArgs(l.withLogLevel(args...)...),
			WithNetwork(l.opts.Network),
			flagsOp,
			cacheBindOp,
			stackOp,
		)

		analyze = phaseFactory.New(configProvider)
	} else {
		configProvider := NewPhaseConfigProvider(
			"analyzer",
			l,
			WithLogPrefix("analyzer"),
			WithImage(l.opts.LifecycleImage),
			WithEnv(
				fmt.Sprintf("%s=%d", builder.EnvUID, l.opts.Builder.UID()),
				fmt.Sprintf("%s=%d", builder.EnvGID, l.opts.Builder.GID()),
			),
			WithDaemonAccess(l.opts.DockerHost),
			launchCacheBindOp,
			WithFlags(l.withLogLevel("-daemon")...),
			WithArgs(args...),
			flagsOp,
			WithNetwork(l.opts.Network),
			cacheBindOp,
			stackOp,
		)

		analyze = phaseFactory.New(configProvider)
	}

	defer analyze.Cleanup()
	return analyze.Run(ctx)
}

func (l *LifecycleExecution) Build(ctx context.Context, phaseFactory PhaseFactory) error {
	flags := []string{"-app", l.mountPaths.appDir()}
	configProvider := NewPhaseConfigProvider(
		"builder",
		l,
		WithLogPrefix("builder"),
		WithArgs(l.withLogLevel()...),
		WithNetwork(l.opts.Network),
		WithBinds(l.opts.Volumes...),
		WithFlags(flags...),
	)

	build := phaseFactory.New(configProvider)
	defer build.Cleanup()
	return build.Run(ctx)
}

func (l *LifecycleExecution) Extend(ctx context.Context, buildCache Cache, phaseFactory PhaseFactory) error {
	flags := []string{"-app", l.mountPaths.appDir()}

	// set kaniko cache opt
	var kanikoCacheBindOp PhaseConfigProviderOperation
	switch buildCache.Type() {
	case cache.Volume:
		kanikoCacheBindOp = WithBinds(fmt.Sprintf("%s:%s", buildCache.Name(), l.mountPaths.kanikoCacheDir()))
	default:
		return fmt.Errorf("build cache must be volume cache when building with extensions")
	}

	configProvider := NewPhaseConfigProvider(
		"extender",
		l,
		WithLogPrefix("extender"),
		WithArgs(l.withLogLevel()...),
		WithBinds(l.opts.Volumes...),
		WithEnv("CNB_EXPERIMENTAL_MODE=warn"),
		WithFlags(flags...),
		WithNetwork(l.opts.Network),
		WithRoot(),
		kanikoCacheBindOp,
	)

	extend := phaseFactory.New(configProvider)
	defer extend.Cleanup()
	return extend.Run(ctx)
}

func determineDefaultProcessType(platformAPI *api.Version, providedValue string) string {
	shouldSetForceDefault := platformAPI.Compare(api.MustParse("0.4")) >= 0 &&
		platformAPI.Compare(api.MustParse("0.6")) < 0
	if providedValue == "" && shouldSetForceDefault {
		return defaultProcessType
	}

	return providedValue
}

func (l *LifecycleExecution) Export(ctx context.Context, buildCache, launchCache Cache, phaseFactory PhaseFactory) error {
	flags := []string{
		"-app", l.mountPaths.appDir(),
		"-cache-dir", l.mountPaths.cacheDir(),
		"-stack", l.mountPaths.stackPath(),
	}

	if l.platformAPI.LessThan("0.7") {
		flags = append(flags,
			"-run-image", l.opts.RunImage,
		)
	}
	processType := determineDefaultProcessType(l.platformAPI, l.opts.DefaultProcessType)
	if processType != "" {
		flags = append(flags, "-process-type", processType)
	}
	if l.opts.GID >= overrideGID {
		flags = append(flags, "-gid", strconv.Itoa(l.opts.GID))
	}

	cacheBindOp := NullOp()
	switch buildCache.Type() {
	case cache.Image:
		flags = append(flags, "-cache-image", buildCache.Name())
	case cache.Volume:
		cacheBindOp = WithBinds(fmt.Sprintf("%s:%s", buildCache.Name(), l.mountPaths.cacheDir()))
	}

	withEnv := NullOp()
	if l.opts.CreationTime != nil && l.platformAPI.AtLeast("0.9") {
		withEnv = WithEnv(fmt.Sprintf("%s=%s", sourceDateEpochEnv, strconv.Itoa(int(l.opts.CreationTime.Unix()))))
	}

	opts := []PhaseConfigProviderOperation{
		WithLogPrefix("exporter"),
		WithImage(l.opts.LifecycleImage),
		WithEnv(
			fmt.Sprintf("%s=%d", builder.EnvUID, l.opts.Builder.UID()),
			fmt.Sprintf("%s=%d", builder.EnvGID, l.opts.Builder.GID()),
		),
		WithFlags(
			l.withLogLevel(flags...)...,
		),
		WithArgs(append([]string{l.opts.Image.String()}, l.opts.AdditionalTags...)...),
		WithRoot(),
		WithNetwork(l.opts.Network),
		cacheBindOp,
		WithContainerOperations(WriteStackToml(l.mountPaths.stackPath(), l.opts.Builder.Stack(), l.os)),
		WithContainerOperations(WriteProjectMetadata(l.mountPaths.projectPath(), l.opts.ProjectMetadata, l.os)),
		If(l.opts.SBOMDestinationDir != "", WithPostContainerRunOperations(
			EnsureVolumeAccess(l.opts.Builder.UID(), l.opts.Builder.GID(), l.os, l.layersVolume, l.appVolume),
			CopyOutTo(l.mountPaths.sbomDir(), l.opts.SBOMDestinationDir))),
		If(l.opts.Interactive, WithPostContainerRunOperations(
			EnsureVolumeAccess(l.opts.Builder.UID(), l.opts.Builder.GID(), l.os, l.layersVolume, l.appVolume),
			CopyOut(l.opts.Termui.ReadLayers, l.mountPaths.layersDir(), l.mountPaths.appDir()))),
		withEnv,
	}

	var export RunnerCleaner
	if l.opts.Publish {
		authConfig, err := auth.BuildEnvVar(authn.DefaultKeychain, l.opts.Image.String(), l.opts.RunImage, l.opts.CacheImage, l.opts.PreviousImage)
		if err != nil {
			return err
		}

		opts = append(
			opts,
			WithRegistryAccess(authConfig),
			WithRoot(),
		)
		export = phaseFactory.New(NewPhaseConfigProvider("exporter", l, opts...))
	} else {
		opts = append(
			opts,
			WithDaemonAccess(l.opts.DockerHost),
			WithFlags("-daemon", "-launch-cache", l.mountPaths.launchCacheDir()),
			WithBinds(fmt.Sprintf("%s:%s", launchCache.Name(), l.mountPaths.launchCacheDir())),
		)
		export = phaseFactory.New(NewPhaseConfigProvider("exporter", l, opts...))
	}

	defer export.Cleanup()
	return export.Run(ctx)
}

func (l *LifecycleExecution) withLogLevel(args ...string) []string {
	if l.logger.IsVerbose() {
		return append([]string{"-log-level", "debug"}, args...)
	}
	return args
}

func (l *LifecycleExecution) hasExtensions() bool {
	return len(l.opts.Builder.OrderExtensions()) > 0
}

func prependArg(arg string, args []string) []string {
	return append([]string{arg}, args...)
}

func addTags(flags, additionalTags []string) []string {
	for _, tag := range additionalTags {
		flags = append(flags, "-tag", tag)
	}
	return flags
}
