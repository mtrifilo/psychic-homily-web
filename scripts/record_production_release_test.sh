#!/usr/bin/env bash
# scripts/record_production_release_test.sh
#
# Self-test for scripts/record_production_release.sh. Builds a throwaway
# repository with a bare "remote" in a temp dir (with main and production
# branches), puts a stub `gh` on PATH that logs its arguments and refuses a
# release whose tag is not on that remote, and asserts the exit code, what
# the script changed (tags on the remote) and the actionable part of each
# message.
#
# Runs in CI (Migration Lint) and locally:
#   bash scripts/record_production_release_test.sh
#
# It touches nothing outside its own mktemp -d and needs no network, no gh
# login and no configured git identity.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNDER_TEST="$SCRIPT_DIR/record_production_release.sh"

FIXTURE="$(mktemp -d)" || exit 1
[ -d "$FIXTURE" ] || exit 1
trap 'rm -rf "$FIXTURE"' EXIT

REMOTE_DIR="$FIXTURE/remote.git"
WORK="$FIXTURE/work"
STUB_BIN="$FIXTURE/bin"
GH_LOG="$FIXTURE/gh.log"
RECORD="$FIXTURE/record.md"
REPO="example/fixture"

# The annotated tag needs a committer identity; the fixture commits need an
# author. A CI runner has neither configured.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null

FAILURES=0
CASE=""

# Fixture git. Commit signing and a global core.hooksPath are neutralised so
# the self-test behaves the same on a CI runner and on a laptop that has either
# turned on globally.
g() {
  git -C "$WORK" -c commit.gpgsign=false -c core.hooksPath=/dev/null "$@" >/dev/null 2>&1
}

commit() {
  # commit <name>: one commit on the current branch
  printf '%s\n' "$1" >"$WORK/$1.txt"
  g add "$1.txt"
  g commit -m "$1"
}

set_production() {
  # set_production <sha>: what the fixture remote's production branch runs
  git -C "$REMOTE_DIR" update-ref refs/heads/production "$1"
}

# The stub gh: `auth status` succeeds; `api repos/...` answers the push
# permission (false when GH_STUB_NOPUSH is set); `release create` is logged
# as one line and succeeds only when its tag is already on the remote (as the
# real --verify-tag would) and GH_STUB_FAIL is unset.
mkdir -p "$STUB_BIN"
cat >"$STUB_BIN/gh" <<'STUB'
#!/usr/bin/env bash
case "${1:-} ${2:-}" in
  "auth status") exit 0 ;;
  "api repos/"*)
    if [ -n "${GH_STUB_NOPUSH:-}" ]; then echo false; else echo true; fi
    exit 0 ;;
  "release create")
    printf '%s\n' "$*" >>"$GH_LOG"
    if ! git -C "$REMOTE_DIR" rev-parse -q --verify "refs/tags/$3" >/dev/null 2>&1; then
      echo "stub gh: tag $3 is not on the remote (release create ran before the push)" >&2
      exit 3
    fi
    if [ -n "${GH_STUB_FAIL:-}" ]; then exit 1; fi
    exit 0 ;;
  *) echo "stub gh: unexpected call: $*" >&2; exit 9 ;;
esac
STUB
chmod +x "$STUB_BIN/gh"
export PATH="$STUB_BIN:$PATH"
export GH_LOG REMOTE_DIR

git init --bare -q "$REMOTE_DIR"
# A pre-receive hook that rejects every push while the flag file exists.
cat >"$REMOTE_DIR/hooks/pre-receive" <<'HOOK'
#!/usr/bin/env bash
if [ -e "$GIT_DIR/reject-pushes" ]; then echo "remote: pushes rejected by the test" >&2; exit 1; fi
exit 0
HOOK
chmod +x "$REMOTE_DIR/hooks/pre-receive"

git init -q -b main "$WORK"
commit a
commit b
commit c
g remote add origin "$REMOTE_DIR"
g push -q origin main
A="$(git -C "$WORK" rev-parse main~2)"
B="$(git -C "$WORK" rev-parse main~1)"
C="$(git -C "$WORK" rev-parse main)"
g checkout -q -b side "$A"
commit side
SIDE="$(git -C "$WORK" rev-parse side)"
g checkout -q main
commit d
g push -q origin main
D="$(git -C "$WORK" rev-parse main)"
set_production "$B"

printf '## Deploy record\n\n- range, migrations, verification\n' >"$RECORD"

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
  git -C "$REMOTE_DIR" rev-parse -q --verify "refs/tags/$1" >/dev/null 2>&1
}

expect_remote_tag() {
  remote_has_tag "$1" || fail "expected tag $1 on the remote"
}

expect_no_tag() {
  if remote_has_tag "$1" || git -C "$WORK" rev-parse -q --verify "refs/tags/$1" >/dev/null 2>&1; then
    fail "expected no tag $1 anywhere"
  fi
}

