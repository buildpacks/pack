package builder

type SuggestedBuilder struct {
	Vendor             string
	Image              string
	DefaultDescription string
}

var SuggestedBuilders = []SuggestedBuilder{
	{
		Vendor:             "Google",
		Image:              "gcr.io/buildpacks/builder:v1",
		DefaultDescription: "GCP Builder for all runtimes",
	},
	{
		Vendor:             "Heroku",
		Image:              "heroku/builder:22",
		DefaultDescription: "heroku-22 base image with buildpacks for Java and Node.js",
	},
	{
		Vendor:             "Heroku",
		Image:              "heroku/buildpacks:20",
		DefaultDescription: "heroku-20 base image with buildpacks for Ruby, Java, Node.js, Python, Golang, & PHP",
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
}
