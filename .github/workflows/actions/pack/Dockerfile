FROM buildpacksio/pack

COPY --from=busybox:latest /bin /bin
COPY --from=docker:stable /usr/local/bin/docker /usr/local/bin/docker
COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]