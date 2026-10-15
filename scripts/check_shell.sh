#!/usr/bin/env bash
# scripts/check_shell.sh
#
# CI gate: shellcheck at warning severity over the repository's shell
# scripts. This file is the single definition of the checked set:
#
#   scripts/**/*.sh
#   backend/scripts/**/*.sh
#   backend/docker-entrypoint.sh
#
# A finding is either fixed or excluded on its own line, with the reason in a
# comment directly above the directive:
#
#   # <why the flagged pattern is intended>
#   # shellcheck disable=SCxxxx
#
# Info and style findings (for example SC2016 on a deliberately single-quoted
# literal) are below the gate's severity and do not fail it.
#
# Usage:
#   scripts/check_shell.sh
#
# Needs the shellcheck binary on PATH (apt-get or brew install it). Runs
# from any directory; paths resolve from the repository root. Every file in
# the set sits under the root .shellcheckrc, so that is the config in effect.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if ! command -v shellcheck >/dev/null 2>&1; then
  echo "check_shell: shellcheck is not on PATH (apt-get install shellcheck, or brew install shellcheck)." >&2
  exit 2
fi

# One find over every start path, assigned through a command substitution:
# a missing start path makes find exit non-zero, which fails the script under
# `set -eo pipefail` instead of silently shrinking the checked set.
file_list="$(
  find scripts backend/scripts backend/docker-entrypoint.sh -type f -name '*.sh' \
    | LC_ALL=C sort
)"

files=()
while IFS= read -r file; do
  files+=("$file")
done <<<"$file_list"

shellcheck --version | sed -n 's/^version: /check_shell: shellcheck /p'
echo "check_shell: checking ${#files[@]} files at severity warning"

if ! shellcheck --severity=warning "${files[@]}"; then
  echo "check_shell: fix each finding above, or exclude it on its own line with a reason (see this script's header)." >&2
  exit 1
fi

echo "check_shell: no findings"
