#!/usr/bin/env bash
# scripts/record_production_release_test.sh
#
# Self-test for scripts/record_production_release.sh. Builds a throwaway
# repository with a bare "remote" in a temp dir, puts a stub `gh` on PATH that
# only logs its arguments, and asserts the exit code, what the script changed
# (tags locally and on the remote) and the actionable part of each message.
#
# Runs in CI (Migration Lint) and locally:
#   bash scripts/record_production_release_test.sh
#
# It touches nothing outside its own mktemp -d.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNDER_TEST="$SCRIPT_DIR/record_production_release.sh"

FIXTURE="$(mktemp -d)"
trap 'rm -rf "$FIXTURE"' EXIT

REMOTE="$FIXTURE/remote.git"
WORK="$FIXTURE/work"
STUB_BIN="$FIXTURE/bin"
GH_LOG="$FIXTURE/gh.log"
RECORD="$FIXTURE/record.md"
EMPTY_RECORD="$FIXTURE/empty.md"

FAILURES=0
CASE=""

# Fixture git. Commit signing and a global core.hooksPath are neutralised so
# the self-test behaves the same on a CI runner and on a laptop that has either
# turned on globally.
g() {
  git -C "$WORK" -c commit.gpgsign=false -c core.hooksPath=/dev/null "$@" >/dev/null 2>&1
}

commit() {
  # commit <name>: one commit on the current branch, returns nothing
  printf '%s\n' "$1" >"$WORK/$1.txt"
  g add "$1.txt"
  g -c user.name=t -c user.email=t@t commit -m "$1"
}

# The stub gh: `gh auth status` succeeds, every other call is appended to the
# log as one line and succeeds unless GH_STUB_FAIL is set.
mkdir -p "$STUB_BIN"
cat >"$STUB_BIN/gh" <<'STUB'
#!/usr/bin/env bash
if [ "${1:-}" = "auth" ]; then exit 0; fi
printf '%s\n' "$*" >>"$GH_LOG"
if [ -n "${GH_STUB_FAIL:-}" ]; then exit 1; fi
exit 0
STUB
chmod +x "$STUB_BIN/gh"
export PATH="$STUB_BIN:$PATH"
export GH_LOG

git init --bare -q "$REMOTE"
git init -q -b main "$WORK"
commit a
commit b
commit c
g remote add origin "$REMOTE"
g push -q origin main
A="$(git -C "$WORK" rev-parse main~2)"
B="$(git -C "$WORK" rev-parse main~1)"
C="$(git -C "$WORK" rev-parse main)"
g checkout -q -b side "$A"
commit side
SIDE="$(git -C "$WORK" rev-parse side)"
g checkout -q main

printf '# Deploy record\n\nrange, migrations, verification\n' >"$RECORD"
: >"$EMPTY_RECORD"

run() {
  # run <args...>: runs the script in the fixture, captures output + exit code
  : >"$GH_LOG"
  OUTPUT="$(cd "$WORK" && bash "$UNDER_TEST" "$@" 2>&1)"
  STATUS=$?
}

fail() {
  FAILURES=$((FAILURES + 1))
  echo "FAIL [$CASE]: $*"
  echo "  output: $OUTPUT"
}

expect_status() {
  [ "$STATUS" -eq "$1" ] || fail "expected exit $1, got $STATUS"
}

expect_output() {
  case "$OUTPUT" in *"$1"*) ;; *) fail "expected output to mention '$1'" ;; esac
}

expect_gh_calls() {
  local n
  n="$(grep -c . "$GH_LOG" 2>/dev/null || true)"
  [ "${n:-0}" -eq "$1" ] || fail "expected $1 gh call(s), got ${n:-0}: $(cat "$GH_LOG" 2>/dev/null)"
}

expect_gh_args() {
  grep -q -- "$1" "$GH_LOG" || fail "expected the gh call to include '$1', got: $(cat "$GH_LOG")"
}

remote_has_tag() {
  git -C "$REMOTE" rev-parse -q --verify "refs/tags/$1" >/dev/null 2>&1
}

expect_remote_tag() {
  remote_has_tag "$1" || fail "expected tag $1 on the remote"
}

expect_no_tag() {
  if remote_has_tag "$1" || git -C "$WORK" rev-parse -q --verify "refs/tags/$1" >/dev/null 2>&1; then
    fail "expected no tag $1 anywhere"
  fi
}

# ------------------------------------------------------------------ cases

CASE="malformed tag is refused"
run "v1" "$A" "$B" "$RECORD"
expect_status 1; expect_output "prod-YYYY-MM-DD.N"; expect_gh_calls 0; expect_no_tag "v1"

CASE="--help exits 0 with the usage"
run --help
expect_status 0; expect_output "Usage:"; expect_gh_calls 0

CASE="a non-UTC --pushed-at is refused"
run "prod-2026-07-23.1" - "$A" "$RECORD" --baseline --pushed-at "2026-07-22 19:25:14 -0700"
expect_status 1; expect_output "must be UTC"; expect_gh_calls 0; expect_no_tag "prod-2026-07-23.1"

CASE="empty record is refused"
run "prod-2026-07-23.1" - "$A" "$EMPTY_RECORD" --baseline
expect_status 1; expect_output "RECORD_FILE"; expect_gh_calls 0; expect_no_tag "prod-2026-07-23.1"

