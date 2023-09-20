FROM golang:1.20 as builder
ARG pack_version
ENV PACK_VERSION=$pack_version
WORKDIR /app
COPY . .
RUN make build

FROM scratch
COPY --from=builder /app/out/pack /usr/local/bin/pack
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /tmp /tmp
ENTRYPOINT [ "/usr/local/bin/pack" ]
