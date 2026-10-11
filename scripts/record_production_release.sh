#!/usr/bin/env bash
# scripts/record_production_release.sh
#
# Records one production promote as a GitHub Release: an annotated tag at the
# promoted SHA, pushed to the remote, then a release whose body is the deploy
# record followed by GitHub's generated notes (every PR merged between the
# previous production tag and this one, grouped by .github/release.yml).
#
# A production deploy is one fast-forward push of a CI-verified SHA to the
# `production` branch. The push leaves no record on GitHub of which PRs
# shipped or what was verified afterwards; the release created here is that
# record, and the chain of prod-* tags is the promote history.
#
# Usage:
#   scripts/record_production_release.sh TAG FROM_SHA TO_SHA RECORD_FILE [options]
#
#   TAG          prod-YYYY-MM-DD.N: the UTC date of the push and a same-day
#                sequence that starts at 1 every day.
#   FROM_SHA     the SHA production ran before the push. Exactly one prod-*
#                tag must point at it; the generated notes start there.
#                Under --baseline it is `-`: the release has no predecessor.
#   TO_SHA       the SHA pushed to production. Must be on the remote's main.
#   RECORD_FILE  markdown deploy record (SHA range, push time, migrations,
#                verification). It becomes the top of the release body.
#
# Options:
#   --pushed-at TIME      the push time as UTC, 2026-10-11T21:35:04Z; it
#                         becomes the tag's date (default: now).
#   --baseline            the first release ever recorded: FROM_SHA is `-`,
#                         and the body is the record alone with no generated
#                         notes.
#   --not-latest          do not mark the release Latest (backfilling history).
#   --remote NAME         git remote (default origin).
#   --dry-run             run every check, print the mutating commands, change
#                         nothing.
#
# Exit codes:
#   0  recorded (or every check passed under --dry-run)
#   1  refused; nothing was changed
#   2  a mutating step failed part-way; the message says what already exists
#
# Refused before anything is touched: a malformed TAG; a TAG that exists
# locally or on the remote; a TO_SHA that is not on the remote's main; a
# FROM_SHA that is not an ancestor of TO_SHA; a FROM_SHA without exactly one
# prod-* tag; --baseline with any FROM_SHA but `-`, or `-` without --baseline;
# a missing or empty RECORD_FILE; no gh login.

set -euo pipefail

usage() {
  # The header comment above, up to the first non-comment line.
  awk 'NR > 1 && !/^#/ { exit } NR > 1 { sub(/^# ?/, ""); print }' "${BASH_SOURCE[0]}"
}

refuse() {
  echo "REFUSED: $*" >&2
  exit 1
}

case "${1:-}" in -h|--help) usage; exit 0 ;; esac

if [ "$#" -lt 4 ]; then
  usage >&2
  exit 1
fi

TAG="$1"
FROM_SHA="$2"
TO_SHA="$3"
RECORD_FILE="$4"
shift 4

PUSHED_AT=""
BASELINE=0
MARK_LATEST=1
REMOTE="origin"
DRY_RUN=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --pushed-at)
      [ "$#" -ge 2 ] || refuse "--pushed-at needs a value"
      PUSHED_AT="$2"
      shift 2
      ;;
    --baseline) BASELINE=1; shift ;;
    --not-latest) MARK_LATEST=0; shift ;;
    --remote)
      [ "$#" -ge 2 ] || refuse "--remote needs a value"
      REMOTE="$2"
      shift 2
      ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) refuse "unknown option: $1" ;;
  esac
done

# ---------------------------------------------------------------- checks

case "$TAG" in
  prod-[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9].[1-9]|prod-[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9].[1-9][0-9]) ;;
  *) refuse "TAG must look like prod-YYYY-MM-DD.N (got '$TAG')" ;;
esac

[ -s "$RECORD_FILE" ] || refuse "RECORD_FILE '$RECORD_FILE' is missing or empty"

command -v gh >/dev/null 2>&1 || refuse "gh is not installed"
gh auth status >/dev/null 2>&1 || refuse "gh is not logged in (gh auth status)"

git fetch --quiet "$REMOTE" "+refs/heads/main:refs/remotes/$REMOTE/main" "+refs/tags/prod-*:refs/tags/prod-*" \
  || refuse "could not fetch main and prod-* tags from $REMOTE"

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
  refuse "tag $TAG already exists locally"
fi
if [ -n "$(git ls-remote --tags "$REMOTE" "refs/tags/$TAG")" ]; then
  refuse "tag $TAG already exists on $REMOTE"
