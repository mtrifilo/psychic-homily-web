---
name: psy-deploy-prod
description: Promote the current stage state to production (Railway backend + Vercel frontend) via the production branch. Use when the user says "deploy to prod", "promote to production", "release what's on stage", or asks to update the live site after testing on stage. Covers preflight checks, the branch fast-forward, deploy monitoring, the post-deploy smoke checklist, and rollback. Optionally includes a stage→prod DB restore for data-refresh releases.
argument-hint: "[optional: --with-db-restore]"
---

# psy-deploy-prod: promote stage → production

Production deploy topology (verified 2026-07-23, first formal cutover):

- **Both prod targets track the git branch `production`.** Railway prod backend (project `psychic-homily`, env `production`, service `psychic-homily-web`) builds `backend/Dockerfile` from it; Vercel project `psychic-homily-web` builds the frontend Production deployment from it. Previews build from every other branch.
- **A deploy is one fast-forward push.** No GitHub Action is involved; Railway and Vercel GitHub integrations react to the branch moving.
- **Backend boot runs `migrate up`** (`backend/docker-entrypoint.sh`) against `DATABASE_URL` before starting the server — pending migrations apply automatically on deploy. A failed migration fails the deploy (healthcheck `/health` never passes; Railway keeps the old deployment serving).
- Stage backend (`passionate-art`, env `stage`) deploys from `main` — stage IS main. "Tested on stage" = tested on main.
- Prod domains: `www.psychichomily.com` (Vercel) + `api.psychichomily.com` (Railway). Redis services exist but are UNUSED by the backend (verified: zero Go references) — ignore them.

## Preflight (all must pass before pushing)

```bash
# 1. Main's TIP run is COMPLETED and green — read per-JOB conclusions, not the roll-up
gh run list --branch main --limit 3 --json status,conclusion,displayTitle
#   status still in_progress → WAIT. Never promote a commit whose CI hasn't finished.
#   conclusion=failure is NOT automatically a blocker — find out WHICH job failed:
gh run view <run-id> --json jobs --jq '.jobs[] | "\(.conclusion // .status)\t\(.name)"'
#   THREE distinct kinds of red — classify before deciding:
#   1. "E2E Merged Report" red, all 4 shards green = artifact-download flake
#      (2026-07-24). Cosmetic, non-blocking.
#   2. A SHARD red whose log shows `docker compose up failed after 3 attempts
#      (likely a Docker Hub registry timeout)` = the stack never started, ZERO
#      tests ran (2026-07-25). Not a code failure — but DON'T wave it through:
#      `gh run rerun <run-id> --failed` and require a genuine pass. (Rerun is
#      refused while any job is still in progress: "workflow is already
#      running" — wait for the run to complete first.)
#   3. A shard red with real `✘ <spec>` lines = STOP, it's a test failure.
#   Confirm which by grepping the job log for actual failure markers:
gh api "repos/mtrifilo/psychic-homily-web/actions/jobs/<job-id>/logs" | grep -E "✘|Error:"
#   Any Backend/Frontend/Lint/Migration job red = STOP.
cd backend && railway status                  # sanity: CLI linked to the project

# 2. The push is a fast-forward (production must be an ancestor of main)
git fetch origin main production
git merge-base --is-ancestor origin/production origin/main && echo FF-OK
# If NOT FF-OK: STOP. Someone committed to production directly. Reconcile first.

# 3. Pending migrations — ASK PROD, don't eyeball or trust notes (decisive, ~2s)
cd backend
PRODURL="$(railway variables --json -s Postgres -e production | jq -r '.DATABASE_PUBLIC_URL')"
psql "$PRODURL" -tAc 'SELECT version, dirty FROM schema_migrations;'   # applied version + clean?
ls db/migrations/*.up.sql | tail -1 | xargs basename                   # repo head
git diff --name-only origin/production..origin/main -- ':(top)backend/db/migrations/'  # release adds?
#   NOTE the ':(top)' prefix on every pathspec below. We are in backend/ from here on,
#   and git pathspecs are CWD-RELATIVE — a bare 'backend/...' resolves to
#   backend/backend/... and silently matches NOTHING, which these checks would then
#   read as "this release changes nothing". Measured: 'backend/**/*.go' matches 0 files
#   from backend/ and 23 from the repo root. An empty result here must mean "no changes",
#   never "wrong directory".
#   version == repo head + dirty=f + empty diff → ZERO migrations apply on boot (lowest risk).
#   dirty=t → a prior migration half-applied: STOP and resolve before deploying.
#   Non-empty diff → those exact files auto-apply on boot; review each before pushing.

