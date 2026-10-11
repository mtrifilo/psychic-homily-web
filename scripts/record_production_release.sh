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
# record, and the chain of prod-* tags is the promote history. The chain is
# linear and dated: every release's FROM is the newest tag and every new tag
# is dated on or after it. A rollback (production moved backwards) and a
# restore-only promote (same SHA re-deployed) are not recorded; the promote
# after a rollback uses the newest tag's SHA as FROM, so its notes list only
# what is new since the last release, and its record says which older SHA
# production ran in between.
#
# Usage:
#   scripts/record_production_release.sh TAG FROM_SHA TO_SHA RECORD_FILE [options]
#
#   TAG          prod-YYYY-MM-DD.N: the UTC date of the push and a same-day
#                sequence. N is one more than the highest N already recorded
#                for that date (1 for the first), so the chain has no gaps.
#   FROM_SHA     the SHA production ran before the push. Exactly one prod-*
#                tag on the remote must point at it, the newest one; the
#                generated notes start there. Under --baseline it is `-`.
#   TO_SHA       the SHA pushed to production: on the remote's main and, unless
#                --not-latest, the remote's production head right now.
#   RECORD_FILE  markdown deploy record: first line `## Deploy record`, then
#                the SHA range, push time, migrations and verification. It is
#                published verbatim as the top of a PUBLIC release body, so it
#                is refused when it looks like it carries a credential.
#
# Options:
#   --pushed-at TIME      the push time as UTC, 2026-10-11T21:35:04Z; it
#                         becomes the tag's date and its date must be TAG's
#                         date (default: now).
#   --baseline            the first release ever recorded: FROM_SHA is `-`,
#                         no prod-* tag may exist, and the body is the record
#                         alone with no generated notes.
#   --not-latest          backfilling history: the release is not marked
#                         Latest, TO_SHA need not be the production head and
#                         FROM need not be the newest tag.
#   --remote NAME         git remote (default origin).
#   --github-repo O/R     the GitHub repository (default: parsed from the
#                         remote's URL, which must be on github.com).
#   --dry-run             run every check, print the record and the mutating
#                         commands, create nothing.
#
# Exit codes:
#   0  recorded (or every check passed under --dry-run)
#   1  refused; no tag or release was created (the fetch of main still ran)
#   2  a mutating step failed part-way; the message says what already exists
#      and prints the exact command to finish or undo
#
# Needs: git, gh logged in with push permission on the repository, and a git
# committer identity for the annotated tag.

set -euo pipefail

usage() {
  # The header comment above, up to the first non-comment line.
  awk 'NR > 1 && !/^#/ { exit } NR > 1 { sub(/^# ?/, ""); print }' "${BASH_SOURCE[0]}"
}

refuse() {
  echo "REFUSED: $*" >&2
  exit 1
}

# valid_date YYYY MM DD: a real calendar date.
valid_date() {
  local y=$((10#$1)) m=$((10#$2)) d=$((10#$3)) dim
  [ "$m" -ge 1 ] && [ "$m" -le 12 ] || return 1
  case "$m" in
    1|3|5|7|8|10|12) dim=31 ;;
    4|6|9|11) dim=30 ;;
    2)
      if [ $((y % 4)) -eq 0 ] && { [ $((y % 100)) -ne 0 ] || [ $((y % 400)) -eq 0 ]; }; then
        dim=29
      else
        dim=28
      fi
      ;;
  esac
  [ "$d" -ge 1 ] && [ "$d" -le "$dim" ]
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
GITHUB_REPO=""
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
    --github-repo)
      [ "$#" -ge 2 ] || refuse "--github-repo needs a value"
      GITHUB_REPO="$2"
      shift 2
      ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) refuse "unknown option: $1" ;;
  esac
done

# ------------------------------------------------------- local checks

case "$TAG" in
  prod-[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9].[1-9]|prod-[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9].[1-9][0-9]) ;;
  *) refuse "TAG must look like prod-YYYY-MM-DD.N (got '$TAG')" ;;
