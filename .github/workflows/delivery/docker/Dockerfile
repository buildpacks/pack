FROM alpine:3.10

ARG PACK_VERSION
ENV PACK_URL https://github.com/buildpack/pack/releases/download/${PACK_VERSION}/pack-${PACK_VERSION}-linux.tgz
RUN wget -O- "${PACK_URL}" | tar -C /usr/local/bin -xz pack
ENTRYPOINT ["/usr/local/bin/pack"]
