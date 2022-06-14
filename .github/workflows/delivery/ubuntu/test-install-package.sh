#!/usr/bin/env bash
set -e
set -o pipefail

# verify the following are set.
: "$GO_DEP_PACKAGE_NAME"

apt-get update

# install neede packaging utilities
DEBIAN_FRONTEND=noninteractive apt-get install git devscripts debhelper software-properties-common -y

# verify GITHUB_WORKSPACE is set up (we are in an action)
: "$GITHUB_WORKSPACE"

# make and move package source into a testing directory
testdir="$(mktemp -d)"

cp -R $GITHUB_WORKSPACE/* $testdir
pushd $testdir

# install golang using ppa
add-apt-repository ppa:longsleep/golang-backports -y
apt-get update
apt-get install $GO_DEP_PACKAGE_NAME -y

# build a debian binary package
debuild -b -us -uc

# install the binary package
dpkg -i ../*.deb

# list contents installed by the build debain package
dpkg -L pack-cli

popd