expect_refused_clean() {
  # expect_refused_clean <tag> <needle>: exit 1, the message, no gh call, no tag
  expect_status 1; expect_output "$2"; expect_gh_calls 0; expect_no_tag "$1"
}

BASE_OPTS="--github-repo $REPO"

# ---------------------------------------------- refusals before the baseline

CASE="--help exits 0 with the usage"
run --help
expect_status 0; expect_output "Usage:"; expect_gh_calls 0

CASE="malformed tag is refused"
run "v1" - "$A" "$RECORD" --baseline $BASE_OPTS
expect_refused_clean "v1" "prod-YYYY-MM-DD.N"

CASE="impossible calendar date is refused"
run "prod-2026-02-30.1" - "$A" "$RECORD" --baseline --pushed-at 2026-02-30T10:00:00Z $BASE_OPTS
expect_refused_clean "prod-2026-02-30.1" "not a calendar date"

CASE="a non-UTC --pushed-at is refused"
run "prod-2026-07-23.1" - "$A" "$RECORD" --baseline --pushed-at "2026-07-22 19:25:14 -0700" $BASE_OPTS
expect_refused_clean "prod-2026-07-23.1" "must be UTC"

CASE="tag date differing from the push date is refused"
run "prod-2026-07-22.1" - "$A" "$RECORD" --baseline --pushed-at 2026-07-23T02:25:14Z $BASE_OPTS
expect_refused_clean "prod-2026-07-22.1" "UTC date of --pushed-at"

CASE="record without the header line is refused"
printf 'notes\n' >"$FIXTURE/noheader.md"
run "prod-2026-07-23.1" - "$A" "$FIXTURE/noheader.md" --baseline --pushed-at 2026-07-23T02:25:14Z $BASE_OPTS
expect_refused_clean "prod-2026-07-23.1" "must start with the line"

CASE="record lines that look like credentials are refused, one by one"
while IFS= read -r leak; do
  printf '## Deploy record\n\n%s\n' "$leak" >"$FIXTURE/leak.md"
  run "prod-2026-07-23.1" - "$A" "$FIXTURE/leak.md" --baseline --pushed-at 2026-07-23T02:25:14Z $BASE_OPTS
  expect_status 1; expect_gh_calls 0; expect_no_tag "prod-2026-07-23.1"
  case "$OUTPUT" in *"looks like it carries a credential"*) ;; *) fail "expected the credential refusal for: $leak" ;; esac
done <<'LEAKS'
DATABASE_URL=postgresql://user:pw@host:5432/db
curl -H "Authorization: Bearer abcdef0123456789abcd"
SECRET_KEY = abc123def456
"password": "hunter2"
api_key: sk_live_abc123def456ghi7
RESEND_API_KEY: re_123456789abcdef
dsn https://abc123@o1.ingest.sentry.io/456
jwt eyJabc123def456ghi.eyJjkl789mno012pqr.stu345vwx
token ghp_abcdefghij0123456789klmn
LEAKS

CASE="ordinary record prose is not mistaken for a credential (dry run passes)"
cat >"$FIXTURE/prose.md" <<'PROSE'
## Deploy record

- token revoked after the merge; password login skipped (orchestrator holds no prod credentials)
- smoke: https://api.example.com/health 200; GET /verify?token=redacted returned 400
- Verified Bearer authentication still enforced; risk-assessment-page loads; ran disk-usage-check
PROSE
run "prod-2026-07-23.1" - "$A" "$FIXTURE/prose.md" --baseline --not-latest --dry-run --pushed-at 2026-07-23T02:25:14Z $BASE_OPTS
expect_status 0; expect_output "DRY RUN"; expect_gh_calls 0; expect_no_tag "prod-2026-07-23.1"

CASE="a remote that is not on github.com needs --github-repo"
run "prod-2026-07-23.1" - "$A" "$RECORD" --baseline --pushed-at 2026-07-23T02:25:14Z
expect_refused_clean "prod-2026-07-23.1" "pass --github-repo"

CASE="no push permission is refused before anything is tagged"
GH_STUB_NOPUSH=1 run "prod-2026-07-23.1" - "$A" "$RECORD" --baseline --pushed-at 2026-07-23T02:25:14Z $BASE_OPTS
expect_refused_clean "prod-2026-07-23.1" "no push permission"

CASE="first release without --baseline is refused (no prod-* tag anywhere)"
run "prod-2026-09-07.1" "$A" "$B" "$RECORD" --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "--baseline"

CASE="FROM_SHA - without --baseline is refused"
run "prod-2026-09-07.1" - "$B" "$RECORD" --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "only valid with --baseline"

