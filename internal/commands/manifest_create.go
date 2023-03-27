package commands

import (
	v2 "github.com/buildpacks/imgutil/remote"

	"github.com/buildpacks/imgutil/layout"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/google/go-containerregistry/pkg/authn"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/spf13/cobra"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func ManifestCreate(logger logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <id>",
		Short: "Creates a manifest list",
		Args:  cobra.MatchAll(cobra.MinimumNArgs(2)),
		Example: `pack manifest create paketobuildpacks/builder:full-1.0.0 \ paketobuildpacks/builder:full-linux-amd64 \
				 paketobuildpacks/builder:full-linux-arm`,
		Long: "manifest create generates a manifest list for a multi-arch image",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {

			var index v1.ImageIndex

			// initialize and set the media type
			index = empty.Index
			index = mutate.IndexMediaType(index, types.DockerManifestList)

			var adds []mutate.IndexAddendum

			for _, j := range args[1:] {

				ref, err := name.ParseReference(j)
				if err != nil {
					panic(err)
				}

				img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
				if err != nil {
					panic(err)
				}

				desc, err := remote.Get(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
				if err != nil {
					panic(err)
				}

				// ##################################################

				img2, err := v2.NewImage(
					"registry-1.docker.io",
					authn.DefaultKeychain,
					v2.FromBaseImage(j),
				)

				os, _ := img2.OS()
				arch, _ := img2.Architecture()

				platform := v1.Platform{}
				platform.Architecture = arch
				platform.OS = os

				desc.Descriptor.Platform = &platform
				logger.Infof(desc.Descriptor.Platform.OS + "-" + desc.Descriptor.Platform.Architecture)

				// ##################################################

				adds = append(adds, mutate.IndexAddendum{Add: img, Descriptor: desc.Descriptor})

			}

			index = mutate.AppendManifests(index, adds...)

			// write the index on disk, for example
			layout.Write("out/", index)

			return nil
		}),
	}

	// cmd.Flags().StringVarP(&flags.API, "api", "a", "0.8", "Buildpack API compatibility of the generated buildpack")

	AddHelpFlag(cmd, "create")
	return cmd
}
