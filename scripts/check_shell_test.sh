#!/usr/bin/env bash
# scripts/check_shell_test.sh
#
# Self-test for scripts/check_shell.sh. Builds a throwaway git repository in
# a temp dir, replays the file-set shapes the gate has to get right, and
# asserts both the exit code and the actionable part of the message.
#
# Runs in CI (Migration Lint, after shellcheck is on PATH) and locally:
#   bash scripts/check_shell_test.sh
#
# Needs git and shellcheck on PATH. It touches nothing outside its own
# mktemp -d.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNDER_TEST="$SCRIPT_DIR/check_shell.sh"

FIXTURE="$(mktemp -d)"
trap 'rm -rf "$FIXTURE"' EXIT

FAILURES=0
CASE=""

# Fixture git. The gate reads only the index, so staging a file is what puts
# it in the set; no case needs a commit. A global ignore file is neutralised
# so `add -A` stages every fixture file on any machine.
g() {
  git -C "$FIXTURE" -c core.excludesFile=/dev/null "$@" >/dev/null 2>&1
}

# write <path> <first line> [<body line>...]: one file, one line per argument.
write() {
  local path="$FIXTURE/$1"
  shift
  mkdir -p "$(dirname "$path")"
  printf '%s\n' "$@" >"$path"
}

# A body line that shellcheck reports at warning severity (SC2034).
UNUSED='unused_scratch_var=1'

# Assert the gate's exit code, and that its output contains every needle.
# Output is captured with stderr folded into stdout.
expect() {
  local want_code="$1"
  shift
  local out code needle
  out="$(bash "$FIXTURE/scripts/check_shell.sh" 2>&1)"
  code=$?
  if [ "$code" -ne "$want_code" ]; then
    fail "exit $code, wanted $want_code"
    return
  fi
  for needle in "$@"; do
    if ! printf '%s' "$out" | grep -qF -- "$needle"; then
      fail "output missing expected text: $needle"
      return
    fi
  done
  echo "ok   [$CASE]"
}

# fail <reason>: records a failed case and dumps the gate output from expect.
fail() {
  echo "FAIL [$CASE]: $1"
  printf '%s\n' "$out" | sed 's/^/    /'
  FAILURES=$((FAILURES + 1))
}

# Reset the fixture to an index holding the gate itself, a clean .sh
# in a directory the gate never names, a clean extensionless bash script, a
# non-shell script with a #! line, and a plain text file: 3 shell scripts.
reset_fixture() {
  CASE="$1"
  rm -rf "$FIXTURE"
  mkdir -p "$FIXTURE/scripts"
  cp "$UNDER_TEST" "$FIXTURE/scripts/check_shell.sh"
  : >"$FIXTURE/.shellcheckrc"
  write lib/clean.sh '#!/usr/bin/env bash' 'echo "clean"'
  write bin/tool '#!/usr/bin/env bash' 'echo "tool"'
  write bin/report.mjs '#!/usr/bin/env node' 'const unused = 1;'
  write README.md 'not a script'
  g init
  g add -A
}

reset_fixture "clean tree passes and counts the .sh and the shebang file"
expect 0 "checking 3 tracked files" "no findings"

reset_fixture "warning in a new .sh in an unlisted directory fails"
write brand/new/dir/scratch.sh '#!/usr/bin/env bash' "$UNUSED"
g add -A
expect 1 "checking 4 tracked files" "brand/new/dir/scratch.sh" "SC2034" "fix each finding above"

reset_fixture "warning in an extensionless #!/bin/sh script fails"
write bin/posix-tool '#!/bin/sh' "$UNUSED"
g add -A
expect 1 "bin/posix-tool" "SC2034"

reset_fixture "warning in an extensionless env -S bash script fails"
write bin/env-tool '#!/usr/bin/env -S bash -e' "$UNUSED"
g add -A
expect 1 "bin/env-tool" "SC2034"

reset_fixture "warning in an extensionless #!/bin/dash script fails"
write bin/dash-tool '#!/bin/dash' "$UNUSED"
g add -A
expect 1 "bin/dash-tool" "SC2034"

reset_fixture "CRLF extensionless bash script is checked"
printf '#!/bin/bash\r\necho "crlf"\r\n' >"$FIXTURE/bin/crlf-tool"
g add -A
expect 1 "bin/crlf-tool" "SC1017"

reset_fixture "path with spaces is checked"
write 'dir with space/a script.sh' '#!/usr/bin/env bash' "$UNUSED"
g add -A
expect 1 "dir with space/a script.sh" "SC2034"

reset_fixture "root-level script named like an option cannot weaken the run"
write '--severity=error' '#!/usr/bin/env bash' 'echo "named like an option"'
write lib/warn.sh '#!/usr/bin/env bash' "$UNUSED"
g add -A
expect 1 "lib/warn.sh" "SC2034"

reset_fixture "untracked script is not checked"
write scratch/untracked.sh '#!/usr/bin/env bash' "$UNUSED"
expect 0 "checking 3 tracked files" "no findings"

reset_fixture "non-shell and bare #! files are not checked"
write bin/tool.py '#!/usr/bin/env python3' 'unused = 1'
write bin/bare '#!' "$UNUSED"
g add -A
expect 0 "checking 3 tracked files" "no findings"

reset_fixture "zsh script is skipped with a notice naming it"
write bin/zsh-tool '#!/usr/bin/env zsh' 'arr=(a b)' 'echo "${(j:,:)arr}"'
g add -A
expect 0 "NOTICE: skipping zsh script" "bin/zsh-tool" "checking 3 tracked files"

reset_fixture "tracked symlink to a directory is skipped"
ln -s lib "$FIXTURE/lib-link"
g add -A
expect 0 "checking 3 tracked files" "no findings"

# Fail-closed cases: when the gate cannot see the set it is checking it must
# go RED, never quietly green.
reset_fixture "tracked script missing from the work tree fails closed"
rm "$FIXTURE/lib/clean.sh"
expect 1 "tracked files are missing from the work tree" "lib/clean.sh"

CASE="no git repository fails closed"
rm -rf "$FIXTURE/.git"
expect 1 "ERROR: git ls-files failed"

if [ "$FAILURES" -ne 0 ]; then
  echo ""
  echo "$FAILURES shellcheck gate self-test(s) failed."
  exit 1
fi

echo ""
echo "OK: shellcheck gate self-tests passed."
