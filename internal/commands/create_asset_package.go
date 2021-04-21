package commands

import (
	"fmt"
	"sort"

	"github.com/buildpacks/lifecycle/api"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/logging"
)

const RequiredBuildpackAPIForAssets = "0.8"

type CreateAssetPackageFlags struct {
	BuildpackLocator string
	PullPolicy       pubcfg.PullPolicy
	Publish          bool
	Registry         string
	ImagePreference  string
	OS               string
	Format           string
}

var inspectOptionsMapping = map[string][]pack.InspectBuildpackOptions{
	pubcfg.LocalImagePreference:  {{Daemon: true}, {Daemon: false}},
	pubcfg.RemoteImagePreference: {{Daemon: false}, {Daemon: true}},
	pubcfg.OnlyLocalImage:        {{Daemon: true}},
	pubcfg.OnlyRemoteImage:       {{Daemon: false}},
}

// CreateAssetPackage creates an OCI image containing assets, called an Asset Package
// from the specified buildpacks. This image may added to both builders and individual runs
// of 'pack build ...' and allow buildpacks access to the contained assets.
func CreateAssetPackage(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var flags CreateAssetPackageFlags

	cmd := &cobra.Command{
		Use:     "create <asset-package-name>",
		Hidden:  false,
		Args:    cobra.ExactArgs(1),
		Short:   "create an asset package",
		Example: "pack asset-package create /path/to/buildpack/root",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validateAssetPackageFlags(flags); err != nil {
				return err
			}
			cacheImageName, err := validatePackageImageName(args[0])
			if err != nil {
				return err
			}

			// assume that inspectOptionsMapping contains all valid pull policies
			inspectOptions := inspectOptionsMapping[flags.ImagePreference]
			for k := range inspectOptions {
				inspectOptions[k].Registry = flags.Registry
				inspectOptions[k].BuildpackName = flags.BuildpackLocator
			}

			buildpackInfo, err := inspectBuildpack(client, inspectOptions)
			if err != nil {
				return errors.New("buildpack not found")
			}

			assets, err := getAssets(buildpackInfo)
			if err != nil {
				return errors.Wrap(err, "error fetching buildpack assets")
			}
			if err := client.CreateAssetPackage(cmd.Context(), pack.CreateAssetPackageOptions{
				ImageName: cacheImageName.String(),
				Assets:    assets,
				Publish:   flags.Publish,
				OS:        flags.OS,
				Format:    flags.Format,
			}); err != nil {
				return errors.Wrap(err, "error, unable to create asset package")
			}

			return nil
		}),
	}

	cmd.Flags().StringVarP(&flags.Format, "format", "f", "image", `Format to save package as ("image" or "file")`)
	cmd.Flags().StringVarP(&flags.BuildpackLocator, "buildpack", "b", "", "Buildpack Locator")
	cmd.Flags().StringVar(&flags.ImagePreference, "image-preference", pubcfg.LocalImagePreference, `preferred location to look for buildpack images.
Accepted values are:
- only-remote
- prefer-remote
- only-local
- prefer-local`)
	cmd.Flags().StringVarP(&flags.Registry, "buildpack-registry", "R", cfg.DefaultRegistryName, "Buildpack Registry by name")
	cmd.Flags().StringVarP(&flags.BuildpackLocator, "config", "c", "", "optional asset-package.toml to filter assets in the resulting asset package")
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, "Publish to image registry")
	cmd.Flags().StringVar(&flags.OS, "os", pubcfg.LinuxOS, "cache image os type")

	AddHelpFlag(cmd, "create")
	return cmd
}

func inspectBuildpack(c PackClient, inspectOptions []pack.InspectBuildpackOptions) (*pack.BuildpackInfo, error) {
	var buildpackInfo *pack.BuildpackInfo
	var err error
	for _, inspectOption := range inspectOptions {
		buildpackInfo, err = c.InspectBuildpack(inspectOption)
		switch {
		case errors.Is(err, image.ErrNotFound):
			continue
		case err != nil:
			return nil, err
		default:
			return buildpackInfo, nil
		}
	}

	return nil, image.ErrNotFound
}

func validateAssetPackageFlags(flags CreateAssetPackageFlags) error {
	if flags.BuildpackLocator == "" {
		return errors.New("must specify a buildpack locator using the --buildpack flag")
	}
	if err := pubcfg.ValidateOS(flags.OS); err != nil {
		return err
	}
	if err := pubcfg.ValidateImagePreference(flags.ImagePreference); err != nil {
		return err
	}
	if err := pubcfg.ValidateFormat(flags.Format); err != nil {
		return err
	}

	return nil
}

func validatePackageImageName(imgName string) (name.Tag, error) {
	tag, err := name.NewTag(imgName, name.WeakValidation)
	if err != nil {
		return name.Tag{}, errors.Wrap(err, "unable to parse cache image name")
	}
	return tag, nil
}

func validateAPIAllowsAssets(layer dist.BuildpackLayerInfo) error {
	if len(layer.Assets) > 0 && layer.API.Compare(api.MustParse(RequiredBuildpackAPIForAssets)) < 0 {
		return fmt.Errorf("creating asset packages requires buildpack API >= 0.8, got: %s", layer.API.String())
	}

	return nil
}

func getAssets(info *pack.BuildpackInfo) ([]dist.AssetInfo, error) {
	result := []dist.AssetInfo{}
	assetMap := map[string]dist.AssetInfo{}

	for _, bp := range info.Buildpacks {
		layer, ok := info.BuildpackLayers[bp.ID][bp.Version]
		if !ok {
			return []dist.AssetInfo{}, fmt.Errorf("unable to find metadata for buildpack %s, %s", bp.ID, bp.Version)
		}

		if err := validateAPIAllowsAssets(layer); err != nil {
			return []dist.AssetInfo{}, err
		}

		for _, asset := range layer.Assets {
			assetMap[asset.Sha256] = asset
		}
	}

	for _, a := range assetMap {
		result = append(result, a)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Sha256 < result[j].Sha256
	})

	return result, nil
}
