package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/builder"
	internalConfig "github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/image"
)

func (c *Client) Detect(ctx context.Context, opts BuildOptions) error {
	appPath, err := c.processAppPath(opts.AppPath)
	if err != nil {
		return errors.Wrapf(err, "invalid app path '%s'", opts.AppPath)
	}

	proxyConfig := c.processProxyConfig(opts.ProxyConfig)

	builderRef, err := c.processBuilderName(opts.Builder)
	if err != nil {
		return errors.Wrapf(err, "invalid builder '%s'", opts.Builder)
	}

	rawBuilderImage, err := c.imageFetcher.Fetch(ctx, builderRef.Name(), image.FetchOptions{Daemon: true, PullPolicy: opts.PullPolicy})
	if err != nil {
		return errors.Wrapf(err, "failed to fetch builder image '%s'", builderRef.Name())
	}

	builderOS, err := rawBuilderImage.OS()
	if err != nil {
		return errors.Wrapf(err, "getting builder OS")
	}

	builderArch, err := rawBuilderImage.Architecture()
	if err != nil {
		return errors.Wrapf(err, "getting builder architecture")
	}

	bldr, err := c.getBuilder(rawBuilderImage)
	if err != nil {
		return errors.Wrapf(err, "invalid builder %s", style.Symbol(opts.Builder))
	}

	fetchedBPs, order, err := c.processBuildpacks(ctx, bldr.Image(), bldr.Buildpacks(), bldr.Order(), bldr.StackID, opts)
	if err != nil {
		return err
	}

	fetchedExs, orderExtensions, err := c.processExtensions(ctx, bldr.Image(), bldr.Extensions(), bldr.OrderExtensions(), bldr.StackID, opts)
	if err != nil {
		return err
	}

	// Ensure the builder's platform APIs are supported
	var builderPlatformAPIs builder.APISet
	builderPlatformAPIs = append(builderPlatformAPIs, bldr.LifecycleDescriptor().APIs.Platform.Deprecated...)
	builderPlatformAPIs = append(builderPlatformAPIs, bldr.LifecycleDescriptor().APIs.Platform.Supported...)
	if !supportsPlatformAPI(builderPlatformAPIs) {
		c.logger.Debugf("pack %s supports Platform API(s): %s", c.version, strings.Join(build.SupportedPlatformAPIVersions.AsStrings(), ", "))
		c.logger.Debugf("Builder %s supports Platform API(s): %s", style.Symbol(opts.Builder), strings.Join(builderPlatformAPIs.AsStrings(), ", "))
		return errors.Errorf("Builder %s is incompatible with this version of pack", style.Symbol(opts.Builder))
	}

	// Get the platform API version to use
	lifecycleVersion := bldr.LifecycleDescriptor().Info.Version
	var (
		lifecycleOptsLifecycleImage string
		lifecycleAPIs               []string
	)

	if supportsLifecycleImage(lifecycleVersion) {
		lifecycleImageName := opts.LifecycleImage
		if lifecycleImageName == "" {
			lifecycleImageName = fmt.Sprintf("%s:%s", internalConfig.DefaultLifecycleImageRepo, lifecycleVersion.String())
		}

		lifecycleImage, err := c.imageFetcher.Fetch(
			ctx,
			lifecycleImageName,
			image.FetchOptions{
				Daemon:     true,
				PullPolicy: opts.PullPolicy,
				Platform:   fmt.Sprintf("%s/%s", builderOS, builderArch),
			},
		)
		if err != nil {
			return fmt.Errorf("fetching lifecycle image: %w", err)
		}

		lifecycleOptsLifecycleImage = lifecycleImage.Name()
		labels, err := lifecycleImage.Labels()
		if err != nil {
			return fmt.Errorf("reading labels of lifecycle image: %w", err)
		}

		lifecycleAPIs, err = extractSupportedLifecycleApis(labels)
		if err != nil {
			return fmt.Errorf("reading api versions of lifecycle image: %w", err)
		}
	}

	usingPlatformAPI, err := build.FindLatestSupported(append(
		bldr.LifecycleDescriptor().APIs.Platform.Deprecated,
		bldr.LifecycleDescriptor().APIs.Platform.Supported...),
		lifecycleAPIs)
	if err != nil {
		return fmt.Errorf("finding latest supported Platform API: %w", err)
	}

	buildEnvs := map[string]string{}

	for k, v := range opts.Env {
		buildEnvs[k] = v
	}

	ephemeralBuilder, err := c.createEphemeralBuilder(rawBuilderImage, buildEnvs, order, fetchedBPs, orderExtensions, fetchedExs, usingPlatformAPI.LessThan("0.12"))
	if err != nil {
		return err
	}
	defer c.docker.ImageRemove(context.Background(), ephemeralBuilder.Name(), types.ImageRemoveOptions{Force: true})

	if len(bldr.OrderExtensions()) > 0 || len(ephemeralBuilder.OrderExtensions()) > 0 {
		if !c.experimental {
			return fmt.Errorf("experimental features must be enabled when builder contains image extensions")
		}
		if builderOS == "windows" {
			return fmt.Errorf("builder contains image extensions which are not supported for Windows builds")
		}
		if !(opts.PullPolicy == image.PullAlways) {
			return fmt.Errorf("pull policy must be 'always' when builder contains image extensions")
		}
	}

	processedVolumes, warnings, err := processVolumes(builderOS, opts.ContainerConfig.Volumes)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		c.logger.Warn(warning)
	}

	lifecycleOpts := build.LifecycleOptions{
		AppPath:        appPath,
		Builder:        ephemeralBuilder,
		BuilderImage:   builderRef.Name(),
		LifecycleImage: ephemeralBuilder.Name(),
		HTTPProxy:      proxyConfig.HTTPProxy,
		HTTPSProxy:     proxyConfig.HTTPSProxy,
		NoProxy:        proxyConfig.NoProxy,
		Network:        opts.ContainerConfig.Network,
		Volumes:        processedVolumes,
		Keychain:       c.keychain,
	}

	if supportsLifecycleImage(lifecycleVersion) {
		lifecycleOpts.LifecycleImage = lifecycleOptsLifecycleImage
		lifecycleOpts.LifecycleApis = lifecycleAPIs
	}

	if err = c.lifecycleExecutor.Detect(ctx, lifecycleOpts); err != nil {
		return fmt.Errorf("executing detect: %w", err)
	}
	// Log the final detected group
	return nil
}