fi

TO_FULL="$(git rev-parse -q --verify "${TO_SHA}^{commit}" 2>/dev/null)" \
  || refuse "TO_SHA '$TO_SHA' is not a commit in this repository"
git merge-base --is-ancestor "$TO_FULL" "refs/remotes/$REMOTE/main" \
  || refuse "TO_SHA $TO_SHA is not on $REMOTE/main"

if [ "$BASELINE" -eq 1 ]; then
  [ "$FROM_SHA" = "-" ] \
    || refuse "--baseline takes '-' as FROM_SHA (a baseline has no predecessor); got '$FROM_SHA'"
  [ -z "$(git tag -l 'prod-*')" ] \
    || refuse "--baseline is for the first release ever, but prod-* tags already exist ($(git tag -l 'prod-*' | tr '\n' ' '))"
  FROM_TAG=""
  FROM_SHORT="baseline"
else
  [ "$FROM_SHA" != "-" ] \
    || refuse "FROM_SHA '-' is only valid with --baseline"
  FROM_FULL="$(git rev-parse -q --verify "${FROM_SHA}^{commit}" 2>/dev/null)" \
    || refuse "FROM_SHA '$FROM_SHA' is not a commit in this repository"
  [ "$FROM_FULL" != "$TO_FULL" ] || refuse "FROM_SHA and TO_SHA are the same commit"
  git merge-base --is-ancestor "$FROM_FULL" "$TO_FULL" \
    || refuse "FROM_SHA $FROM_SHA is not an ancestor of TO_SHA $TO_SHA"
  FROM_TAGS="$(git tag --points-at "$FROM_FULL" 'prod-*')"
  FROM_TAG_COUNT="$(printf '%s' "$FROM_TAGS" | grep -c . || true)"
  [ "$FROM_TAG_COUNT" -eq 1 ] \
    || refuse "expected exactly one prod-* tag at FROM_SHA $FROM_SHA, found $FROM_TAG_COUNT ($(printf '%s' "$FROM_TAGS" | tr '\n' ' ')); record the earlier release first, or pass --baseline for the first release ever"
  FROM_TAG="$FROM_TAGS"
  FROM_SHORT="$(git rev-parse --short=9 "$FROM_FULL")"
fi

if [ -z "$PUSHED_AT" ]; then
  PUSHED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
fi
case "$PUSHED_AT" in
  [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z) ;;
  *) refuse "--pushed-at must be UTC like 2026-10-11T21:35:04Z (got '$PUSHED_AT'); git would silently date the tag now" ;;
esac

TO_SHORT="$(git rev-parse --short=9 "$TO_FULL")"
TAG_MESSAGE="Production release $TAG: $FROM_SHORT -> $TO_SHORT"

# ------------------------------------------------------------- commands

set -- gh release create "$TAG" --verify-tag --title "$TAG" --notes-file "$RECORD_FILE"
if [ "$BASELINE" -eq 0 ]; then
  set -- "$@" --generate-notes --notes-start-tag "$FROM_TAG"
fi
if [ "$MARK_LATEST" -eq 1 ]; then
  set -- "$@" --latest
else
  set -- "$@" --latest=false
fi

if [ "$DRY_RUN" -eq 1 ]; then
  echo "DRY RUN: every check passed; these commands would run:"
  printf '  GIT_COMMITTER_DATE=%q git tag -a %q %q -m %q\n' "$PUSHED_AT" "$TAG" "$TO_FULL" "$TAG_MESSAGE"
  printf '  git push %q %q\n' "$REMOTE" "refs/tags/$TAG"
  printf '  '
  printf '%q ' "$@"
  printf '\n'
  exit 0
fi

GIT_COMMITTER_DATE="$PUSHED_AT" git tag -a "$TAG" "$TO_FULL" -m "$TAG_MESSAGE" \
  || { echo "FAILED creating tag $TAG locally; nothing was pushed" >&2; exit 2; }

if ! git push "$REMOTE" "refs/tags/$TAG"; then
  echo "FAILED pushing tag $TAG; it exists locally only (git tag -d $TAG to undo)" >&2
  exit 2
fi

if ! "$@"; then
  echo "FAILED creating the release for $TAG; the tag is already on $REMOTE (git push $REMOTE :refs/tags/$TAG and git tag -d $TAG to undo, or rerun only the gh release create command above)" >&2
  exit 2
fi

echo "Recorded $TAG at $TO_SHORT (from $FROM_SHORT) pushed at $PUSHED_AT"