# 4. Postgres volume headroom (PSY-1643). ~5s, and it is the check whose absence cost
#    a 75-minute outage on 2026-07-29.
bash ../scripts/check-volume-headroom.sh production   # repo-root script; we are in backend/
#   Exit 0 = fine. Exit 1 = STOP: the volume is smaller than database + max_wal_size,
#   so it will fill no matter how empty it looks. Exit 2 = warn, plan a resize.
#   Exit 3 = COULD NOT DETERMINE (psql/jq/railway missing, environment not linkable,
#   volume name mismatch). Treat as STOP. An unrun check is not a passed check — an
#   unverified volume is exactly the state prod was in before it filled.
#
#   Why a FLOOR and not "is it 80% full": prod ran a 500 MB volume with
#   max_wal_size=1024 MB. Postgres was configured to use twice the whole disk for WAL
#   alone. A percentage alert says nothing there — the volume was doomed at 0% used.
#   The script links/relinks the environment itself (`railway volume list` has no -e),
#   and restores the previous link even on failure.

# 5. New env vars? Grep the RELEASE RANGE for added config reads; anything new must be
#    set on Railway prod service `psychic-homily-web` and/or Vercel Production BEFORE pushing.
git diff origin/production..origin/main -- ':(top)backend/**/*.go' \
  | grep -E '^\+.*os\.Getenv' | grep -oE 'os\.Getenv\("[A-Z_]+"\)' | sort -u
git diff origin/production..origin/main -- ':(top)frontend/**' \
  | grep -E '^\+.*process\.env\.' | grep -oE 'process\.env\.[A-Z_]+' | sort -u
#    Both empty → release adds no config. Name-level parity check:
railway variables --json -s passionate-art -e stage | jq -r 'keys[]' | sort > /tmp/s.txt
railway variables --json -s psychic-homily-web -e production | jq -r 'keys[]' | sort > /tmp/p.txt
diff /tmp/s.txt /tmp/p.txt   # expect only RAILWAY_SERVICE_* name difference + sweep-flag posture
```

Secret hygiene: NEVER print env var values. Compare names, lengths (`jq '.KEY|length'`), and flag values (`ENABLE_*`/`DISABLE_*`/`LOG_*` are non-secret) only. Copy a secret between envs by capturing into a shell var without echoing.

## Optional: stage → prod DB restore (data-refresh release)

Only when the release intent is "prod data should match stage" (rare after launch; normal releases keep prod data and rely on boot-time migrations). Requires explicit user approval — it is destructive.

Run the whole block under `set -euo pipefail`. Without it a failed `pg_dump`
does not stop the script, and the next line DROPs production — destroying the
data with no rollback artifact. `railway variables` is known to return empty
outside the linked directory (see Gotchas), so an unset URL is a live failure
mode, not a hypothetical one.

```bash
set -euo pipefail
cd backend   # railway CLI must run from the linked project dir
PRODURL="$(railway variables --json -s Postgres -e production | jq -r '.DATABASE_PUBLIC_URL')"
STAGEURL="$(railway variables --json -s Postgres -e stage | jq -r '.DATABASE_PUBLIC_URL')"
ARCHIVE=~/dev/psychic-homily-backups/prod-archive-$(date +%F).dump
SNAPSHOT=~/dev/psychic-homily-backups/stage-snapshot-$(date +%F).dump

# Assert both URLs resolved and PROD really is prod, programmatically — not by
# eyeballing a comment.
[ -n "$PRODURL" ] && [ -n "$STAGEURL" ] || { echo "ABORT: a database URL is empty"; exit 1; }
case "$PRODURL" in *shuttle.proxy.rlwy.net:24983*) ;; *) echo "ABORT: PRODURL is not production"; exit 1;; esac

pg_dump "$PRODURL" -Fc -f "$ARCHIVE"      # rollback artifact
pg_dump "$STAGEURL" -Fc -f "$SNAPSHOT"

# GATE: never DROP without a verified rollback artifact. `pg_dump` can die
# partway through — a network blip to the Railway proxy, a full local disk, an
# interrupted long dump — and leave a truncated file that looks fine by name.
pg_restore --list "$ARCHIVE" >/dev/null || { echo "ABORT: prod archive is not a valid dump"; exit 1; }
pg_restore --list "$SNAPSHOT" >/dev/null || { echo "ABORT: stage snapshot is not a valid dump"; exit 1; }

