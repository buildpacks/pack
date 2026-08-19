# syntax=docker/dockerfile:1
ARG base_image=gcr.io/distroless/static

FROM golang:1.25 as builder
ARG pack_version
ENV PACK_VERSION=$pack_version
WORKDIR /app
COPY . .
RUN make build

FROM ${base_image}
# Ensure CA certificates are present so pack can make TLS connections (e.g. to
# pull builder and run images from registries). The distroless base bundles
# them, but ubuntu:jammy used for the -base image does not, so copy the bundle
# from the builder stage to cover every base image. See #2488.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /app/out/pack /usr/local/bin/pack
ENTRYPOINT [ "/usr/local/bin/pack" ]
