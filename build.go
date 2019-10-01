package pack

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buildpack/imgutil"
	"github.com/docker/docker/api/types"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/api"
	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/cmd"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/internal/paths"
	"github.com/buildpack/pack/style"
)

type Lifecycle interface {
	Execute(ctx context.Context, opts build.LifecycleOptions) error
}

type BuildOptions struct {
	Image             string              // required
	Builder           string              // required
	AppPath           string              // defaults to current working directory
	RunImage          string              // defaults to the best mirror from the builder metadata or AdditionalMirrors
	AdditionalMirrors map[string][]string // only considered if RunImage is not provided
	Env               map[string]string
	Publish           bool
	NoPull            bool
	ClearCache        bool
	Buildpacks        []string
	ProxyConfig       *ProxyConfig // defaults to  environment proxy vars
}

type ProxyConfig struct {
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
}

func (c *Client) Build(ctx context.Context, opts BuildOptions) error {
	imageRef, err := c.parseTagReference(opts.Image)
	if err != nil {
		return errors.Wrapf(err, "invalid image name '%s'", opts.Image)
	}

	appPath, err := c.processAppPath(opts.AppPath)
	if err != nil {
		return errors.Wrapf(err, "invalid app path '%s'", opts.AppPath)
	}

	proxyConfig := c.processProxyConfig(opts.ProxyConfig)

	builderRef, err := c.processBuilderName(opts.Builder)
	if err != nil {
		return errors.Wrapf(err, "invalid builder '%s'", opts.Builder)
	}

	rawBuilderImage, err := c.imageFetcher.Fetch(ctx, builderRef.Name(), true, !opts.NoPull)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch builder image '%s'", builderRef.Name())
	}

	bldr, err := c.processBuilderImage(rawBuilderImage)
	if err != nil {
		return errors.Wrapf(err, "invalid builder '%s'", opts.Builder)
	}

	runImage := c.resolveRunImage(opts.RunImage, imageRef.Context().RegistryStr(), bldr.GetStackInfo(), opts.AdditionalMirrors)

	if _, err := c.validateRunImage(ctx, runImage, opts.NoPull, opts.Publish, bldr.StackID); err != nil {
		return errors.Wrapf(err, "invalid run-image '%s'", runImage)
	}

	fetchedBps, group, err := c.processBuildpacks(ctx, opts.Buildpacks)
	if err != nil {
		return errors.Wrap(err, "invalid buildpack")
	}

	ephemeralBuilder, err := c.createEphemeralBuilder(rawBuilderImage, opts.Env, group, fetchedBps)
	if err != nil {
		return err
	}
	defer c.docker.ImageRemove(context.Background(), ephemeralBuilder.Name(), types.ImageRemoveOptions{Force: true})

	descriptor := ephemeralBuilder.GetLifecycleDescriptor()
	if descriptor.Info.Version == nil {
		c.logger.Warnf("lifecycle version unknown, assuming %s", style.Symbol(builder.AssumedLifecycleVersion))
	} else {
		c.logger.Debugf("Executing lifecycle version %s", style.Symbol(descriptor.Info.Version.String()))
	}

	lcPlatformAPIVersion := api.MustParse(builder.AssumedPlatformAPIVersion)
	if descriptor.API.PlatformVersion != nil {
		lcPlatformAPIVersion = descriptor.API.PlatformVersion
	}

	if !api.MustParse(build.PlatformAPIVersion).SupportsVersion(lcPlatformAPIVersion) {
		return errors.Errorf(
			"pack %s (Platform API version %s) is incompatible with builder %s (Platform API version %s)",
			cmd.Version,
			build.PlatformAPIVersion,
			style.Symbol(opts.Builder),
			lcPlatformAPIVersion,
		)
	}

	return c.lifecycle.Execute(ctx, build.LifecycleOptions{
		AppPath:    appPath,
		Image:      imageRef,
		Builder:    ephemeralBuilder,
		RunImage:   runImage,
		ClearCache: opts.ClearCache,
		Publish:    opts.Publish,
		HTTPProxy:  proxyConfig.HTTPProxy,
		HTTPSProxy: proxyConfig.HTTPSProxy,
		NoProxy:    proxyConfig.NoProxy,
	})
}

func (c *Client) processBuilderName(builderName string) (name.Reference, error) {
	if builderName == "" {
		return nil, errors.New("builder is a required parameter if the client has no default builder")
	}
	return name.ParseReference(builderName, name.WeakValidation)
}

func (c *Client) processBuilderImage(img imgutil.Image) (*builder.Builder, error) {
	bldr, err := builder.GetBuilder(img)
	if err != nil {
		return nil, err
	}
	if bldr.GetStackInfo().RunImage.Image == "" {
		return nil, errors.New("builder metadata is missing runImage")
	}
	return bldr, nil
}

func (c *Client) validateRunImage(context context.Context, name string, noPull bool, publish bool, expectedStack string) (imgutil.Image, error) {
	if name == "" {
		return nil, errors.New("run image must be specified")
	}
	img, err := c.imageFetcher.Fetch(context, name, !publish, !noPull)
	if err != nil {
		return nil, err
	}
	stackID, err := img.Label("io.buildpacks.stack.id")
	if err != nil {
		return nil, err
	}
	if stackID != expectedStack {
		return nil, fmt.Errorf("run-image stack id '%s' does not match builder stack '%s'", stackID, expectedStack)
	}
	return img, nil
}