CASE="--baseline with a real FROM_SHA is refused"
run "prod-2026-07-23.1" "$A" "$B" "$RECORD" --baseline --pushed-at 2026-07-23T02:25:14Z $BASE_OPTS
expect_refused_clean "prod-2026-07-23.1" "takes '-' as FROM_SHA"

CASE="baseline must be .1"
run "prod-2026-07-23.2" - "$A" "$RECORD" --baseline --not-latest --pushed-at 2026-07-23T02:25:14Z $BASE_OPTS
expect_refused_clean "prod-2026-07-23.2" "next tag for 2026-07-23 is prod-2026-07-23.1"

CASE="dry run creates nothing, prints the commands and the record"
run "prod-2026-07-23.1" - "$A" "$RECORD" --baseline --not-latest --dry-run --pushed-at 2026-07-23T02:25:14Z $BASE_OPTS
expect_status 0; expect_output "DRY RUN"; expect_output "git tag -a"; expect_output "gh release create"
expect_output "range, migrations, verification"
expect_gh_calls 0; expect_no_tag "prod-2026-07-23.1"

# ------------------------------------------------------------- baseline

CASE="baseline release: tag at TO, pushed, release with the record only"
run "prod-2026-07-23.1" - "$A" "$RECORD" --baseline --not-latest --pushed-at 2026-07-23T02:25:14Z $BASE_OPTS
expect_status 0; expect_output "Recorded prod-2026-07-23.1"
expect_remote_tag "prod-2026-07-23.1"; expect_gh_calls 1
expect_gh_args "release create prod-2026-07-23.1 -R $REPO --verify-tag --title prod-2026-07-23.1 --notes-file $RECORD --latest=false"
if grep -q -- "--generate-notes" "$GH_LOG"; then fail "baseline must not generate notes"; fi
TAGGED="$(git -C "$WORK" rev-parse "prod-2026-07-23.1^{commit}")"
[ "$TAGGED" = "$A" ] || fail "tag should point at TO ($A), points at $TAGGED"
TAG_DATE="$(TZ=UTC git -C "$WORK" for-each-ref --format='%(taggerdate:iso-strict-local)' refs/tags/prod-2026-07-23.1)"
case "$TAG_DATE" in 2026-07-23T02:25:14*) ;; *) fail "tag date should be the push time, got $TAG_DATE" ;; esac

# ------------------------------------------- refusals once a chain exists

CASE="existing local tag is refused"
run "prod-2026-07-23.1" "$A" "$B" "$RECORD" --pushed-at 2026-07-23T03:00:00Z $BASE_OPTS
expect_status 1; expect_output "already exists locally"; expect_gh_calls 0

CASE="tag that exists only on the remote is refused"
git -C "$REMOTE_DIR" tag prod-2026-08-01.1 "$B"
run "prod-2026-08-01.1" "$A" "$B" "$RECORD" --not-latest --pushed-at 2026-08-01T00:00:00Z $BASE_OPTS
expect_status 1; expect_output "already exists on origin"; expect_gh_calls 0
git -C "$REMOTE_DIR" tag -d prod-2026-08-01.1 >/dev/null

CASE="a tag dated before the newest tag is refused"
run "prod-2026-07-22.1" "$A" "$B" "$RECORD" --not-latest --pushed-at 2026-07-22T10:00:00Z $BASE_OPTS
expect_refused_clean "prod-2026-07-22.1" "dated before the newest tag"

CASE="a remote prod-* tag outside the naming scheme is refused"
git -C "$REMOTE_DIR" tag prod-hotfix "$B"
run "prod-2026-09-07.1" "$A" "$B" "$RECORD" --not-latest --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "outside the prod-YYYY-MM-DD.N scheme"
git -C "$REMOTE_DIR" tag -d prod-hotfix >/dev/null

CASE="--baseline once a prod-* tag exists is refused"
run "prod-2026-09-07.1" - "$B" "$RECORD" --baseline --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "already has prod-* tags"

CASE="TO not on the remote main is refused"
run "prod-2026-09-07.1" "$A" "$SIDE" "$RECORD" --not-latest --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "not on origin/main"

CASE="without --not-latest the remote must have a production branch"
git -C "$REMOTE_DIR" update-ref -d refs/heads/production
run "prod-2026-09-07.1" "$A" "$B" "$RECORD" --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "no production branch"

CASE="TO that production does not run is refused"
set_production "$C"
run "prod-2026-09-07.1" "$A" "$B" "$RECORD" --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "not what production runs"

CASE="a SHA that already has a release is refused"
set_production "$A"
run "prod-2026-09-07.1" "$B" "$A" "$RECORD" --not-latest --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "already records TO_SHA"

CASE="FROM not an ancestor of TO is refused (a rollback)"
git -C "$REMOTE_DIR" tag prod-2026-09-01.1 "$C"
set_production "$B"
run "prod-2026-09-07.1" "$C" "$B" "$RECORD" --not-latest --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "not an ancestor"
git -C "$REMOTE_DIR" tag -d prod-2026-09-01.1 >/dev/null

