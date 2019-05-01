package pack

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buildpack/imgutil"
	"github.com/docker/docker/api/types"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/stack"
	"github.com/buildpack/pack/style"
)

type Lifecycle interface {
	Execute(ctx context.Context, opts build.LifecycleOptions) error
}

type BuildOptions struct {
	AppDir     string // defaults to current working directory
	Builder    string // defaults to default builder on the client config
	RunImage   string // defaults to the best mirror from the builder image or pack config
	Env        map[string]string
	Image      string // required
	Publish    bool
	NoPull     bool
	ClearCache bool
	Buildpacks []string
}

func (c *Client) Build(ctx context.Context, opts BuildOptions) error {
	imageRef, err := c.validateImageReference(opts.Image)
	if err != nil {
		return errors.Wrapf(err, "invalid image name '%s'", opts.Image)
	}

	appDir, err := c.processAppDir(opts.AppDir)
	if err != nil {
		return errors.Wrapf(err, "invalid app dir '%s'", opts.AppDir)
	}

	builderRef, err := c.processBuilderName(opts.Builder)
	if err != nil {
		return errors.Wrapf(err, "invalid builder '%s'", opts.Builder)
	}
	rawBuilderImage, err := c.imageFetcher.Fetch(ctx, builderRef.Name(), true, !opts.NoPull)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch builder image '%s'", builderRef.Name())
	}

	builderImage, err := c.processBuilderImage(rawBuilderImage)
	if err != nil {
		return errors.Wrapf(err, "invalid builder '%s'", opts.Builder)
	}

	runImage := c.processRunImageName(opts.RunImage, imageRef.Context().RegistryStr(), builderImage.GetStackInfo())

	if _, err := c.validateRunImage(ctx, runImage, opts.NoPull, opts.Publish, builderImage.StackID); err != nil {
		return errors.Wrapf(err, "invalid run-image '%s'", runImage)
	}

	extraBuildpacks, group, err := c.processBuildpacks(opts.Buildpacks)
	if err != nil {
		return errors.Wrap(err, "invalid buildpack")
	}

	ephemeralBuilder, err := c.createEphemeralBuilder(rawBuilderImage, opts.Env, group, extraBuildpacks)
	if err != nil {
		return err
	}
	defer c.docker.ImageRemove(context.Background(), ephemeralBuilder.Name(), types.ImageRemoveOptions{Force: true})

	return c.lifecycle.Execute(ctx, build.LifecycleOptions{
		AppDir:     appDir,
		Image:      imageRef,
		Builder:    ephemeralBuilder,
		RunImage:   runImage,
		ClearCache: opts.ClearCache,
		Publish:    opts.Publish,
	})
}

func (c *Client) processBuilderName(builderName string) (name.Reference, error) {
	if builderName == "" {
		if c.config.DefaultBuilder != "" {
			c.logger.Verbose("Using default builder image %s", style.Symbol(c.config.DefaultBuilder))
			builderName = c.config.DefaultBuilder
		} else {
			return nil, errors.New("builder is a required parameter if the client has no default builder")
		}
	}
	return name.ParseReference(builderName, name.WeakValidation)
}

func (c *Client) processBuilderImage(img imgutil.Image) (*builder.Builder, error) {
	builder, err := builder.GetBuilder(img)
	if err != nil {
		return nil, err
	}
	if builder.GetStackInfo().RunImage.Image == "" {
		return nil, errors.New("builder metadata is missing runImage")
	}
	return builder, nil
}

func (c *Client) processRunImageName(runImage, targetRegistry string, builderStackInfo stack.Metadata) string {
	if runImage != "" {
		c.logger.Verbose("Using provided run-image %s", style.Symbol(runImage))
		return runImage
	}
	var localMirrors []string
	localRunImageConfig := c.config.GetRunImage(builderStackInfo.RunImage.Image)
	if localRunImageConfig != nil {
		localMirrors = localRunImageConfig.Mirrors
	}
	runImageName := builderStackInfo.GetBestMirror(targetRegistry, localMirrors)

	// log run image source
	if runImageName == builderStackInfo.GetBestMirror(targetRegistry, []string{}) {
		if runImageName == builderStackInfo.RunImage.Image {
			c.logger.Verbose("Selected run image %s from builder", style.Symbol(runImageName))
		} else {
			c.logger.Verbose("Selected run image mirror %s from builder", style.Symbol(runImageName))
		}
	} else {
		c.logger.Verbose("Selected run image mirror %s from local config", style.Symbol(runImageName))
	}
	return runImageName
}

