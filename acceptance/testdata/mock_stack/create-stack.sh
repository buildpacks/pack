#!/usr/bin/env bash

dir="$(cd $(dirname $0) && pwd)"

docker build --tag test/build "$dir/build"
docker build --tag test/run "$dir/run"