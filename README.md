# pack - Buildpack CLI [![Build Status](https://travis-ci.org/buildpack/pack.svg?branch=master)](https://travis-ci.org/buildpack/pack)

**pack** is a tool to create runnable images from applications using buildpacks.

For information on buildpacks: [buildpacks.io](https://buildpacks.io/)

## Example Usage

```
./pack build packs/myimage:mytag --path ./myapp
```

The above will create images on your local daemon. If you wish to export images directly to a docker registry, use the `--publish` flag.

```
./pack build packs/myimage:mytag --path ./myapp --publish
```
