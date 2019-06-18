# pack - Buildpack CLI [![Travis Build Status](https://travis-ci.org/buildpack/pack.svg?branch=master)](https://travis-ci.org/buildpack/pack)

**`pack`** makes it easy for
- **Application developers** to use [Cloud Native Buildpacks](https://buildpacks.io/) to convert code into runnable images
- **Buildpack authors** to develop and package buildpacks for distribution

## Resources

- [Get Started with `pack`](https://buildpacks.io/docs/app-journey)
- [Latest `pack` Documentation](https://buildpacks.io/docs/using-pack)
- [Buildpack & Platform Specifications](https://github.com/buildpack/spec)

## Development

### Building

To build pack:
```
$ make build
```

This will output the binary to the directory `out`.

Set the `PACK_BIN` environment variable prior to running to change the name or location of the binary within `out`.
Default is `pack`.

Set the `PACK_VERSION` environment variable to tell `pack` what version to consider itself. Default is `dev`.

> This project uses [go modules](https://github.com/golang/go/wiki/Modules)

### Testing

To run unit and integration tests:

```bash
$ make unit
```

To run acceptance tests:
```bash
$ make acceptance
```

Alternately, to run all tests:
```bash
$ make test
```