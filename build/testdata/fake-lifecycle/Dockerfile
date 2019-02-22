FROM golang

RUN mkdir /lifecycle
WORKDIR /go/src/step
COPY . .
RUN GO111MODULE=on go build -mod=vendor -o /lifecycle/phase ./phase.go

RUN mkdir -p /buildpacks
RUN echo -n "original-order-toml" > /buildpacks/order.toml

ENV PACK_USER_ID 111
ENV PACK_GROUP_ID 222