func (c *Client) validateRunImage(context context.Context, name string, noPull bool, publish bool, expectedStack string) (imgutil.Image, error) {
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

func (c *Client) validateImageReference(imageName string) (name.Reference, error) {
	if imageName == "" {
		return nil, errors.New("image name is a required parameter")
	}
	if _, err := name.ParseReference(imageName, name.WeakValidation); err != nil {
		return nil, err
	}
	ref, err := name.NewTag(imageName, name.WeakValidation)
	if err != nil {
		return nil, fmt.Errorf("'%s' is not a tag reference", imageName)
	}

	return ref, nil
}

func mapAppDirError(dir string, err error) error {
	if os.IsNotExist(err) {
		return errors.Errorf("app directory %s does not exist", style.Symbol(dir))
	} else {
		return err
	}
}

func (c *Client) processAppDir(appDir string) (string, error) {
	var (
		resolvedAppDir = appDir
		err            error
	)

	if appDir == "" {
		if appDir, err = os.Getwd(); err != nil {
			return "", err
		}
	}

	if resolvedAppDir, err = filepath.EvalSymlinks(appDir); err != nil {
		return "", mapAppDirError(appDir, err)
	}

	if resolvedAppDir, err = filepath.Abs(resolvedAppDir); err != nil {
		return "", err
	}

	if fi, err := os.Stat(resolvedAppDir); err != nil {
		return "", mapAppDirError(appDir, err)
	} else if !fi.IsDir() {
		return "", fmt.Errorf("%s is not a directory", appDir)
	}

	return resolvedAppDir, nil
}

func (c *Client) processBuildpacks(buildpacks []string) ([]buildpack.Buildpack, builder.GroupMetadata, error) {
	group := builder.GroupMetadata{Buildpacks: []builder.GroupBuildpack{}}
	var bps []buildpack.Buildpack
	for _, bp := range buildpacks {
		if isLocalBuildpack(bp) {
			if runtime.GOOS == "windows" {
				return nil, builder.GroupMetadata{}, fmt.Errorf("directory buildpacks are not implemented on windows")
			}
			c.logger.Verbose("fetching buildpack from %s", style.Symbol(bp))
			fetchedBP, err := c.buildpackFetcher.FetchBuildpack(bp)
			if err != nil {
				return nil, builder.GroupMetadata{}, errors.Wrapf(err, "failed to fetch buildpack from uri '%s'", bp)
			}
			bps = append(bps, fetchedBP)
			group.Buildpacks = append(group.Buildpacks, builder.GroupBuildpack{ID: fetchedBP.ID, Version: fetchedBP.Version})
		} else {
			id, version := c.parseBuildpack(bp)
			group.Buildpacks = append(group.Buildpacks, builder.GroupBuildpack{ID: id, Version: version})
		}
	}
	return bps, group, nil
}

func isLocalBuildpack(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "buildpack.toml")); !os.IsNotExist(err) {
		return true
	}
	return false
}

func (c *Client) parseBuildpack(bp string) (string, string) {
	parts := strings.Split(bp, "@")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	c.logger.Verbose("No version for %s buildpack provided, will use %s", style.Symbol(parts[0]), style.Symbol(parts[0]+"@latest"))
	return parts[0], "latest"
}

func (c *Client) createEphemeralBuilder(rawBuilderImage imgutil.Image, env map[string]string, group builder.GroupMetadata, buildpacks []buildpack.Buildpack) (*builder.Builder, error) {
	origBuilderName := rawBuilderImage.Name()
	bldr, err := builder.New(rawBuilderImage, fmt.Sprintf("pack.local/builder/%x:latest", randString(10)))
	if err != nil {
		return nil, errors.Wrapf(err, "invalid builder %s", style.Symbol(origBuilderName))
	}
	bldr.SetEnv(env)
	for _, bp := range buildpacks {
		c.logger.Verbose("adding buildpack %s version %s to builder", style.Symbol(bp.ID), style.Symbol(bp.Version))
		if err := bldr.AddBuildpack(bp); err != nil {
			return nil, errors.Wrapf(err, "failed to add buildpack %s version %s to builder", style.Symbol(bp.ID), style.Symbol(bp.Version))
		}
	}
	if len(group.Buildpacks) > 0 {
		c.logger.Verbose("setting custom order")
		if err := bldr.SetOrder([]builder.GroupMetadata{group}); err != nil {
			return nil, errors.Wrap(err, "failed to set custom buildpack order")
		}
	}
	if err := bldr.Save(); err != nil {
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
