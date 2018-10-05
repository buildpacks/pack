# pack - Buildpack CLI [![Build Status](https://travis-ci.org/buildpack/pack.svg?branch=master)](https://travis-ci.org/buildpack/pack)

**pack** is a tool to create runnable images from applications using buildpacks.

For information on buildpacks: [buildpacks.io](https://buildpacks.io/)

## Example Usage

Currently we recommend using the development detect image

```
./pack build <REPONAME> [-p <PATH to APP>] --detect-image packsdev/v3:detect
```

The above will create images on your local daemon. If you wish to create images on a docker registry, use the `--publish` flag.

```
./pack build myorg/myapp -p acceptance/fixtures/node_app --detect-image packsdev/v3:detect --publish
```