func (c *Client) processAppPath(appPath string) (string, error) {
	var (
		resolvedAppPath string
		err             error
	)

	if appPath == "" {
		if appPath, err = os.Getwd(); err != nil {
			return "", errors.Wrap(err, "get working dir")
		}
	}

	if resolvedAppPath, err = filepath.EvalSymlinks(appPath); err != nil {
		return "", errors.Wrap(err, "evaluate symlink")
	}

	if resolvedAppPath, err = filepath.Abs(resolvedAppPath); err != nil {
		return "", errors.Wrap(err, "resolve absolute path")
	}

	fi, err := os.Stat(resolvedAppPath)
	if err != nil {
		return "", errors.Wrap(err, "stat file")
	}

	if !fi.IsDir() {
		fh, err := os.Open(resolvedAppPath)
		if err != nil {
			return "", errors.Wrap(err, "read file")
		}
		defer fh.Close()

		isZip, err := archive.IsZip(fh)
		if err != nil {
			return "", errors.Wrap(err, "check zip")
		}

		if !isZip {
			return "", errors.New("app path must be a directory or zip")
		}
	}

	return resolvedAppPath, nil
}

func (c *Client) processProxyConfig(config *ProxyConfig) ProxyConfig {
	var (
		httpProxy, httpsProxy, noProxy string
		ok                             bool
	)
	if config != nil {
		return *config
	}
	if httpProxy, ok = os.LookupEnv("HTTP_PROXY"); !ok {
		httpProxy = os.Getenv("http_proxy")
	}
	if httpsProxy, ok = os.LookupEnv("HTTPS_PROXY"); !ok {
		httpsProxy = os.Getenv("https_proxy")
	}
	if noProxy, ok = os.LookupEnv("NO_PROXY"); !ok {
		noProxy = os.Getenv("no_proxy")
	}
	return ProxyConfig{
		HTTPProxy:  httpProxy,
		HTTPSProxy: httpsProxy,
		NoProxy:    noProxy,
	}
}

func (c *Client) processBuildpacks(ctx context.Context, buildpacks []string) ([]builder.Buildpack, builder.OrderEntry, error) {
	group := builder.OrderEntry{Group: []builder.BuildpackRef{}}
	var bps []builder.Buildpack
	for _, bp := range buildpacks {
		if isBuildpackID(bp) {
			id, version := c.parseBuildpack(bp)
			group.Group = append(group.Group, builder.BuildpackRef{
				BuildpackInfo: builder.BuildpackInfo{
					ID:      id,
					Version: version,
				},
			})
		} else {
			err := ensureBPSupport(bp)
			if err != nil {
				return nil, builder.OrderEntry{}, err
			}

			c.logger.Debugf("fetching buildpack from %s", style.Symbol(bp))

			blob, err := c.downloader.Download(ctx, bp)
			if err != nil {
				return nil, builder.OrderEntry{}, errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(bp))
			}

			fetchedBP, err := builder.NewBuildpack(blob)
			if err != nil {
				return nil, builder.OrderEntry{}, errors.Wrapf(err, "creating buildpack from %s", style.Symbol(bp))
			}

			bps = append(bps, fetchedBP)

			group.Group = append(group.Group, builder.BuildpackRef{
				BuildpackInfo: fetchedBP.Descriptor().Info,
			})
		}
	}
	return bps, group, nil
}

func isBuildpackID(bp string) bool {
	if !paths.IsURI(bp) {
		if _, err := os.Stat(bp); err != nil {
			return true
		}
	}
	return false
}

func ensureBPSupport(bpPath string) (err error) {
	p := bpPath
	if paths.IsURI(bpPath) {
		var u *url.URL
		u, err = url.Parse(bpPath)
		if err != nil {
			return err
		}

		if u.Scheme == "file" {
			p, err = paths.URIToFilePath(bpPath)
			if err != nil {
				return err
			}
		}
	}

	if runtime.GOOS == "windows" && !paths.IsURI(p) {
		isDir, err := paths.IsDir(p)
		if err != nil {
			return err
		}

		if isDir {
			return fmt.Errorf("buildpack %s: directory-based buildpacks are not currently supported on Windows", style.Symbol(bpPath))
		}
	}

	return nil
}

func (c *Client) parseBuildpack(bp string) (string, string) {
	parts := strings.Split(bp, "@")
	if len(parts) == 2 {
		if parts[1] == "latest" {
			c.logger.Warn("@latest syntax is deprecated, will not work in future releases")
			return parts[0], ""
		}

		return parts[0], parts[1]
	}

	return parts[0], ""
}

func (c *Client) createEphemeralBuilder(rawBuilderImage imgutil.Image, env map[string]string, group builder.OrderEntry, buildpacks []builder.Buildpack) (*builder.Builder, error) {
	origBuilderName := rawBuilderImage.Name()
	bldr, err := builder.New(rawBuilderImage, fmt.Sprintf("pack.local/builder/%x:latest", randString(10)))
	if err != nil {
		return nil, errors.Wrapf(err, "invalid builder %s", style.Symbol(origBuilderName))
	}
	bldr.SetEnv(env)
	for _, bp := range buildpacks {
		bpInfo := bp.Descriptor().Info
		c.logger.Debugf("adding buildpack %s version %s to builder", style.Symbol(bpInfo.ID), style.Symbol(bpInfo.Version))
		bldr.AddBuildpack(bp)
	}
	if len(group.Group) > 0 {
		c.logger.Debug("setting custom order")
		bldr.SetOrder([]builder.OrderEntry{group})
	}
	if err := bldr.Save(c.logger); err != nil {
		return nil, err
	}
	return bldr, nil
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}
