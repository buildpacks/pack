#!/usr/bin/env bash
set -e
set -u

# ensure variable is set
: "$PACKAGE_NAME"
: "$GITHUB_WORKSPACE"

# setup non-root user
useradd -m archie

# add non-root user to sudoers
pacman -Sy --noconfirm sudo
echo 'archie ALL=(ALL:ALL) NOPASSWD:ALL' >> /etc/sudoers

# run everything else as non-root user
su archie << "EOF"
# setup workspace
WORKSPACE=$GITHUB_WORKSPACE/$PACKAGE_NAME
sudo chown -R archie $WORKSPACE
sudo chmod -R +w $WORKSPACE
cd $WORKSPACE

# debug info
ls -al
sha512sum ./*

# setup AUR packaging deps
sudo pacman -Sy --noconfirm git base-devel libffi

# install package
makepkg -sri --noconfirm
EOF