esac
TAG_DATE="${TAG#prod-}"; TAG_DATE="${TAG_DATE%.*}"
TAG_SEQ="${TAG##*.}"
valid_date "${TAG_DATE:0:4}" "${TAG_DATE:5:2}" "${TAG_DATE:8:2}" \
  || refuse "TAG date $TAG_DATE is not a calendar date"

if [ -z "$PUSHED_AT" ]; then
  PUSHED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
fi
case "$PUSHED_AT" in
  [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z) ;;
  *) refuse "--pushed-at must be UTC like 2026-10-11T21:35:04Z (got '$PUSHED_AT'); git would silently date the tag now" ;;
esac
[ "${PUSHED_AT:0:10}" = "$TAG_DATE" ] \
  || refuse "TAG date $TAG_DATE is not the UTC date of --pushed-at $PUSHED_AT; name the tag by the UTC push date"
[ "${PUSHED_AT:11:2}" -le 23 ] && [ "${PUSHED_AT:14:2}" -le 59 ] && [ "${PUSHED_AT:17:2}" -le 59 ] \
  || refuse "--pushed-at $PUSHED_AT is not a valid time of day"

[ -s "$RECORD_FILE" ] || refuse "RECORD_FILE '$RECORD_FILE' is missing or empty"
[ "$(head -n 1 "$RECORD_FILE")" = "## Deploy record" ] \
  || refuse "RECORD_FILE '$RECORD_FILE' must start with the line '## Deploy record' (is it the right file?)"
