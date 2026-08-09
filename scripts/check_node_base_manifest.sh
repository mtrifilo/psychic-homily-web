#!/usr/bin/env bash
# scripts/check_node_base_manifest.sh
#
# Assert that backend/Dockerfile's pinned Node digest is a MANIFEST LIST (an OCI
# image index) covering both architectures this project builds for.
#
# This is the one failure a `--platform linux/amd64` build cannot see. The
# Dockerfile's NODE PIN comment requires the index digest, not a per-platform
# one, because Railway builds linux/amd64 while local dev is usually
# linux/arm64 and a single-platform digest fails the pull on the other arch. An
# amd64-specific digest would sail through CI's build and break every arm64
# laptop instead -- which is the slowest possible way to find out.
#
# Now that Dependabot writes these digests (.github/dependabot.yml), "the bot
# always picks the index digest" is an assumption worth asserting rather than
# believing.
#
# Usage:
#   scripts/check_node_base_manifest.sh
#
# Requires network access to the registry, plus docker buildx and jq. That
# network dependency is exactly why this is NOT part of the always-run pin gate:
# a Docker Hub outage must not block PRs that never touched the image. It runs
# in the Backend Image Build workflow, which is path-filtered to the PRs where a
# digest can actually have changed.
#
# The pinned reference comes from scripts/check_node_base_pin.sh --print-ref, so
# there is exactly one parser of the Dockerfile and this check inherits its
# shape validation (all node stages identical, full patch tag, digest present).

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

ref=$("$script_dir/check_node_base_pin.sh" --print-ref)
echo "Inspecting $ref"

# One retry. On Dependabot-authored PRs the Docker Hub login is skipped (those
# runs have no DOCKERHUB_* secrets), so the pull is anonymous and shares a
# per-runner-IP rate limit. A 429 here is not a finding about the digest, and
# reddening the job for it would burn the check on precisely the PRs it exists
# to gate.
raw=""
for attempt in 1 2; do
  if raw=$(docker buildx imagetools inspect "$ref" --raw 2>&1); then
    break
  fi
  if [ "$attempt" -eq 2 ]; then
    echo "ERROR: could not read the manifest for $ref after 2 attempts." >&2
    printf '%s\n' "$raw" | sed 's|^|         |' >&2
    echo "       If this is a registry rate limit or outage, re-run. If the digest" >&2
    echo "       genuinely does not exist, the image build below will fail too." >&2
    exit 1
  fi
  echo "Registry read failed, retrying in 15s..." >&2
  sleep 15
done

media=$(printf '%s' "$raw" | jq -r '.mediaType // ""')
case "$media" in
  application/vnd.oci.image.index.v1+json | application/vnd.docker.distribution.manifest.list.v2+json) ;;
  *)
    echo "ERROR: $ref resolves to '$media', not a manifest list." >&2
    echo "       That is a per-platform digest. It will pull on one architecture" >&2
    echo "       and fail on the other." >&2
    echo "       Re-pin using the TOP-LEVEL 'Digest:' from" >&2
    echo "         docker buildx imagetools inspect node:<version>-alpine<ver>" >&2
    exit 1
    ;;
esac

have=$(printf '%s' "$raw" \
       | jq -r '.manifests[] | select(.platform.os != null) | "\(.platform.os)/\(.platform.architecture)"')

for want in linux/amd64 linux/arm64; do
  if ! printf '%s\n' "$have" | grep -qx "$want"; then
    echo "ERROR: $ref has no $want manifest. It covers:" >&2
    printf '%s\n' "$have" | sed 's|^|         |' >&2
    echo "       Railway deploys linux/amd64 and local dev is usually linux/arm64;" >&2
    echo "       both have to resolve from the same pinned digest." >&2
    exit 1
  fi
done

echo "OK: manifest list ($media) covers linux/amd64 and linux/arm64"
