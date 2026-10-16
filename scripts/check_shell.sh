#!/usr/bin/env bash
# scripts/check_shell.sh
#
# CI gate: shellcheck at warning severity over every tracked shell script.
# A tracked file is a shell script when its name ends in .sh, or when its
# first line is a #! for sh, bash or dash, directly or through env. A script
# added anywhere in the tree is therefore checked without editing this file,
# and an untracked one is not checked until it is added.
#
# A zsh script (by its #! line) is skipped with a notice naming it: shellcheck
# does not support zsh, and forcing --shell=bash reports zsh syntax such as
# ${(j:,:)arr} as an error.
#
# A tracked path missing from the work tree fails the gate, so a deleted or
# moved script cannot shrink the checked set without anyone noticing.
#
# A finding is either fixed or excluded on its own line, with the reason in a
# comment directly above the directive:
#
#   # <why the flagged pattern is intended>
#   # shellcheck disable=SCxxxx
#
# Usage:
#   scripts/check_shell.sh
#
# Needs git and the shellcheck binary on PATH (apt-get or brew install it).
# Every tracked file sits under the root .shellcheckrc, so that is the config
# in effect. scripts/check_shell_test.sh is the self-test.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if ! command -v shellcheck >/dev/null 2>&1; then
  echo "ERROR: shellcheck is not on PATH (apt-get install shellcheck, or brew install shellcheck)." >&2
  exit 1
fi

# The listing goes through a file so a git failure is seen here, and so
# NUL-separated paths survive (a variable cannot hold NUL).
listing="$(mktemp)"
trap 'rm -f "$listing"' EXIT
if ! git ls-files -z >"$listing"; then
  echo "ERROR: git ls-files failed in $REPO_ROOT; the checked set comes from the git index." >&2
  exit 1
fi

files=()
missing=()
while IFS= read -r -d '' path; do
  # A tracked symlink's target is listed on its own when it is tracked, and a
  # directory here is a gitlink to another repository.
  if [ -L "$path" ] || [ -d "$path" ]; then
    continue
  fi
  if [ ! -f "$path" ]; then
    missing+=("$path")
    continue
  fi

  # The interpreter is the first word of the #! line that is not env, an env
  # option or an env assignment: `#!/usr/bin/env -S bash -e` gives bash. The
  # read is capped so a binary file with no newline is not read whole, and \r
  # splits like a blank so a CRLF line still names its interpreter.
  first_line=""
  IFS= read -r -n 256 first_line <"$path" || true
  interpreter=""
  case "$first_line" in
    '#!'*)
      shebang_words=()
      IFS=$' \t\r' read -r -a shebang_words <<<"${first_line#\#!}" || true
      # The +-guard keeps a bare `#!` (no words) from tripping `set -u` on
      # bash 3.2, which treats an empty array expansion as unbound.
      for word in ${shebang_words[@]+"${shebang_words[@]}"}; do
        case "$word" in
          env | */env | -* | *=*) continue ;;
        esac
        interpreter="${word##*/}"
        break
      done
      ;;
  esac

  case "$interpreter" in
    zsh) echo "check_shell: NOTICE: skipping zsh script (shellcheck does not support zsh): $path" ;;
    sh | bash | dash) files+=("$path") ;;
    *)
      case "$path" in
        *.sh) files+=("$path") ;;
      esac
      ;;
  esac
done <"$listing"

if [ "${#missing[@]}" -gt 0 ]; then
  echo "ERROR: tracked files are missing from the work tree; restore them, or git rm them:" >&2
  printf '  %s\n' "${missing[@]}" >&2
  exit 1
fi

# This script is tracked, so an empty set means the listing did not come from
# this repository's index.
if [ "${#files[@]}" -eq 0 ]; then
  echo "ERROR: git ls-files listed no shell scripts, not even this one." >&2
  exit 1
fi

shellcheck --version | sed -n 's/^version: /check_shell: shellcheck /p'
echo "check_shell: checking ${#files[@]} tracked files at severity warning"

# `--` keeps a tracked root-level file named like an option (for example
# --severity=error) from being parsed as one and weakening the whole run.
if ! shellcheck --severity=warning -- "${files[@]}"; then
  echo "ERROR: fix each finding above, or exclude it on its own line with a reason (see this script's header)." >&2
  exit 1
fi

echo "check_shell: no findings"