# The record is published on a public page. Refuse a line that looks like a
# credential: a database or broker connection string, a URL with userinfo
# (a Sentry DSN, user:password@host), a bearer token, a GitHub or Stripe or
# OpenAI style key, a JWT, or a key/secret/token/password name followed by a
# value of six or more characters that contains a digit. Prose such as
# "token revoked" or "Bearer authentication enforced" is not a credential.
looks_like_credential() {
  awk '
    function digit(v) { return v ~ /[0-9]/ }
    {
      line = $0; low = tolower($0)
      if (low ~ /(postgres(ql)?|mysql|redis|amqp|mongodb(\+srv)?):\/\//) { print NR ": " line; next }
      if (low ~ /:\/\/[^\/@ \t]+@/) { print NR ": " line; next }
      if (match(low, /bearer[ \t]+[a-z0-9._~+\/=-]{12,}/) && digit(substr(low, RSTART, RLENGTH))) { print NR ": " line; next }
      if (low ~ /gh[pousr]_[a-z0-9]{20,}/ || low ~ /github_pat_[a-z0-9_]{20,}/) { print NR ": " line; next }
      if (low ~ /(^|[^a-z0-9])sk[-_](live_|test_|proj-)?[a-z0-9_-]{16,}/) { print NR ": " line; next }
      if (low ~ /eyj[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\.[a-z0-9_-]+/) { print NR ": " line; next }
      if (match(low, /(^|[^a-z0-9])[a-z0-9_]*(key|secret|token|password|passwd|pwd)[a-z0-9_]*["\047]?[ \t]*[:=][ \t]*["\047]?[a-z0-9._~+\/=-]{6,}/)) {
        v = substr(low, RSTART, RLENGTH); sub(/^.*[:=][ \t]*["\047]?/, "", v)
        if (digit(v)) { print NR ": " line; next }
      }
    }' "$1"
}
CREDENTIAL_LINES="$(looks_like_credential "$RECORD_FILE")"
if [ -n "$CREDENTIAL_LINES" ]; then
  echo "REFUSED: RECORD_FILE '$RECORD_FILE' looks like it carries a credential; the release body is public. Offending line(s):" >&2
  printf '%s\n' "$CREDENTIAL_LINES" | cut -c1-120 >&2
  exit 1
fi

command -v gh >/dev/null 2>&1 || refuse "gh is not installed"
gh auth status >/dev/null 2>&1 || refuse "gh is not logged in (gh auth status)"

if [ -z "$GITHUB_REPO" ]; then
  REMOTE_URL="$(git remote get-url "$REMOTE" 2>/dev/null)" || refuse "no git remote named '$REMOTE'"
  case "$REMOTE_URL" in
    git@github.com:*) GITHUB_REPO="${REMOTE_URL#git@github.com:}" ;;
    ssh://git@github.com/*) GITHUB_REPO="${REMOTE_URL#ssh://git@github.com/}" ;;
    https://github.com/*) GITHUB_REPO="${REMOTE_URL#https://github.com/}" ;;
    *) refuse "remote '$REMOTE' ($REMOTE_URL) is not on github.com; pass --github-repo OWNER/REPO" ;;
  esac
  GITHUB_REPO="${GITHUB_REPO%.git}"
  GITHUB_REPO="${GITHUB_REPO%/}"
fi
case "$GITHUB_REPO" in
  */*) ;;
  *) refuse "--github-repo must be OWNER/REPO (got '$GITHUB_REPO')" ;;
esac

CAN_PUSH="$(gh api "repos/$GITHUB_REPO" --jq '.permissions.push' 2>/dev/null || true)"
[ "$CAN_PUSH" = "true" ] \
  || refuse "gh has no push permission on $GITHUB_REPO (or the repository is not reachable); the release step would fail after the tag was pushed"

# ------------------------------------------------------ remote checks
# The remote is the source of truth for tags and branches: a stray local tag
# or a stale remote-tracking ref must not decide a public release.

# --no-tags: git would otherwise copy every remote tag reachable from main
# into this repository, and a local tag must never stand in for a remote one.
git fetch --no-tags --quiet "$REMOTE" "+refs/heads/main:refs/remotes/$REMOTE/main" \
  || refuse "could not fetch main from $REMOTE"

# "name commit" per remote prod-* tag; an annotated tag's peeled line wins.
REMOTE_TAG_REFS="$(git ls-remote --tags "$REMOTE" 'refs/tags/prod-*')" \
  || refuse "could not list tags on $REMOTE"
REMOTE_TAGS="$(printf '%s\n' "$REMOTE_TAG_REFS" | awk -F'\t' '
  NF == 2 { ref = $2; sha = $1
    if (sub(/\^\{\}$/, "", ref)) { peeled[ref] = sha } else { plain[ref] = sha } }
  END { for (r in plain) { n = r; sub(/^refs\/tags\//, "", n)
        print n, ((r in peeled) ? peeled[r] : plain[r]) } }' | sort -t. -k1,1 -k2,2n)"
REMOTE_TAG_NAMES="$(printf '%s\n' "$REMOTE_TAGS" | awk 'NF { print $1 }')"
# Every prod-* name on the remote must follow the scheme, or the chain's
# ordering and the same-day sequence mean nothing.
OFF_SCHEME="$(printf '%s\n' "$REMOTE_TAG_NAMES" | awk 'NF && $0 !~ /^prod-[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]\.[1-9][0-9]?$/')"
[ -z "$OFF_SCHEME" ] \
  || refuse "$REMOTE has prod-* tags outside the prod-YYYY-MM-DD.N scheme ($(printf '%s' "$OFF_SCHEME" | tr '\n' ' ')); rename or delete them before recording"
PRODUCTION_HEAD="$(git ls-remote "$REMOTE" refs/heads/production | awk '{ print $1 }')" \
  || refuse "could not read the production branch on $REMOTE"

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
  refuse "tag $TAG already exists locally (git tag -d $TAG if it is stale)"
fi
if printf '%s\n' "$REMOTE_TAG_NAMES" | grep -qx "$TAG"; then
  refuse "tag $TAG already exists on $REMOTE"
fi

TO_FULL="$(git rev-parse -q --verify "${TO_SHA}^{commit}" 2>/dev/null)" \
  || refuse "TO_SHA '$TO_SHA' is not a commit in this repository (git fetch first?)"
git merge-base --is-ancestor "$TO_FULL" "refs/remotes/$REMOTE/main" \
  || refuse "TO_SHA $TO_SHA is not on $REMOTE/main"
if [ "$MARK_LATEST" -eq 1 ]; then
  [ -n "$PRODUCTION_HEAD" ] || refuse "$REMOTE has no production branch"
  [ "$PRODUCTION_HEAD" = "$TO_FULL" ] \
    || refuse "TO_SHA $TO_SHA is not what production runs: $REMOTE/production is at ${PRODUCTION_HEAD:0:9}. Record the SHA that was pushed, or pass --not-latest when backfilling history"
fi
TAGS_AT_TO="$(printf '%s\n' "$REMOTE_TAGS" | awk -v c="$TO_FULL" '$2 == c { print $1 }')"
[ -z "$TAGS_AT_TO" ] \
  || refuse "a release already records TO_SHA $TO_SHA ($(printf '%s' "$TAGS_AT_TO" | tr '\n' ' ')); a promote that re-deploys the same SHA is not recorded"

SAME_DAY_MAX="$(printf '%s\n' "$REMOTE_TAG_NAMES" | { grep "^prod-$TAG_DATE\." || true; } | sed 's/.*\.//' | sort -n | tail -n 1)"
EXPECTED_SEQ=$(( ${SAME_DAY_MAX:-0} + 1 ))
[ "$TAG_SEQ" -eq "$EXPECTED_SEQ" ] \
  || refuse "the next tag for $TAG_DATE is prod-$TAG_DATE.$EXPECTED_SEQ (highest recorded: ${SAME_DAY_MAX:-none}); got .$TAG_SEQ"

NEWEST_TAG="$(printf '%s\n' "$REMOTE_TAG_NAMES" | awk 'NF' | tail -n 1)"
if [ -n "$NEWEST_TAG" ]; then
  NEWEST_DATE="${NEWEST_TAG#prod-}"; NEWEST_DATE="${NEWEST_DATE%.*}"
  [ "$TAG_DATE" \> "$NEWEST_DATE" ] || [ "$TAG_DATE" = "$NEWEST_DATE" ] \
    || refuse "TAG $TAG is dated before the newest tag $NEWEST_TAG; the chain is dated, so a release recorded out of order would wedge it"
fi

if [ "$BASELINE" -eq 1 ]; then
  [ "$FROM_SHA" = "-" ] \
    || refuse "--baseline takes '-' as FROM_SHA (a baseline has no predecessor); got '$FROM_SHA'"
  [ -z "$NEWEST_TAG" ] \
    || refuse "--baseline is for the first release ever, but $REMOTE already has prod-* tags (newest $NEWEST_TAG)"
  FROM_TAG=""
  FROM_SHORT="baseline"
else
  [ "$FROM_SHA" != "-" ] \
    || refuse "FROM_SHA '-' is only valid with --baseline"
  FROM_FULL="$(git rev-parse -q --verify "${FROM_SHA}^{commit}" 2>/dev/null)" \
    || refuse "FROM_SHA '$FROM_SHA' is not a commit in this repository (git fetch first?)"
  [ "$FROM_FULL" != "$TO_FULL" ] \
    || refuse "FROM_SHA and TO_SHA are the same commit; a promote that re-deploys the same SHA is not recorded"
  git merge-base --is-ancestor "$FROM_FULL" "$TO_FULL" \
    || refuse "FROM_SHA $FROM_SHA is not an ancestor of TO_SHA $TO_SHA; a rollback is not recorded as a release"
  FROM_TAGS="$(printf '%s\n' "$REMOTE_TAGS" | awk -v c="$FROM_FULL" '$2 == c { print $1 }')"
  FROM_TAG_COUNT="$(printf '%s' "$FROM_TAGS" | grep -c . || true)"
  if [ "$FROM_TAG_COUNT" -eq 0 ]; then
    if [ -z "$NEWEST_TAG" ]; then
      refuse "$REMOTE has no prod-* tag yet; record the first release ever with FROM_SHA '-' and --baseline"
    fi
    refuse "no prod-* tag on $REMOTE points at FROM_SHA $FROM_SHA (newest tag: $NEWEST_TAG); record the promote that shipped $FROM_SHA first, oldest first"
  fi
  [ "$FROM_TAG_COUNT" -eq 1 ] \
    || refuse "$FROM_TAG_COUNT prod-* tags on $REMOTE point at FROM_SHA $FROM_SHA ($(printf '%s' "$FROM_TAGS" | tr '\n' ' ')); delete the stray one, the tag with no release (check each with gh release view NAME; deleting a released tag turns that release into a draft), via git push $REMOTE :refs/tags/NAME"
  FROM_TAG="$FROM_TAGS"
  if [ "$MARK_LATEST" -eq 1 ] && [ "$FROM_TAG" != "$NEWEST_TAG" ]; then
    refuse "FROM tag $FROM_TAG is not the newest on $REMOTE ($NEWEST_TAG); the chain is linear, so FROM is the last release's SHA even after a rollback (say in the record which older SHA production ran in between). Pass --not-latest only when backfilling history"
  fi
  FROM_SHORT="$(git rev-parse --short=9 "$FROM_FULL")"
fi

TO_SHORT="$(git rev-parse --short=9 "$TO_FULL")"
TAG_MESSAGE="Production release $TAG: $FROM_SHORT -> $TO_SHORT"

# ------------------------------------------------------------- commands

set -- gh release create "$TAG" -R "$GITHUB_REPO" --verify-tag --title "$TAG" --notes-file "$RECORD_FILE"
if [ "$BASELINE" -eq 0 ]; then
  set -- "$@" --generate-notes --notes-start-tag "$FROM_TAG"
fi
if [ "$MARK_LATEST" -eq 1 ]; then
  set -- "$@" --latest
else
  set -- "$@" --latest=false
fi

RELEASE_CMD="$(printf '%q ' "$@")"

if [ "$DRY_RUN" -eq 1 ]; then
  echo "DRY RUN: every check passed; these commands would run:"
  printf '  GIT_COMMITTER_DATE=%q git tag -a %q %q -m %q\n' "$PUSHED_AT" "$TAG" "$TO_FULL" "$TAG_MESSAGE"
  printf '  git push %q %q\n' "$REMOTE" "refs/tags/$TAG"
  printf '  %s\n' "$RELEASE_CMD"
  echo "--- record file, published verbatim as the top of the public release body ---"
  cat "$RECORD_FILE"
  echo "--- end of record file ---"
  exit 0
fi

GIT_COMMITTER_DATE="$PUSHED_AT" git tag -a "$TAG" "$TO_FULL" -m "$TAG_MESSAGE" \
  || { echo "FAILED creating tag $TAG locally; nothing was pushed" >&2; exit 2; }

if ! git push "$REMOTE" "refs/tags/$TAG"; then
  echo "FAILED pushing tag $TAG; it exists locally only (undo: git tag -d $TAG; retry: rerun this command)" >&2
  exit 2
fi

if ! "$@"; then
  {
    echo "FAILED creating the release for $TAG; the tag is already on $REMOTE at $TO_SHORT."
    echo "  finish:  $RELEASE_CMD"
    echo "  or undo: git push $REMOTE :refs/tags/$TAG && git tag -d $TAG"
    echo "  Do not rerun this script with another tag name for the same SHA."
  } >&2
  exit 2
fi

echo "Recorded $TAG at $TO_SHORT (from $FROM_SHORT) pushed at $PUSHED_AT: https://github.com/$GITHUB_REPO/releases/tag/$TAG"
