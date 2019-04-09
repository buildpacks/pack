ARG lifecycleVersion="0.1.0"
FROM packs/run:${lifecycleVersion}

USER root
RUN apt-get update && apt-get -yq install netcat
USER pack

LABEL io.buildpacks.stack.id=pack.test.stack