psql "$PRODURL" -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
pg_restore --no-owner --no-privileges -d "$PRODURL" "$SNAPSHOT"
# Scrub stage-only auth state:
psql "$PRODURL" -c 'TRUNCATE webauthn_challenges; TRUNCATE api_tokens;'
# PSY-1612 — REQUIRED, or the release plants a week of daily false Sentry alerts.
# background_service_runs came over from stage WITH stage's registration
# timestamps, and stage's health check re-stamps them every ~15 minutes, so every
# row lands in prod looking freshly registered. The sweeps stage runs and prod does
# not (the ENABLE_* ones) therefore read as STALLED in prod and page once a day
# until they age out of the 7-day retirement window. Retiring them all is safe:
# every loop prod actually runs re-registers itself within one health-check pass.
psql "$PRODURL" -c 'UPDATE background_service_runs SET last_registered_at = NULL, last_overdue_alert_at = NULL;'
# webauthn_credentials are RP-ID-bound to stage.psychichomily.com — truncate unless empty.
# Verify: schema_migrations matches repo head + row counts match stage.
```

## The deploy

```bash
git push origin origin/main:refs/heads/production
```

Push the **CI-verified SHA**, not `origin/main` — main can move between the CI check and the push:

```bash
git push origin <verified-sha>:refs/heads/production
```

**Then WAIT ~60s and check before doing anything else** — both Vercel and Railway normally auto-deploy from this push:

```bash
railway deployment list -s psychic-homily-web -e production --json \
  | jq -r '.[0] | "\(.status) commit=\(.meta.commitHash // "CLI-upload")"'
```

A row with `commit=<your sha>` means the integration fired — **you are done; do NOT run `railway up`.** A CLI upload started in parallel WINS THE RACE and replaces the GitHub-sourced deployment with one carrying no commit provenance (hit 2026-07-24; the redundant upload had to be cancelled).

**PSY-1526 fallback — only if no deployment appears after ~1-2 min.** At the launch cutover Railway ignored the production-branch push entirely (trigger configured correctly, yet nothing fired, and `serviceInstanceDeploy` rebuilt a stale months-old commit). It has NOT recurred since — three consecutive releases (2026-07-24 ×2, 2026-07-25) auto-deployed with correct commit provenance, so treat the cutover as a one-off from a 5-month-dormant branch. If it ever happens again:

```bash
# Clean worktree of the release commit — NEVER railway-up your working tree
git worktree add /tmp/prod-deploy <release-sha>
# Upload the REPO ROOT (service rootDirectory=/backend resolves inside the archive)
railway up /tmp/prod-deploy --path-as-root -s psychic-homily-web -e production -d
git worktree remove /tmp/prod-deploy
```

Then monitor both builds (~3–5 min):

```bash
railway deployment list -s psychic-homily-web -e production --json \
  | jq -r '.[0] | "\(.status) commit=\(.meta.commitHash // "CLI")"'   # SUCCESS + your sha
cd frontend && vercel ls --prod | sed -n '5,7p'                        # row 2 = newest; ● Ready
curl -s -o /dev/null -w '%{http_code}\n' https://api.psychichomily.com/health              # 200 (liveness only)
# LIVENESS IS NOT ENOUGH. /health returns 200 whenever the process is serving, even with
# the database completely down — that is exactly how the 2026-07-29 outage looked healthy
# from outside. /health/ready is the one that proves the backend can do its job:
curl -s -o /dev/null -w '%{http_code}\n' https://api.psychichomily.com/health/ready        # 200 REQUIRED
#   503 here = the process is up but a critical dependency is unreachable. Do NOT call the
#   release good on a 200 from /health alone.
```

Don't script `vercel ls` output by awk column index — the column positions shift and a
poll loop silently reports empty forever (bit us twice). Grep for `● Ready` instead, or
just read `sed -n '5,7p'` by eye.

**When the release contains a migration, a green deployment badge is NOT sufficient.**
Verify it actually applied, and check the neighbours a destructive migration could touch:

```bash
railway logs -s psychic-homily-web -e production --lines 150 | grep -i migrat
#   expect "Running database migrations..." → "Migrations completed successfully!"
psql "$PRODURL" -tAc 'SELECT version, dirty FROM schema_migrations;'   # advanced + dirty=f
#   For a DROP: confirm the table is gone AND the FK-parent tables still hold their rows.
```

If the Railway deploy fails on migration: the old deployment keeps serving; fix forward (new migration) or roll back the push target — do NOT hand-edit prod schema.

## Post-deploy smoke (minimum bar)

1. `www.psychichomily.com` serves the new build (check a visible change from the release).
2. Log in with password (test user `matt.trifilo@gmail.com`); admin account `psychichomily@gmail.com` sees admin nav.
3. One read journey (artist page or /shows) + one write journey (comment, follow, or save) — no CORS errors in console.
4. Skim Railway logs for recurring errors; confirm Sentry isn't lighting up.
5. `curl -s -o /dev/null -w '%{http_code}' https://api.psychichomily.com/sitemap/entries` → **200**, and `curl -s https://psychichomily.com/sitemap.xml | grep -c '<loc>'` → thousands, not dozens.

