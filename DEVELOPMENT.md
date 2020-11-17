# Development

## Prerequisites

* [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)
    * macOS: _(built-in)_
    * Windows:
        * `choco install git -y`
        * `git config --global core.autocrlf false`
* [Go](https://golang.org/doc/install)
    * macOS: `brew install go`
    * Windows: `choco install golang -y`
* [Docker](https://www.docker.com/products/docker-desktop)
* Make (and build tools)
    * macOS: `xcode-select --install`
    * Windows:
        * `choco install cygwin make -y`
        * `[Environment]::SetEnvironmentVariable("PATH", "C:\tools\cygwin\bin;$ENV:PATH", "MACHINE")`

### Windows Caveats

* Symlinks - Some of our tests attempt to create symlinks. On Windows, this requires the [permission to be provided](https://stackoverflow.com/a/24353758).

## Tasks

### Building

To build pack:
```
make build
```

This will output the binary to the directory `out/`.

Options:

| ENV_VAR      | Description                                                            | Default |
|--------------|------------------------------------------------------------------------|---------|
| GOCMD        | Change the `go` executable. For example, [richgo][rgo] for testing.    | go      |
| PACK_BIN     | Change the name or location of the binary relative to `out/`.          | pack    |
| PACK_VERSION | Tell `pack` what version to consider itself                            | `dev`   |

[rgo]: https://github.com/kyoh86/richgo

_NOTE: This project uses [go modules](https://github.com/golang/go/wiki/Modules) for dependency management._

### Testing

To run unit and integration tests:
```shell
make unit
```

To run acceptance tests:
```shell
make acceptance
```

Alternately, to run all tests:
```shell
make test
```

### Tidy

To format the code:
```shell
make format
```

To tidy up the codebase and dependencies:
```shell
make tidy
```

### Verification

To verify formatting and code quality:
```shell
make verify
```

### Prepare for PR

Runs various checks to ensure compliance:
```shell
make prepare-for-pr
```