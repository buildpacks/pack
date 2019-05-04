#!/usr/bin/env bash

dir="$(cd $(dirname $0) && pwd)"

docker build --tag pack-test/build "$dir"
docker tag pack-test/build pack-test/run