ARG base=ubuntu:18.04
ARG go_version=1.11.4

FROM golang:${go_version} as builder

WORKDIR /go/src/github.com/buildpack/lifecycle
COPY . .
RUN CGO_ENABLED=0 GO111MODULE=on go install -mod=vendor -a -installsuffix static "./cmd/..."

FROM ${base}
ARG jq_url=https://github.com/stedolan/jq/releases/download/jq-1.5/jq-linux64
ARG yj_url=https://github.com/sclevine/yj/releases/download/v2.0/yj-linux
ARG pack_uid=1000
ARG pack_gid=1000


RUN apt-get update && \
  apt-get install -y wget xz-utils ca-certificates && \
  rm -rf /var/lib/apt/lists/*

RUN \
  groupadd pack --gid ${pack_gid} && \
  useradd --uid ${pack_uid} --gid ${pack_gid} -m -s /bin/bash pack

ENV PACK_USER_ID=${pack_uid}
ENV PACK_GROUP_ID=${pack_gid}
ENV PACK_USER_GID=${pack_gid}

COPY --from=builder /go/bin /lifecycle

RUN wget -qO /usr/local/bin/jq "${jq_url}" && chmod +x /usr/local/bin/jq && \
  wget -qO /usr/local/bin/yj "${yj_url}" && chmod +x /usr/local/bin/yj

WORKDIR /workspace
RUN chown -R pack:pack /workspace