CASE="FROM without a remote tag is refused with the newest tag named"
set_production "$C"
run "prod-2026-09-07.1" "$B" "$C" "$RECORD" --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "newest tag: prod-2026-07-23.1"

CASE="a local-only tag at FROM does not count"
g tag prod-2026-09-01.1 "$B"
run "prod-2026-09-07.1" "$B" "$C" "$RECORD" --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.1" "no prod-* tag on origin points at FROM_SHA"
g tag -d prod-2026-09-01.1

# -------------------------------------------------------- normal release

CASE="normal release: notes start at the FROM tag, marked latest, repo targeted"
set_production "$B"
run "prod-2026-09-07.1" "$A" "$B" "$RECORD" --pushed-at 2026-09-07T22:55:37Z $BASE_OPTS
expect_status 0; expect_remote_tag "prod-2026-09-07.1"; expect_gh_calls 1
expect_gh_args "release create prod-2026-09-07.1 -R $REPO --verify-tag --title prod-2026-09-07.1 --notes-file $RECORD --generate-notes --notes-start-tag prod-2026-07-23.1 --latest"
if grep -q -- "--latest=false" "$GH_LOG"; then fail "a current release must be marked latest"; fi
TAGGED="$(git -C "$WORK" rev-parse "prod-2026-09-07.1^{commit}")"
[ "$TAGGED" = "$B" ] || fail "tag should point at TO ($B), points at $TAGGED"

CASE="same-day sequence must be the next number"
set_production "$C"
run "prod-2026-09-07.3" "$B" "$C" "$RECORD" --pushed-at 2026-09-07T23:00:00Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.3" "next tag for 2026-09-07 is prod-2026-09-07.2"

CASE="two remote prod-* tags at FROM is refused with the delete hint"
git -C "$REMOTE_DIR" tag prod-2026-09-07.2 "$B"
run "prod-2026-09-07.3" "$B" "$C" "$RECORD" --pushed-at 2026-09-07T23:00:00Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.3" "delete the stray one"
git -C "$REMOTE_DIR" tag -d prod-2026-09-07.2 >/dev/null

CASE="FROM that is not the newest tag is refused unless backfilling"
run "prod-2026-09-07.2" "$A" "$C" "$RECORD" --pushed-at 2026-09-07T23:00:00Z $BASE_OPTS
expect_refused_clean "prod-2026-09-07.2" "is not the newest"

CASE="a rejected push exits 2 with the tag local only"
touch "$REMOTE_DIR/reject-pushes"
run "prod-2026-09-07.2" "$B" "$C" "$RECORD" --pushed-at 2026-09-07T23:00:00Z $BASE_OPTS
expect_status 2; expect_output "exists locally only"; expect_gh_calls 0
remote_has_tag "prod-2026-09-07.2" && fail "the rejected tag must not be on the remote"
rm -f "$REMOTE_DIR/reject-pushes"
g tag -d prod-2026-09-07.2

CASE="gh failure after the push exits 2, names the pushed tag and prints the finish command"
GH_STUB_FAIL=1 run "prod-2026-09-07.2" "$B" "$C" "$RECORD" --pushed-at 2026-09-07T23:00:00Z $BASE_OPTS
expect_status 2; expect_output "tag is already on origin"; expect_remote_tag "prod-2026-09-07.2"
expect_output "finish:  gh release create prod-2026-09-07.2"
expect_output "--notes-start-tag prod-2026-09-07.1"

CASE="backfill: --not-latest skips the production-head and newest-tag checks"
git -C "$REMOTE_DIR" tag -d prod-2026-09-07.2 >/dev/null
g tag -d prod-2026-09-07.2
set_production "$D"
run "prod-2026-09-07.2" "$B" "$C" "$RECORD" --not-latest --pushed-at 2026-09-07T23:00:00Z $BASE_OPTS
expect_status 0; expect_remote_tag "prod-2026-09-07.2"; expect_gh_calls 1
expect_gh_args "--notes-start-tag prod-2026-09-07.1 --latest=false"

CASE="chain continues at the newest tag onto the production head"
run "prod-2026-09-08.1" "$C" "$D" "$RECORD" --pushed-at 2026-09-08T01:00:00Z $BASE_OPTS
expect_status 0; expect_remote_tag "prod-2026-09-08.1"; expect_gh_calls 1
expect_gh_args "--notes-start-tag prod-2026-09-07.2 --latest"

# ---------------------------------------------------------------- result

if [ "$FAILURES" -gt 0 ]; then
  echo "record_production_release_test: $FAILURES failure(s)"
  exit 1
fi
echo "record_production_release_test: all cases passed"
