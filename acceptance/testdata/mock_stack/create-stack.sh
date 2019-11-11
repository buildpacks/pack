#!/usr/bin/env bash

dir="$(cd $(dirname $0) && pwd)"

docker build --tag pack-test/build "$dir"/build
docker build --tag pack-test/run "$dir"/run
