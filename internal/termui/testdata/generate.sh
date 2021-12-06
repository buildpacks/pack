#!/usr/bin/env bash

dir=$(cd $(dirname $0) && pwd)
mkdir -p $dir/layers/some_buildpack-1
mkdir -p $dir/layers/some_buildpack-2/some-dir

echo -n "some-content-1" > $dir/layers/some_buildpack-1/some-file-1.txt
echo -n "some-content-2" > $dir/layers/some_buildpack-2/some-dir/some-file-2.txt

tar cvf $dir/fake-layers.tar layers
rm -rf $dir/layers