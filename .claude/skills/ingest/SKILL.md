---
name: ingest
description: Extract and import entities into the Psychic Homily knowledge graph via the ph CLI — screenshots/posts, venue calendars, label rosters, Bandcamp release-pass, link enrichment, discography pages, catalog refresh, and (stub) radio playlist backfill. Load references/ docs for the matching workflow before executing.
argument-hint: "[dev|stage|prod] [screenshot/post, venue URL, label roster, enrich links, release-pass, stale refresh, …]"
---

# Ingest: Knowledge Graph Import

Import structured entities via the `ph` CLI. This file is the **router** — read the matching reference doc before executing a workflow.

## Environment targeting

Default: whichever env is set in `~/.psychic-homily/config.json`.

- `/ingest dev` → local · `/ingest stage` → staging · `/ingest prod` → production
- `/ingest --env <name> ...` also works

**Parsing:** If the first word is `dev`, `local`, `stage`, `staging`, `prod`, or `production`, strip it and append `--env <name>` to **all** `ph` commands.

**Resolve shorthand against configured names** — CLI does exact match. Run `config show` and map: `dev`/`local` → local env, `stage`/`staging` → staging env, `prod`/`production` → production env. Configured names are typically `local`, `stage`, `production` (**`stage`, not `staging`**).

```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts config show
```

## Prerequisites

```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts status
```

If not configured: `go run ./cmd/gen-api-token --make-admin` in `backend/`, then `ph init --url … --token phk_…`.

## Workflow router

**MUST read the linked reference before executing.**

| User intent / trigger | Read first |
| --- | --- |
| Screenshot, flyer, Instagram post, WFMU playlist image, tour poster, festival lineup | [references/screenshot-batch.md](references/screenshot-batch.md) |
| Venue events page — add or refresh calendar | [references/venue-events.md](references/venue-events.md) |
| Label artists/roster page | [references/label-roster.md](references/label-roster.md) |
| Enrich missing artist/venue links after ingest | [references/link-enrichment.md](references/link-enrichment.md) |
| Bandcamp discography → releases (+ release tags) | [references/release-pass.md](references/release-pass.md) |
| Roll release keywords up to artist tags | [references/artist-tag-rollup.md](references/artist-tag-rollup.md) |
| Flat catalogue/discography page (defunct labels) | [references/label-discography.md](references/label-discography.md) |
| Refresh N stalest registered sources | [references/catalog-refresh.md](references/catalog-refresh.md) |
| Radio station historical playlists / archive backfill | [references/radio-playlist.md](references/radio-playlist.md) — **admin backfill API**, not `ph batch` |
| Gotchas (show dedup, timezone, verify endpoints, 422s) | [references/troubleshooting.md](references/troubleshooting.md) |
| Tag allowlist for release-pass / rollup | [references/tag-allowlist.md](references/tag-allowlist.md) |

Machine-readable extraction rules: `cli/eval/extraction-prompt.md` + `cli/eval/batch-schema.json` (keep in sync with screenshot-batch prose).

## Label enrichment pipeline

Typical order after a new label roster:

```
roster ingest → link enrichment → release-pass → artist tag rollup
```

Each step after roster is optional. Names-only Shopify rosters (Sacred Bones, Dais) need link enrichment before release-pass.

## Shared rules (all workflows)

1. **Dry-run → explicit user OK → `--confirm`** — never skip confirmation on bulk writes.
2. **Social URLs must be full on-platform URLs** — bare `@handles` are rejected. Bluesky does **not** go on `twitter` (422).
3. **Verify via detail endpoints** — list/roster projections omit `social`/`bandcamp`; use `GET /artists/{id}`, not `GET /labels/{id}/artists`, for link checks.
4. **Artist-skip QA scan** on large rosters/calendars — map SKIPs to proposed names; pre-create distinct artists via `POST /admin/artists` (not `ph submit artist`) to beat 0.6 fuzzy false-matches.
5. **Don't auto-split `and`/`&`/`,` in band names** unless the source clearly lists separate acts.
6. **Register + stamp sources** after successful venue/label ingests so [catalog-refresh](references/catalog-refresh.md) can pick them up.

## CLI quick reference

```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli

# Batch (default dry-run)
bun run src/entry.ts --env <env> batch /tmp/ph-ingest.json
bun run src/entry.ts --env <env> batch --confirm /tmp/ph-ingest.json

# Search
bun run src/entry.ts search artist|venue|release|label "<query>"

# Submit single entity types (add --confirm to write)
bun run src/entry.ts submit artist|venue|show|release|label|festival '<json>'

# Source registry
bun run src/entry.ts --env <env> sources stale --limit 20
bun run src/entry.ts --env <env> sources register venue|label <id> "<url>"
bun run src/entry.ts --env <env> sources refresh venue|label <id>
```

## References index

| File | Contents |
| --- | --- |
| [screenshot-batch.md](references/screenshot-batch.md) | Screenshot/post extraction, batch JSON schemas, dry-run ceremony |
| [venue-events.md](references/venue-events.md) | Venue calendar ingest, transform skeleton, venue registry |
| [label-roster.md](references/label-roster.md) | Label roster ingest, inline `artists` shape, label registry |
| [link-enrichment.md](references/link-enrichment.md) | Bandcamp hub, MusicBrainz, PATCH follow-up |
| [release-pass.md](references/release-pass.md) | Bandcamp `#music-grid` parser, workflow, PSY-1173 gate |
| [artist-tag-rollup.md](references/artist-tag-rollup.md) | Release keywords → artist genre/locale tags |
| [tag-allowlist.md](references/tag-allowlist.md) | GENRES/LOCALES allowlist + promotion loop |
| [label-discography.md](references/label-discography.md) | CAT – Artist – Title catalogue pages |
| [catalog-refresh.md](references/catalog-refresh.md) | `ph sources stale` loop |
| [radio-playlist.md](references/radio-playlist.md) | Radio archive backfill (stub) |
| [troubleshooting.md](references/troubleshooting.md) | Cross-workflow gotchas |
