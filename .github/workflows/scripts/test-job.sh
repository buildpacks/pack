#!/usr/bin/env bash
set -e

: ${GITHUB_TOKEN?"Need to set GITHUB_TOKEN env var."}

usage() {
  echo "Usage: "
  echo "  $0 <workflow> <job>"
  echo "    <workflow>  the workflow file to use"
  echo "    <job>  job name to execute"
  exit 1; 
}

WORKFLOW_FILE="${1}"
if [[ -z "${WORKFLOW_FILE}" ]]; then
  echo "Must specify a workflow file"
  echo
  usage
  exit 1
fi

JOB_NAME="${2}"
if [[ -z "${JOB_NAME}" ]]; then
  echo "Must specify a job"
  echo
  usage
  exit 1
fi

ACT_EXEC=/Users/javier.romero/dev/nektos/act/dist/local/act
if [[ -z "${ACT_EXEC}" ]]; then
  echo "Need act to be available: https://github.com/nektos/act"
  exit 1
fi

${ACT_EXEC} \
  -v \
  -P ubuntu-latest=nektos/act-environments-ubuntu:18.04 \
  -s GITHUB_TOKEN \
  -W "${WORKFLOW_FILE}" \
  -j "${JOB_NAME}"

#act -P ubuntu-latest=nektos/act-environments-ubuntu:18.04 \
#    -e .github/workflows/testdata/event-release.json \
#    -s GITHUB_TOKEN \
#    -j "${JOB_NAME}"
