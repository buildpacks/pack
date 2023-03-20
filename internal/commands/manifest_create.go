package commands

import (
	"fmt"
	"strings"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/remote"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
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
			id := args[0]
			idParts := strings.Split(id, " ")
			// dirName := idParts[len(idParts)-1]

			fmt.Println(id)

			repoName := "registry-1.docker.io"
			manifestListName := idParts[0]

			// for i, j := range args {
			// 	manifestVal, err := crane.Manifest(args[i])
			// 	if err != nil {
			// 		return err
			// 	}
			// 	manifest := &v1.Manifest{}
			// 	json.Unmarshal(manifestVal, manifest)

			// }

			manifest1, err := crane.Manifest(args[1])
			manifest2, err := crane.Digest(args[2])
			fmt.Print(string(manifest1))
			fmt.Print(string(manifest2))

			img, err := remote.NewImage(
				repoName,
				authn.DefaultKeychain,
				remote.FromBaseImage(manifestListName),
				remote.WithDefaultPlatform(imgutil.Platform{
					OS:           "linux",
					Architecture: "amd64",
				}),
			)

			// manifest := v1.Manifest{}
			// err := json.Unmarshal(data, &manifest)

			// manifest := &v1.Manifest{}
			// if err := unmarshalJSONFromBlob(blob, pathFromDescriptor(*manifestDescriptor), manifest); err != nil {
			// 	return nil, err
			// }

			// arch, err := img.Architecture()

			os, err := img.OS()

			// manifestSize, err := img.ManifestSize()
			// labels, err := img.Labels()

			if err != nil {
				return err
			}

			fmt.Println(os)
			// fmt.Println(labels)
			// img.fetchRemoteImage

			// tag := "hello-universe"
			// repository := "cnbs/sample-package"
			// url := fmt.Sprintf("https://registry-1.docker.io/v2/%s/manifests/%s", repository, tag)

			// req, err := http.NewRequest("GET", url, nil)
			// if err != nil {
			// 	fmt.Println(err)
			// 	return nil
			// }

			// // Encode the Docker registry username and password in base64 format
			// auth := base64.StdEncoding.EncodeToString([]byte("drac98:dckr_pat_-t8WI7sW7xE2xoew5lr6YM3jbY0"))
			// req.Header.Set("Bearer", fmt.Sprintf("Basic %s", auth))
			// req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

			// client := &http.Client{}
			// resp, err := client.Do(req)
			// if err != nil {
			// 	fmt.Println(err)
			// 	return nil
			// }

			// defer resp.Body.Close()
			// body, err := ioutil.ReadAll(resp.Body)
			// if err != nil {
			// 	fmt.Println(err)
			// 	return nil
			// }

			// fmt.Println(string(body))

			// var path string
			// if len(flags.Path) == 0 {
			// 	cwd, err := os.Getwd()
			// 	if err != nil {
			// 		return err
			// 	}
			// 	path = filepath.Join(cwd, dirName)
			// } else {
			// 	path = flags.Path
			// }

			// _, err := os.Stat(path)
			// if !os.IsNotExist(err) {
			// 	return fmt.Errorf("directory %s exists", style.Symbol(path))
			// }

			return nil
		}),
	}

	// cmd.Flags().StringVarP(&flags.API, "api", "a", "0.8", "Buildpack API compatibility of the generated buildpack")

	AddHelpFlag(cmd, "create")
	return cmd
}
