FROM golang:1.14-nanoserver-1809 AS gobuild

# bake in a simple server util
WORKDIR /util
COPY server.go /util/server.go
RUN go build /util/server.go

FROM mcr.microsoft.com/windows/nanoserver:1809

COPY --from=gobuild /util/server.exe /util/server.exe

# placeholder values until correct values are deteremined
ENV CNB_USER_ID=0
ENV CNB_GROUP_ID=0

USER ContainerAdministrator

RUN net users /ADD pack /passwordreq:no /expires:never

LABEL io.buildpacks.stack.id=pack.test.stack
LABEL io.buildpacks.stack.mixins="[\"mixinA\", \"netcat\", \"mixin3\"]"

USER pack