**Backend-first ordering (PSY-1621).** Vercel and Railway both react to the same `production` branch push and deploy in PARALLEL with no ordering guarantee — a Next build finishes well before a Go build + `migrate up` + healthcheck. `/sitemap.xml` fetches `/sitemap/entries` and **fails closed**, so during that window the frontend 500s on `/sitemap.xml` against a backend that doesn't have the route yet. Self-heals in minutes, but on any release that adds or changes a backend endpoint the frontend depends on, verify the backend is live *before* treating the release as done. This route is dynamic (`ƒ`, no ISR window) — there is no cached document to stale-serve while it catches up.

For a full launch-grade checklist see `docs/runbooks/production-deploy.md` (topology detail, env inventory, rollback, and the 2026-07 cutover record).

## Rollback

- **Frontend**: Vercel dashboard → promote the previous Production deployment (instant), or `vercel rollback`.
- **Backend**: Railway dashboard → redeploy the previous deployment (previous image, instant).
- **DB**: only needed if a migration or restore went wrong — restore the archive dump from `~/dev/psychic-homily-backups/`.
- **Branch**: never force-push `production` backwards while investigating; roll back the *deployments*, then fix forward on main.

## Gotchas learned the hard way

- `railway variables` returns empty (and pg_dump silently falls back to a local socket) when run OUTSIDE the linked project dir — always `cd backend` first.
- `vercel env pull` values can carry a trailing newline into the stored value (bit us on `BACKEND_URL`, fixed 2026-07-23 with `printf '<url>' | vercel env add`). Never `echo "<val>" | vercel env add` — use `printf '%s'`.
- Vercel CLI needs the team scope: the linked project lives under `matts-projects-722d5204`; run from `frontend/` (has `.vercel/project.json`).
- Flag posture settled by PSY-1523 (2026-07-24): all five `ENABLE_*_SWEEP` = 1 on BOTH envs; `ENABLE_ENGAGEMENT_MUTATION_RATE_LIMITS=1` both; `DISABLE_RADIO_FETCH=1` on **stage only** (prod is the sole radio poller). Deploy cadence is MANUAL promote — never auto-promote main.
- Changing a prod env var: use `--skip-deploys`, then redeploy the known-good image via the `deploymentRedeploy(id:)` GraphQL mutation. A bare `railway variables --set` on prod triggers a source redeploy that can rebuild a stale commit while PSY-1526 is open; `railway redeploy` ignores `-e` and uses the linked env.
- `railway up` archive root must be the REPO root, not `backend/` — the prod service sets `rootDirectory=/backend` and the build fails fast with `lstat .../backend: no such file or directory` if the archive root IS backend.
- **Verify prod state from prod, never from notes.** On 2026-07-24 the project memory claimed prod was "at migration 32/141" (i.e. ~126 pending); the actual `schema_migrations` query showed prod at repo head with `dirty=f` — zero pending. Acting on the stale note would have mis-scoped a routine deploy as a high-risk one. Same rule for "what's deployed": read `origin/production`'s SHA, don't infer from what merged.
- **A prod-only bug report is a deploy-lag hypothesis first.** When a fix is merged to main but the symptom persists on `psychichomily.com`, check `git branch -r --contains <merge-sha>` and `origin/production` before re-debugging the code — prod deploys are a MANUAL promote, so main can sit many commits ahead. (2026-07-24: the PSY-1510 Dial-link fix was reported "still broken in prod" while production sat at the pre-dispatch SHA.)
- The full E2E suite runs post-merge only — a green PR does not mean a green main. Check `gh run list --branch main` conclusions (not just "did CI run") before promoting; the launch cutover shipped a red-CI commit because this check was skipped (PSY-1525).
