# An App's Brief Journey from Source to Image

In this tutorial, we'll explain how to use `pack` and [Cloud Native Buildpacks](https://buildpacks.io/) to
create a runnable app image from source code.

## What is a buildpack?

Let's start with the basics. A buildpack is something you've likely leveraged without knowing it, as they're already
being used in many cloud platforms. A buildpack's job is to gather dependencies that your app needs to build and run,
and a buildpack often performs this job quickly and quietly.

That said, while buildpacks are often a behind-the-scenes detail, they are extremely important.

## Auto-detection

What enables buildpacks to often go unnoticed is auto-detection. Auto-detection happens when a platform sequentially
tests groups of buildpacks against your app's source code. The first group that deems itself fit for your source code
will become the selected set of buildpacks for your app. Detection criteria is specific to each buildpack -- for
instance, an NPM buildpack might look for a `package.json`, and a Go buildpack might look for Go source files.

Let's see auto-detection in action by running `pack build` against a simple Java application.

## `pack` for the journey

Start by installing `pack`. Simply download the appropriate binary from the latest
[GitHub release](https://github.com/buildpack/pack/releases) and move it somewhere on your `$PATH`.

If you're on MacOS, you can use Homebrew instead:

```bash
$ brew tap buildpack/tap
$ brew install pack
```

> `pack` requires [Docker](https://www.docker.com/get-started) to run, so make sure you've got that installed too.

## Next stop, the end

Next, clone or download this simple [Java app source code](https://github.com/buildpack/sample-java-app) to a location
of your choosing.

Now run the following commands in a shell:

```bash
$ cd path/to/sample-java-app
$ pack build myapp
```

> If this is your first time running `pack`, you might see a message about selecting a default
> [builder](../../README.md#working-with-builders-using-create-builder) (essentially, an image containing buildpacks). Simply choose
> either builder presented by running `pack set-default-builder <builder>`, then run `pack build myapp` again.
>
> (Please note that currently the
> [Heroku Java buildpack](https://github.com/heroku/java-buildpack#heroku-cloud-native-buildpack-for-java)
> only supports Maven)

That's it. You've now got a runnable app image called `myapp` available on your local Docker daemon.
We did say this was a "brief" journey after all! Take note that your app was built without any need for you to install
a JDK, run Maven, or otherwise configure a build environment. `pack` and the buildpack took care of all that for you.

## Beyond the journey

To test out your new app image locally, you can run it with Docker:

```bash
$ docker run --rm -p 8080:8080 myapp
```

Now just hit `localhost:8080` in your favorite browser and take a minute to admire your handiwork.

### Take your image to the skies

`pack` uses Cloud Native Buildpacks to help you easily create OCI images that you can run just about anywhere. Try
deploying your new image to your favorite cloud!

> In case you need it, `pack build` has a handy flag called `--publish` that will publish your app image to a Docker
> registry after building it.