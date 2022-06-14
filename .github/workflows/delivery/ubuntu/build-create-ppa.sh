#!/usr/bin/env bash

set -e
set -o pipefail

readonly PROG_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PACKAGE_NAME="pack-cli"
readonly MAINTAINER="cncf-buildpacks"
readonly MAINTANER_EMAIL="cncf-buildpacks-maintainers@lists.cncf.io"

# verify the following are set.
: "$PACKAGE_VERSION"
: "$GITHUB_WORKSPACE"
: "$GO_DEP_PACKAGE_NAME"

function dependencies() {
    apt-get update
    apt-get install software-properties-common -y
    add-apt-repository ppa:longsleep/golang-backports -y
    apt-get update
    apt-get install gnupg dput dh-make devscripts lintian $GO_DEP_PACKAGE_NAME -y
}

function main() {
    # import secrets needed to sign packages we build with debuild
    import_gpg

    # vendor local dependencies. Otherwise these fail to be pulled in during
    # the Launchpad build process
    go mod vendor

    # set up debian user info.
    export DEBEMAIL=$MAINTAINER_EMAIL
    export DEBFULLNAME=$MAINTAINER
    echo "creating package: ${PACKAGE_NAME}_${PACKAGE_VERSION}"

    # generate the skeleton of a debian package.
    dh_make -p "${PACKAGE_NAME}_${PACKAGE_VERSION}" --single --native --copyright apache --email "${MAINTAINER_EMAIL}" -y

    # copy our templated configuration files.
    cp "$PROG_DIR/debian/"* debian/

    echo "======="
    echo "compat"
    echo "======="
    cat debian/compat
    echo
    echo "======="
    echo "changelog"
    echo "======="
    cat debian/changelog
    echo
    echo "======="
    echo "control"
    echo "======="
    cat debian/control
    echo
    echo "======="
    echo "rules"
    echo "======="
    cat debian/rules
    echo
    echo "======="
    echo "copyright"
    echo "======="
    cat debian/copyright
    echo

    # Remove empty default files created by dh_make
    rm -f debian/*.ex
    rm -f debian/*.EX
    rm -f debian/README.*

    # build a source based debian package, Ubuntu ONLY accepts source packages.
    debuild -S
}

# import gpg keys from env
function import_gpg() {
  # verify the following are set.
  : "$GPG_PUBLIC_KEY"
  : "$GPG_PRIVATE_KEY"

  echo "Importing GPG keys..."
  gpg --import <(echo "$GPG_PUBLIC_KEY")
  gpg --allow-secret-key-import --import <(echo "$GPG_PRIVATE_KEY")
}

dependencies
main
