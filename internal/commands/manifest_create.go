package commands

import (
	"encoding/json"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/remote"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"

	// v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
)

// BuildpackNew generates the scaffolding of a buildpack
func ManifestCreate(logger logging.Logger) *cobra.Command {
	// var flags BuildpackNewFlags
	cmd := &cobra.Command{
		Use:   "create <id>",
		Short: "Creates a manifest list",
		Args:  cobra.MatchAll(cobra.MinimumNArgs(2)),
		Example: `pack manifest create paketobuildpacks/builder:full-1.0.0 \ paketobuildpacks/builder:full-linux-amd64 \
				 paketobuildpacks/builder:full-linux-arm`,
		Long: "manifest create generates a manifest list for a multi-arch image",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			var manifests []v1.Descriptor
			desc := v1.Descriptor{}

			for _, j := range args[1:] {
				digestVal, err := crane.Digest(j)
				if err != nil {
					return err
				}

				manifestVal, err := crane.Manifest(j)
				if err != nil {
					return err
				}

				manifest := v1.Manifest{}
				json.Unmarshal(manifestVal, &manifest)

				img, err := remote.NewImage(
					"registry-1.docker.io",
					authn.DefaultKeychain,
					remote.FromBaseImage(j),
					remote.WithDefaultPlatform(imgutil.Platform{
						OS:           "linux",
						Architecture: "amd64",
					}),
				)

				// fmt.Println(manifest)
				os, _ := img.OS()
				arch, _ := img.Architecture()

				platform := v1.Platform{}
				platform.Architecture = arch
				platform.OS = os

				desc.Size, _ = img.ManifestSize()
				desc.Platform = &platform
				desc.Digest = digest.Digest(digestVal)
				desc.MediaType = manifest.MediaType

				manifests = append(manifests, desc)

			}

			index := v1.Index{}
			index.SchemaVersion = 2
			index.MediaType = "application/vnd.oci.image.index.v1+json"
			index.Manifests = manifests

			// for _, j := range manifests {
			// 	de, _ := json.Marshal(j)
			// 	logger.Infof(string(de))
			// }

			de, _ := json.Marshal(index)
			logger.Infof(string(de))

			return nil
		}),
	}

	// cmd.Flags().StringVarP(&flags.API, "api", "a", "0.8", "Buildpack API compatibility of the generated buildpack")

	AddHelpFlag(cmd, "create")
	return cmd
}
