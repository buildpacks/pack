FROM mcr.microsoft.com/windows/nanoserver:1809

# placeholder values until correct values are deteremined
ENV CNB_USER_ID=0
ENV CNB_GROUP_ID=0

USER ContainerAdministrator

RUN net users /ADD pack /passwordreq:no /expires:never

LABEL io.buildpacks.stack.id=pack.test.stack
LABEL io.buildpacks.stack.mixins="[\"mixinA\", \"build:mixinTwo\", \"netcat\", \"mixin3\"]"

USER pack
