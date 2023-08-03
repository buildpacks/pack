FROM golang:1.20-alpine as builder
ARG pack_version
ENV PACK_VERSION=$pack_version
WORKDIR /app
COPY . .
RUN apk update && apk upgrade && apk add --no-cache make ca-certificates
RUN update-ca-certificates
RUN make build

FROM scratch
COPY --from=builder /app/out/pack /usr/local/bin/pack
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT [ "/usr/local/bin/pack" ]
