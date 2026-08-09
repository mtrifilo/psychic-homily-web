#!/usr/bin/env bash
# scripts/check_node_base_pin.sh
#
# The single source of truth for which Node the graph-layout worker runs.
#
# backend/Dockerfile pins the runtime base to a full patch tag plus the
# manifest-list digest, because ForceAtlas2 float output is a V8 observable (see
# the NODE PIN comment there). CI has to run backend/layout's `node --test` on
# that same V8, or the determinism assertions are evidence about nothing.
#
# CI used to restate the version in a `node-version:` literal, which put one
# fact in two files. Dependabot edits Dockerfiles and knows nothing about
# workflow YAML, so every automated bump would have moved one half and left the
# other behind. Rather than detect that drift, this script removes it: the
# workflow asks
#
#     scripts/check_node_base_pin.sh --print-version
#
# and hands the answer to actions/setup-node. There is no second value to keep
# in sync, so a Dependabot PR that touches only the Dockerfile is a COMPLETE
# bump.
#
# What it does still enforce, because these are choosable and being wrong about
# them is silent:
#
#   * every `FROM node:` stage pins the identical reference -- the npm that
#     resolves backend/layout/node_modules has to be the node that later runs
#     the script
#   * the tag is a full patch, not a floating major or minor
#   * a digest is present -- even a full patch tag gets republished when its
#     Alpine base is patched, so the tag on its own pins nothing
#
# Deliberately NOT enforced: how many node stages there are. The invariant is
# that they agree, not that there are two of them, and pinning the stage count
# would fail CI on a legitimate restructuring with nothing actually wrong.
#
# Usage:
#   scripts/check_node_base_pin.sh                  # validate + report
#   scripts/check_node_base_pin.sh --print-version  # 22.23.2
#   scripts/check_node_base_pin.sh --print-ref      # node:22.23.2-alpine3.24@sha256:...
#
# Validation runs in every mode; the print modes only replace the report with
# the bare value, so no caller can consume an unvalidated pin. No network I/O
# and no git plumbing, so it behaves identically in CI and on a laptop.
#
# bash 3.2 compatible (macOS system bash): no mapfile, no associative arrays.

set -euo pipefail

mode=report
case "${1:-}" in
  '') ;;
  --print-version) mode=version ;;
  --print-ref) mode=ref ;;
  *)
    echo "usage: $(basename "$0") [--print-version|--print-ref]" >&2
    exit 2
    ;;
esac

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
dockerfile="$repo_root/backend/Dockerfile"

if [ ! -f "$dockerfile" ]; then
  echo "ERROR: expected file not found: $dockerfile" >&2
  exit 1
fi

refs=$(grep -E '^FROM[[:space:]]+node:' "$dockerfile" \
       | sed -E 's/^FROM[[:space:]]+([^[:space:]]+).*/\1/' || true)

if [ -z "$refs" ]; then
  echo "ERROR: backend/Dockerfile has no 'FROM node:...' stage." >&2
  echo "       The graph-layout worker needs a Node runtime, and CI derives its" >&2
  echo "       own Node version from that line. If the worker genuinely stopped" >&2
  echo "       needing Node, retire this gate and its caller together." >&2
  exit 1
fi

image_ref=${refs%%$'\n'*}

while IFS= read -r ref; do
  [ "$ref" = "$image_ref" ] && continue
  echo "ERROR: the 'FROM node:...' stages in backend/Dockerfile disagree:" >&2
  printf '%s\n' "$refs" | sed 's|^|         |' >&2
  echo "       Every stage must pin the identical tag AND digest -- the npm that" >&2
  echo "       resolves backend/layout/node_modules has to be the node that runs it." >&2
  exit 1
done <<<"$refs"

if ! printf '%s' "$image_ref" \
     | grep -qE '^node:[0-9]+\.[0-9]+\.[0-9]+-alpine[0-9]+\.[0-9]+@sha256:[0-9a-f]{64}$'; then
  echo "ERROR: backend/Dockerfile's node base is not pinned in the expected form." >&2
  echo "       found:    $image_ref" >&2
  echo "       expected: node:<major>.<minor>.<patch>-alpine<x>.<y>@sha256:<64 hex>" >&2
  echo "       The full patch tag documents the version for a human; the digest is" >&2
  echo "       what makes the pull immutable. Neither half is optional." >&2
  echo "       See the NODE PIN comment in backend/Dockerfile." >&2
  exit 1
fi

version=$(printf '%s' "$image_ref" | sed -E 's/^node:([0-9]+\.[0-9]+\.[0-9]+)-alpine.*/\1/')

case "$mode" in
  version) printf '%s\n' "$version" ;;
  ref)     printf '%s\n' "$image_ref" ;;
  report)
    echo "OK: backend/Dockerfile pins node $version"
    echo "    image: $image_ref"
    ;;
esac
