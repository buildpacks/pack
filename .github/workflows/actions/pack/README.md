![test](https://github.com/dfreilich/pack-action/workflows/test/badge.svg)

# Pack Github Action
This Github Action uses the [Pack CLI](https://github.com/buildpacks/pack) (a product of the [Cloud Native Buildpacks project](https://buildpacks.io)) to build applications and associated artifacts, without a Dockerfile. For more about pack concepts, see the [pack docs](https://buildpacks.io/docs). For `pack` usage specific documentation, see the [Pack CLI docs](https://buildpacks.io/docs/reference/pack/pack/).

## Usage
### Help
```yaml
jobs:
  test:
    runs-on: linux
    steps:
      - name: Checkout
        uses: actions/checkout@v2
      - name: Pack Help
        uses: dfreilich/pack-action@v1
        with:
          args: help
```

### Local Build
```yaml
jobs:
  local_build:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v2
      - name: Pack Build
        uses: dfreilich/pack-action@v1
        with:
          args: 'build test_img --builder paketobuildpacks/builder:full'
```

For a list of suggested builders, run:
```shell script
$ pack suggest-builders
```
on your local machine.

### Remote Build
```yaml
  dockerhub_remote_build:
    runs-on: ubuntu-latest
    env:
      USERNAME: '<SOMETHING>'
      IMG_NAME: '<SOME_IMG>'
    steps:
      - name: Checkout
        uses: actions/checkout@v2
      - name: Set App Name
        run: 'echo "::set-env name=IMG_NAME::$(echo ${USERNAME})/$(echo ${IMG_NAME})"'
      - name: Pack Remote Build
        uses: dfreilich/pack-action@v1
        with:
          args: 'build ${{ env.IMG_NAME }} --builder paketobuildpacks/builder:full --publish'
          username: ${{ env.USERNAME }}
          password: ${{ secrets.<TOKEN> }}
```

If you are publishing to a registry that is not Docker Hub, you can also add in an optional `registry` argument:
```yaml
  github_registry_remote_build:
    runs-on: ubuntu-latest
    env:
      USERNAME: '<NAME>'
      IMG_NAME: '<IMAGE>'
      REGISTRY: '<REGISTRY: ex. ghcr.io>'
    steps:
      - name: Checkout
        uses: actions/checkout@v2
      - name: Set App Name
        run: 'echo "::set-env name=IMG_NAME::$(echo ${REGISTRY})/$(echo ${USERNAME})/$(echo ${IMG_NAME})"'
      - name: Pack Remote Build
        uses: dfreilich/pack-action@v1
        with:
          args: 'build ${{ env.IMG_NAME }} --builder ${{ env.BUILDER }} --publish'
          username: ${{ env.USERNAME }}
          password: ${{ secrets.<GH_PACKAGES TOKEN> }}
          registry: ${{ env.REGISTRY }}
```

### Remote Builds Using [docker/login-action](https://github.com/docker/login-action)
Alternatively (in cases where that doesn't work, such as ECR), users can login separately, using either the `docker cli` directly, or the [docker/login-action](https://github.com/docker/login-action).

> NOTE: If you use the [docker/login-action](https://github.com/docker/login-action), you need to add a job container, in order for the Pack Github Action to access the docker credentials. An example of that is below. However, that can have side-effects, and isn't recommended unless necessary.
```yaml
  dockerhub_remote_build:
    runs-on: ubuntu-latest
    container:
      image: docker:stable
      volumes:
        - /home/runner:/var/www
    env:
      USERNAME: '<SOMETHING>'
      IMG_NAME: '<SOME_IMG>'
    steps:
      - name: Checkout
        uses: actions/checkout@v2
      - name: Set App Name
        run: 'echo "::set-env name=IMG_NAME::$(echo ${USERNAME})/$(echo ${IMG_NAME})"'
      - name: Login to Dockerhub
        uses: docker/login-action@v1
        with:
          username: ${{ env.USERNAME }}
          password: ${{ secrets.DOCKER_TOKEN }}
      - name: Pack Remote Build
        uses: dfreilich/pack-action@v1
        with:
          args: 'build ${{ env.IMG_NAME }} --builder paketobuildpacks/builder:full --publish'
```


### More Examples
For more examples, see the [test workflows](.github/workflows/main.yml).

## Inputs
### `args`
**Required** The arguments to pass into `pack`. A list of available commands can be found [here](https://buildpacks.io/docs/reference/pack/pack/).

### `username`
**Optional** Username used to log in to a Docker registry. If not set, then no login will occur.

### `password`
**Optional** Password or personal access token used to log in to a Docker registry. If not set, then no login will occur.

### `registry`
**Optional** Server address of Docker registry. If not set then will default to Docker Hub.

## How can I help ?
Any contribution is welcome! The most basic way to show your support is to star :star2: the project, or to raise issues :speech_balloon:.

## License
Apache. See [LICENSE](LICENSE) for more details.