package builder

import (
	"slices"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/pack/internal/config"
)

// TrustedBuilders is a flat list of builder image names that are trusted by default.
var TrustedBuilders = []string{
	"gcr.io/buildpacks/builder:google-22",
	"heroku/builder:24",
	"heroku/builder:22",
	"heroku/builder:20",
	"paketobuildpacks/builder-jammy-base",
	"paketobuildpacks/builder-jammy-full",
	"paketobuildpacks/builder-jammy-tiny",
	"paketobuildpacks/builder-jammy-buildpackless-static",
	"paketobuildpacks/ubuntu-noble-builder",
	"paketobuildpacks/builder-ubi8-base",
	"paketobuildpacks/ubi-9-builder",
	"paketobuildpacks/ubi-10-builder",
}

// SuggestedBuilder contains display metadata for recommended builders.
type SuggestedBuilder struct {
	Vendor             string
	Image              string
	DefaultDescription string
}

// SuggestedBuilders is the list of builders shown by `builder suggest`.
var SuggestedBuilders = []SuggestedBuilder{
	{
		Vendor:             "Google",
		Image:              "gcr.io/buildpacks/builder:google-22",
		DefaultDescription: "Ubuntu 22.04 base image with buildpacks for .NET, Dart, Go, Java, Node.js, PHP, Python, and Ruby",
	},
	{
		Vendor:             "Heroku",
		Image:              "heroku/builder:24",
		DefaultDescription: "Ubuntu 24.04 AMD64+ARM64 base image with buildpacks for Go, Java, Node.js, PHP, Python, Ruby & Scala.",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/builder-jammy-base",
		DefaultDescription: "Small base image with buildpacks for Java, Node.js, Golang, .NET Core, Python & Ruby",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/builder-jammy-full",
		DefaultDescription: "Larger base image with buildpacks for Java, Node.js, Golang, .NET Core, Python, Ruby, & PHP",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/builder-jammy-tiny",
		DefaultDescription: "Tiny base image (jammy build image, distroless run image) with buildpacks for Golang & Java",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/builder-jammy-buildpackless-static",
		DefaultDescription: "Static base image (jammy build image, distroless run image) suitable for static binaries like Go or Rust",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/ubuntu-noble-builder",
		DefaultDescription: "Small base image with buildpacks for Java, Node.js or .NET Core",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/builder-ubi8-base",
		DefaultDescription: "Universal Base Image (RHEL8) with buildpacks to build Node.js or Java runtimes. Support also the new extension feature (aka apply Dockerfile)",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/ubi-9-builder",
		DefaultDescription: "Universal Base Image (RHEL9) with buildpacks to build Node.js runtimes.",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/ubi-10-builder",
		DefaultDescription: "Universal Base Image (RHEL10) with buildpacks to build Node.js runtimes.",
	},
}

func IsKnownTrustedBuilder(builderName string) bool {
	return slices.Contains(TrustedBuilders, builderName)
}

func IsTrustedBuilder(cfg config.Config, builderName string) (bool, error) {
	builderReference, err := name.ParseReference(builderName, name.WithDefaultTag(""))
	if err != nil {
		return false, err
	}

	// Collect all trusted builder names
	trustedBuilderNames := make([]string, len(TrustedBuilders))
	copy(trustedBuilderNames, TrustedBuilders)

	// Add user-configured trusted builders
	for _, trustedBuilder := range cfg.TrustedBuilders {
		trustedBuilderNames = append(trustedBuilderNames, trustedBuilder.Name)
	}

	// Check if builder matches any trusted builder
	for _, trustedBuilderName := range trustedBuilderNames {
		trustedBuilderReference, err := name.ParseReference(trustedBuilderName, name.WithDefaultTag(""))
		if err != nil {
			return false, err
		}

		if trustedBuilderReference.Identifier() != "" {
			if builderReference.Name() == trustedBuilderReference.Name() {
				return true, nil
			}
		} else {
			if builderReference.Context().RepositoryStr() == trustedBuilderReference.Context().RepositoryStr() {
				return true, nil
			}
		}
	}

	return false, nil
}
