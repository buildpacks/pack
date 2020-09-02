package pack_test

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/buildpacks/pack"
)

// This example shows the basic usage of the package: Create a client,
// call a configuration object, call the client's Build function.
func Example_build() {
	//create a context object
	context := context.Background()

	//initialize a pack client
	client, err := pack.NewClient()
	if err != nil {
		panic(err)
	}

	// replace this with the location of a sample application
	// For a list of prepared samples see the 'apps' folder at
	// https://github.com/buildpacks/samples.
	appPath := "local/path/to/application/root"

	// randomly select a builder to use from among the following
	builderList := []string{
		"gcr.io/buildpacks/builder:v1",
		"heroku/buildpacks:18",
		"gcr.io/paketo-buildpacks/builder:base",
	}

	randomIndex := rand.Intn(len(builderList))
	randomBuilder := builderList[randomIndex]

	// initialize our options
	buildOpts := pack.BuildOptions{
		Image:        "pack-lib-test-image:0.0.1",
		Builder:      randomBuilder,
		AppPath:      appPath,
		TrustBuilder: true,
	}

	fmt.Println("building application image")

	// build an image
	err = client.Build(context, buildOpts)
	if err != nil {
		panic(err)
	}

	fmt.Println("build completed")
}
