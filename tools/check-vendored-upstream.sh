#!/usr/bin/env bash
#
# Reports whether moby has changed daemon/volume/mounts since the revision
# internal/volume is pinned to.
#
# Prints the new commits to stdout and exits 1 when there are any, 0 when the
# pin is current. With --open-issue it also files (or comments on) a tracking
# issue; that needs gh to be authenticated with issues:write.
#
# Run it locally any time:  tools/check-vendored-upstream.sh

set -euo pipefail

UPSTREAM_PATH="daemon/volume/mounts"
ISSUE_TITLE="Vendored internal/volume is behind moby/moby ${UPSTREAM_PATH}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENDORED="${REPO_ROOT}/internal/volume/VENDORED.txt"

open_issue=false
[[ "${1:-}" == "--open-issue" ]] && open_issue=true

SHA="$(sed -n 's/^Commit SHA:[[:space:]]*//p' "${VENDORED}")"
DATE="$(sed -n 's/^Commit date:[[:space:]]*//p' "${VENDORED}")"

if [[ -z "${SHA}" || -z "${DATE}" ]]; then
  echo "error: could not read the pinned revision from ${VENDORED}" >&2
  exit 2
fi

echo "internal/volume is pinned to moby/moby@${SHA} (${DATE})"

# The pinned commit itself falls inside this window, so drop it. Anything else
# returned touched the vendored directory after we last took a copy.
commits="$(gh api "repos/moby/moby/commits?path=${UPSTREAM_PATH}&since=${DATE}T00:00:00Z" \
  --jq '.[] | "- \(.sha[0:12]) \(.commit.author.date[0:10]) \(.commit.message | split("\n")[0])"' \
  | { grep -v "^- ${SHA:0:12} " || true; })"

if [[ -z "${commits}" ]]; then
  echo "up to date: no upstream commits to ${UPSTREAM_PATH} since ${DATE}"
  exit 0
fi

echo "upstream has moved since the pinned revision:"
echo "${commits}"

if [[ "${open_issue}" != true ]]; then
  exit 1
fi

body_file="$(mktemp)"
trap 'rm -f "${body_file}"' EXIT

{
  echo "\`internal/volume\` is vendored from [moby/moby \`${UPSTREAM_PATH}\`](https://github.com/moby/moby/tree/${SHA}/${UPSTREAM_PATH}), pinned at \`${SHA}\` (${DATE})."
  echo
  echo "Upstream has changed since then:"
  echo
  echo "${commits}"
  echo
  echo "To take the changes:"
  echo
  echo "1. Update \`UPSTREAM_SHA\` and \`UPSTREAM_DATE\` in \`tools/vendor-volume-mounts.sh\`."
  echo "2. Run \`tools/vendor-volume-mounts.sh\`."
  echo "3. Reconcile \`internal/volume/mounts.go\` by hand -- it is reduced to the parsing subset and the script does not regenerate it. See \`internal/volume/VENDORED.txt\`."
  echo "4. Run \`make verify\` and \`go test ./internal/volume/ ./pkg/client/\`."
  echo
  echo "Not every upstream commit needs taking. Closing this as reviewed-and-not-needed is a fine outcome, but bump the pin anyway so the next run starts from here."
} > "${body_file}"

existing="$(gh issue list --state open --search "\"${ISSUE_TITLE}\" in:title" \
  --json number,title --jq "map(select(.title == \"${ISSUE_TITLE}\")) | .[0].number // empty")"

if [[ -n "${existing}" ]]; then
  echo "commenting on existing issue #${existing}"
  gh issue comment "${existing}" --body-file "${body_file}"
else
  echo "opening a new tracking issue"
  gh issue create --title "${ISSUE_TITLE}" --body-file "${body_file}" --label dependencies
fi

exit 1
