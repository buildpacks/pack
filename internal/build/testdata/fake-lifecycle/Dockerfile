FROM golang

RUN mkdir /lifecycle
WORKDIR /go/src/step
COPY . .
RUN GO111MODULE=on go build -mod=vendor -o /lifecycle/phase ./phase.go

RUN mkdir -p /buildpacks
RUN echo '[[groups]]\n\
[[groups.buildpacks]]\n\
id = orig.buildpack.id\n\
version = orig.buildpack.version\n'\
> /buildpacks/order.toml

ENV CNB_USER_ID 111
ENV CNB_GROUP_ID 222

LABEL io.buildpacks.stack.id="test.stack"
LABEL io.buildpacks.builder.metadata='{"buildpacks":[{"id":"orig.buildpack.id","version":"orig.buildpack.version"}],"groups":[{"buildpacks":[{"id":"orig.buildpack.id","version":"orig.buildpack.version"}]}],"stack":{"runImage":{"image":"run/image"}}}'