CASE="first release without --baseline is refused (no prod-* tag at FROM)"
run "prod-2026-09-07.1" "$A" "$B" "$RECORD"
expect_status 1; expect_output "--baseline"; expect_gh_calls 0; expect_no_tag "prod-2026-09-07.1"

CASE="FROM_SHA - without --baseline is refused"
run "prod-2026-09-07.1" - "$B" "$RECORD"
expect_status 1; expect_output "only valid with --baseline"; expect_gh_calls 0; expect_no_tag "prod-2026-09-07.1"

CASE="--baseline with a real FROM_SHA is refused"
run "prod-2026-07-23.1" "$A" "$B" "$RECORD" --baseline
expect_status 1; expect_output "takes '-' as FROM_SHA"; expect_gh_calls 0; expect_no_tag "prod-2026-07-23.1"

CASE="dry run changes nothing and prints the commands"
run "prod-2026-07-23.1" - "$A" "$RECORD" --baseline --dry-run --pushed-at 2026-07-23T02:25:14Z
expect_status 0; expect_output "DRY RUN"; expect_output "git tag -a"; expect_output "gh release create"
expect_gh_calls 0; expect_no_tag "prod-2026-07-23.1"

CASE="baseline release: tag at TO, pushed, release with the record only"
run "prod-2026-07-23.1" - "$A" "$RECORD" --baseline --not-latest --pushed-at 2026-07-23T02:25:14Z
expect_status 0; expect_output "Recorded prod-2026-07-23.1"
expect_remote_tag "prod-2026-07-23.1"; expect_gh_calls 1
expect_gh_args "release create prod-2026-07-23.1 --verify-tag --title prod-2026-07-23.1 --notes-file $RECORD --latest=false"
if grep -q -- "--generate-notes" "$GH_LOG"; then fail "baseline must not generate notes"; fi
TAGGED="$(git -C "$WORK" rev-parse "prod-2026-07-23.1^{commit}")"
[ "$TAGGED" = "$A" ] || fail "tag should point at TO ($A), points at $TAGGED"
TAG_DATE="$(TZ=UTC git -C "$WORK" for-each-ref --format='%(taggerdate:iso-strict-local)' refs/tags/prod-2026-07-23.1)"
case "$TAG_DATE" in 2026-07-23T02:25:14*) ;; *) fail "tag date should be the push time, got $TAG_DATE" ;; esac

CASE="existing tag is refused"
run "prod-2026-07-23.1" "$A" "$B" "$RECORD"
expect_status 1; expect_output "already exists"; expect_gh_calls 0

CASE="--baseline once a prod-* tag exists is refused"
run "prod-2026-09-07.1" - "$B" "$RECORD" --baseline
expect_status 1; expect_output "prod-* tags already exist"; expect_gh_calls 0; expect_no_tag "prod-2026-09-07.1"

CASE="TO not on the remote main is refused"
run "prod-2026-09-07.1" "$A" "$SIDE" "$RECORD"
expect_status 1; expect_output "not on origin/main"; expect_gh_calls 0; expect_no_tag "prod-2026-09-07.1"

CASE="FROM not an ancestor of TO is refused"
g tag prod-2026-09-01.1 "$C"
run "prod-2026-09-07.1" "$C" "$B" "$RECORD"
expect_status 1; expect_output "not an ancestor"; expect_gh_calls 0; expect_no_tag "prod-2026-09-07.1"
g tag -d prod-2026-09-01.1

CASE="normal release: notes start at the FROM tag, marked latest"
run "prod-2026-09-07.1" "$A" "$B" "$RECORD" --pushed-at 2026-09-07T22:55:37Z
expect_status 0; expect_remote_tag "prod-2026-09-07.1"; expect_gh_calls 1
expect_gh_args "--generate-notes --notes-start-tag prod-2026-07-23.1 --latest"
if grep -q -- "--latest=false" "$GH_LOG"; then fail "a current release must be marked latest"; fi
TAGGED="$(git -C "$WORK" rev-parse "prod-2026-09-07.1^{commit}")"
[ "$TAGGED" = "$B" ] || fail "tag should point at TO ($B), points at $TAGGED"

commit d
g push -q origin main
D="$(git -C "$WORK" rev-parse main)"

CASE="two prod-* tags at FROM is refused"
g tag prod-2026-09-07.2 "$B"
run "prod-2026-09-08.1" "$B" "$D" "$RECORD"
expect_status 1; expect_output "expected exactly one prod-* tag"; expect_gh_calls 0; expect_no_tag "prod-2026-09-08.1"
g tag -d prod-2026-09-07.2

CASE="gh failure after the push exits 2 and names the pushed tag"
GH_STUB_FAIL=1 run "prod-2026-09-08.1" "$B" "$D" "$RECORD"
expect_status 2; expect_output "tag is already on origin"; expect_remote_tag "prod-2026-09-08.1"

# ---------------------------------------------------------------- result

if [ "$FAILURES" -gt 0 ]; then
  echo "record_production_release_test: $FAILURES failure(s)"
  exit 1
fi
echo "record_production_release_test: all cases passed"
