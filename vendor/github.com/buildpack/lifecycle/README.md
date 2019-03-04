# Lifecycle

[![Build Status](https://travis-ci.org/buildpack/lifecycle.svg?branch=master)](https://travis-ci.org/buildpack/lifecycle)
[![GoDoc](https://godoc.org/github.com/buildpack/lifecycle?status.svg)](https://godoc.org/github.com/buildpack/lifecycle)

A reference implementation of [Buildpack API v3](https://github.com/buildpack/spec).

## Commands

### Build

* `detector` - chooses buildpacks (via `/bin/detect`)
* `analyzer` - restores launch layer metadata from the previous build
* `builder` -  executes buildpacks (via `/bin/build`)
* `exporter` - remotely patches images with new layers (via rebase & append)
* `launcher` - invokes choice of process

### Develop

* `detector` - chooses buildpacks (via `/bin/detect`)
* `developer` - executes buildpacks (via `/bin/develop`)
* `launcher` - invokes choice of process

### Cache

* `retriever` - restores cache
* `cacher` - updates cache

## Notes

Cache implementations (`retriever` and `cacher`) are intended to be interchangable and platform-specific.
A platform may choose not to deduplicate cache layers.
