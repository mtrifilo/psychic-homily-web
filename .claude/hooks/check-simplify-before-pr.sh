#!/usr/bin/env bash
# PreToolUse hook on Bash — blocks `gh pr create` if the PR body lacks a
# checked test-plan item for /simplify.
#
# Encodes the psy-solo phase 5 + psy-dispatch ironclad rule 3 convention:
# /simplify must run before push. Belt-and-suspenders on top of
# /psy-self-review's body-claim audit — catches the case where the agent
# skips /simplify AND doesn't claim it (which /psy-self-review can't detect
# because there's no [x] claim to audit).
#
# Bypass for explicit one-offs: set CLAUDE_SKIP_SIMPLIFY_CHECK=1 in the call.

set -euo pipefail

# Fail-open if jq is missing — better to skip the check than to block ALL
# Bash tool calls because a dependency is unavailable on this machine.
command -v jq >/dev/null || exit 0

# Read tool input JSON from stdin
input="$(cat)"
command=$(echo "$input" | jq -r '.tool_input.command // empty')

# Only fire for `gh pr create` (skip `gh pr edit`, `gh pr view`, etc.)
if ! [[ "$command" =~ ^[[:space:]]*gh[[:space:]]+pr[[:space:]]+create ]]; then
    exit 0
fi

# Explicit bypass
if [[ "${CLAUDE_SKIP_SIMPLIFY_CHECK:-}" == "1" ]]; then
    echo "[check-simplify-before-pr] CLAUDE_SKIP_SIMPLIFY_CHECK=1 — bypassing" >&2
    exit 0
fi

# Locate the PR body. Handles both `--body-file <path>` and `--body-file=<path>`.
# Paths with spaces / shell quoting aren't supported here; if the regex misses,
# the empty-body fallback at the next block falls back to permissive (rely on
# /psy-self-review's Agent 1 audit instead of risking a false-positive block).
body=""
if [[ "$command" =~ --body-file[=[:space:]]+([^[:space:]]+) ]]; then
    body_file="${BASH_REMATCH[1]}"
    if [[ -r "$body_file" ]]; then
        body="$(cat "$body_file")"
    fi
elif [[ "$command" =~ --body[[:space:]]+\"(.*)\" ]]; then
    body="${BASH_REMATCH[1]}"
fi

# If we can't read the body, fall back to permissive — avoid false-positive
# blocks on edge syntax (heredoc inside subshell, --body-file with shell
# expansion, etc.). /psy-self-review's Agent 1 audit is the second line.
if [[ -z "$body" ]]; then
    exit 0
fi

# Accept any of:
#   - [x] /simplify
#   - [x] `/simplify`
#   - [x] /simplify — <outcome>
#   - [x] `/simplify` — <outcome>
# Lowercase `[x]` only — `[X]` is a typo signal worth catching as a miss,
# not silently accepting. Indented bullet allowed.
if echo "$body" | grep -qE '^[[:space:]]*-[[:space:]]*\[x\][[:space:]]*`?/simplify`?'; then
    exit 0
fi

# Missing — block the push
cat >&2 <<'EOF'

ERROR — pre-push convention violation

PR body must contain a checked test-plan item for /simplify:
    - [x] /simplify — <outcome>
or
    - [x] `/simplify` — <outcome>

The /psy-solo phase 5 + /psy-dispatch ironclad rule 3 both require /simplify
to run before push. If you ran /simplify, add the [x] claim to your PR body
and re-run gh pr create.

If you genuinely skipped /simplify (tiny typo fix, docs-only, etc.), explicitly
downgrade to [ ] with a "skipped — <reason>" note. Reviewers need the signal.

To bypass this hook for a specific call:
    CLAUDE_SKIP_SIMPLIFY_CHECK=1 gh pr create ...

EOF
exit 1
