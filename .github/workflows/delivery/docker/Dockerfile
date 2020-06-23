FROM alpine:3.10

ARG VERSION
ENV PACK_URL "https://github.com/buildpack/pack/releases/download/v${VERSION}/pack-v${VERSION}-linux.tgz"
RUN wget -O- "${PACK_URL}" | tar -C /usr/local/bin -xz pack
ENTRYPOINT ["/usr/local/bin/pack"]
