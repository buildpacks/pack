# `STAGE` package

The PACKAGE `stage` is similar to docker's [multi-stage](https://docs.docker.com/build/guide/multi-stage/).Using multi-stage builds, you can choose to use different base images for your build and runtime environments. You can copy build artifacts from the build stage over to the runtime stage.

This package is a superset of `Packerfile` package where each stage can INSTRUCT using set of commands exposed by _Packerfile_ package. Possible to build multi-arch stages.

Each stage can be either be Converted Into an Image or an ImageIndex based on user needs.
