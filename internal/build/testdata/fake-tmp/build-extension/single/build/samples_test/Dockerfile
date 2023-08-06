ARG base_image
FROM ${base_image}

USER root
# Update and upgrade the Alpine packages
RUN apk update && apk upgrade

# Install packages
RUN apk add --no-cache     build-base     openssl     libffi-dev     python3     tree

# Print package versions
RUN gcc --version &&     openssl version &&     python3 --version

# Remove packages
RUN apk del tree

# Install additional packages
RUN apk add --no-cache     postgresql-client     git     curl

# Print package versions
RUN psql --version &&     git --version &&     curl --version

# ENV variables
ENV ENV_VARIABLE=value
