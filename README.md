# pack - Buildpack CLI [![Travis Build Status](https://travis-ci.org/buildpack/pack.svg?branch=master)](https://travis-ci.org/buildpack/pack) [![Windows Build status](https://ci.appveyor.com/api/projects/status/0dspvks46snrda2v?svg=true)](https://ci.appveyor.com/project/buildpack/pack)

**`pack`** makes it easy for
- **application developers** to use [buildpacks](https://buildpacks.io/) to convert code into runnable images
- **buildpack authors** to develop and package buildpacks for distribution

## Contents
- [Building app images using `build`](#building-app-images-using-build)
  - [Example: Building using the default builder image](#example-building-using-the-default-builder-image)
  - [Example: Building using a specified buildpack](#example-building-using-a-specified-buildpack)
  - [Building explained](#building-explained)
- [Updating app images using `rebase`](#updating-app-images-using-rebase)
  - [Example: Rebasing an app image](#example-rebasing-an-app-image)
  - [Rebasing explained](#rebasing-explained)
- [Working with builders using `create-builder`](#working-with-builders-using-create-builder)
  - [Example: Creating a builder from buildpacks](#example-creating-a-builder-from-buildpacks)
  - [Builders explained](#builders-explained)
- [Managing stacks](#managing-stacks)
  - [Example: Adding a stack](#example-adding-a-stack)
  - [Example: Updating a stack](#example-updating-a-stack)
  - [Example: Deleting a stack](#example-deleting-a-stack)
  - [Example: Setting the default stack](#example-setting-the-default-stack)
  - [Listing stacks](#listing-stacks)
- [Resources](#resources)
- [Development](#development)

----

## Building app images using `build`

`pack build` enables app developers to create runnable app images from source code using buildpacks.

```bash
$ pack build <image-name>
```

### Example: Building using the default builder image

In the following example, an app image is created from Node.js application source code.

```bash
$ cd /path/to/node/app
$ pack build my-app:my-tag

# ... Detect, analyze and build output

Successfully built 2452b4b1fce1
Successfully tagged my-app:my-tag
```

In this case, the default [builder](#working-with-builders-using-create-builder) is used, and an appropriate buildpack
is automatically selected from the builder based on the app source code. To understand more about what builders are and
how to create or use them, see the
[Working with builders using `create-builder`](#working-with-builders-using-create-builder) section.

To publish the produced image to an image registry, include the `--publish` flag:

```bash
$ pack build private-registry.example.com/my-app:my-tag --publish
```

### Example: Building using a specified buildpack

In the following example, an app image is created from Node.js application source code, using a buildpack chosen by the
user.

```bash
$ cd /path/to/node/app
$ pack build my-app:my-tag --buildpack path/to/some/buildpack

# ...
*** DETECTING WITH MANUALLY-PROVIDED GROUP:
2018/10/29 18:31:05 Group: Name Of Some Buildpack: pass
# ...

Successfully built 2452b4b1fce1
Successfully tagged my-app:my-tag
```

The message `DETECTING WITH MANUALLY-PROVIDED GROUP` indicates that the buildpack was chosen by the user, rather than
by the automated detection process.

The `--buildpack` parameter can be
- a path to a directory
- a path to a `.tgz` file
- a URL to a `.tgz` file, or
- the ID of a buildpack located in a builder

### Building explained

![build diagram](docs/build.svg)

To create an app image, `build` executes one or more buildpacks against the app's source code.
Each buildpack inspects the source code and provides relevant dependencies. An image is then generated
from the app's source code and these dependencies.

Buildpacks are compatible with one or more [stacks](#managing-stacks). A stack designates a **build image**
and a **run image**. During the build process, a stack's build image becomes the environment in which buildpacks are
executed, and its run image becomes the base for the final app image. For more information on working with stacks, see
the [Managing stacks](#managing-stacks) section.

Buildpacks can be bundled together with a specific stack's build image, resulting in a
[builder](#working-with-builders-using-create-builder) image (note the "er" ending). Builders provide the most
convenient way to distribute buildpacks for a given stack. For more information on working with builders, see the
[Working with builders using `create-builder`](#working-with-builders-using-create-builder) section.

## Updating app images using `rebase`

The `pack rebase` command allows app developers to rapidly update an app image when its stack's run image has changed.
By using image layer rebasing, this command avoids the need to fully rebuild the app.

```bash
$ pack rebase <image-name>
```

### Example: Rebasing an app image

Consider an app image `my-app:my-tag` that was originally built using the default builder. That builder's stack has a
run image called `pack/run`. Running the following will update the base of `my-app:my-tag` with the latest version of
`pack/run`.

```bash
$ pack rebase my-app:my-tag
```

Like [`build`](#building-app-images-using-build), `rebase` has a `--publish` flag that can be
used to publish the updated app image to a registry.

### Rebasing explained

![rebase diagram](docs/rebase.svg)

At its core, image rebasing is a simple process. By inspecting an app image, `rebase` can determine whether or not a
newer version of the app's base image exists (either locally or in a registry). If so, `rebase` updates the app image's
layer metadata to reference the newer base image version.

## Working with builders using `create-builder`

`pack create-builder` enables buildpack authors and platform operators to bundle a collection of buildpacks into a
single image for distribution and use with a specified stack.

```bash
$ pack create-builder <image-name> --builder-config <path-to-builder-toml>
```

### Example: Creating a builder from buildpacks

In this example, a builder image is created from buildpacks `org.example.buildpack-1` and `org.example.buildpack-2`.
A `builder.toml` file provides necessary configuration to the command.

```toml
[[buildpacks]]
  id = "org.example.buildpack-1"
  uri = "relative/path/to/buildpack-1" # URIs without schemes are read as paths relative to builder.toml

[[buildpacks]]
  id = "org.example.buildpack-2"
  uri = "https://example.org/buildpacks/buildpack-2.tgz"

[[groups]]
  [[groups.buildpacks]]
    id = "org.example.buildpack-1"
    version = "0.0.1"
  
  [[groups.buildpacks]]
    id = "org.example.buildpack-2"
    version = "0.0.1"
```

Running `create-builder` while supplying this configuration file will produce the builder image.

```bash
$ pack create-builder my-builder:my-tag --builder-config path/to/builder.toml

2018/10/29 15:35:47 Pulling builder base image packs/build
2018/10/29 15:36:06 Successfully created builder image: my-builder:my-tag
```

Like [`build`](#building-app-images-using-build), `create-builder` has a `--publish` flag that can be used to publish
the generated builder image to a registry.

> The above example uses the default stack, whose build image is `packs/build`.
> The `--stack` parameter can be used to specify a different stack (currently, the only built-in stack is
> `io.buildpacks.stacks.bionic`). For more information about managing stacks and their associations with build and run
> images, see the [Managing stacks](#managing-stacks) section.

The builder can then be used in `build` by running:

```bash
$ pack build my-app:my-tag --builder my-builder:my-tag --buildpack org.example.buildpack-1
```

### Builders explained

![create-builder diagram](docs/create-builder.svg)

A builder is an image containing a collection of buildpacks that will be executed, in the order that they appear in
`builder.toml`, against app source code. This image's base will be the build image associated with a given stack.

> A buildpack's primary role is to inspect the source code, determine any
> dependencies that will be required to compile and/or run the app, and provide those dependencies as layers in the
> final app image. 
> 
> It's important to note that the buildpacks in a builder are not actually executed until
> [`build`](#building-explained) is run.

## Managing stacks

As mentioned [previously](#building-explained), a stack is associated with a build image and a run image. Stacks in
`pack`'s configuration can be managed using the following commands:

```bash
$ pack add-stack <stack-name> --build-image <build-image-name> --run-image <run-image-name1,run-image-name2,...>
```

```bash
$ pack update-stack <stack-name> --build-image <build-image-name> --run-image <run-image-name1,run-image-name2,...>
```

```bash
$ pack delete-stack <stack-name>
```

```bash
$ pack set-default-stack <stack-name>
```

> Technically, a stack can be associated with multiple run images, as a variant is needed for each registry to
> which an app image might be published when using `--publish`.

### Example: Adding a stack

In this example, a new stack called `org.example.my-stack` is added and associated with build image `my-stack/build`
and run image `my-stack/run`.

```bash
$ pack add-stack org.example.my-stack --build-image my-stack/build --run-image my-stack/run
```

### Example: Updating a stack

In this example, an existing stack called `org.example.my-stack` is updated with a new build image `my-stack/build:v2`
and a new run image `my-stack/run:v2`.

```bash
$ pack add-stack org.example.my-stack --build-image my-stack/build:v2 --run-image my-stack/run:v2
```

### Example: Deleting a stack

In this example, the existing stack `org.example.my-stack` is deleted from `pack`'s configuration.

```bash
$ pack delete-stack org.example.my-stack
```

### Example: Setting the default stack

In this example, the default stack, used by [`create-builder`](#working-with-builders-using-create-builder), is set to
`org.example.my-stack`.

```bash
$ pack set-default-stack org.example.my-stack
```

### Listing stacks

To inspect available stacks and their names (denoted by `id`), run:

```bash
$ cat ~/.pack/config.toml

...

[[stacks]]
  id = "io.buildpacks.stacks.bionic"
  build-images = ["packs/build"]
  run-images = ["packs/run"]

[[stacks]]
  id = "org.example.my-stack"
  build-images = ["my-stack/build"]
  run-images = ["my-stack/run"]

...
```

> Note that this method of inspecting available stacks will soon be replaced by a new command. The format of
> `config.toml` is subject to change at any time.

## Resources

- [Buildpack & Platform Specifications](https://github.com/buildpack/spec)

----

## Development

To run the tests, simply run:

```bash
$ go test
```
