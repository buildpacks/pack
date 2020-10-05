#!/bin/sh
if [ -n "${INPUT_USERNAME}" ] && [ -n "${INPUT_PASSWORD}" ]; then
  echo "${INPUT_PASSWORD}" | docker login -u "${INPUT_USERNAME}" --password-stdin "${INPUT_REGISTRY}"
fi

pack_exe=$(command -v pack)
if [ -z "${pack_exe}" ]; then
  pack_exe=$(command -v web)
  if [ -z "${pack_exe}" ]; then
    echo "failed to find the pack executable"
    exit 1
  fi
fi

eval "${pack_exe} ${INPUT_ARGS}"
