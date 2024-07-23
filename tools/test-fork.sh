#!/usr/bin/env bash

# $1 - registry repo name

echo "Parse registry: $1"
firstPart=$(echo "$1" | cut -d/ -f1)
secondPart=$(echo "$1" | cut -d/ -f2)
thirdPart=$(echo "$1" | cut -d/ -f3)

registry=""
username=""
reponame=""
if [[ -z $thirdPart ]]; then # assume Docker Hub
  registry="index.docker.io"
  username=$firstPart
  reponame=$secondPart
else
  registry=$firstPart
  username=$secondPart
  reponame=$thirdPart
fi

echo "Disabling workflows that should not run on the forked repository"
disable=(
  delivery-archlinux-git.yml
  delivery-archlinux.yml
  delivery-chocolatey.yml
  delivery-homebrew.yml
  delivery-release-dispatch.yml
  delivery-ubuntu.yml
  privileged-pr-process.yml
)
for d in "${disable[@]}"; do
  if [ -e "$d" ]; then
    mv ".github/workflows/$d" ".github/workflows/$d.disabled"
  fi
done

echo "Removing upstream maintainers from the benchmark alert CC"
sed -i '' "/          alert-comment-cc-users:/d" .github/workflows/benchmark.yml
