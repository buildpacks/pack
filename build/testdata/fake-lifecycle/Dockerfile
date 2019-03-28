FROM golang

RUN mkdir /lifecycle
WORKDIR /go/src/step
COPY . .
RUN GO111MODULE=on go build -mod=vendor -o /lifecycle/phase ./phase.go

RUN mkdir -p /buildpacks
RUN echo -n "original-order-toml" > /buildpacks/order.toml

ENV CNB_USER_ID 111
ENV CNB_GROUP_ID 222