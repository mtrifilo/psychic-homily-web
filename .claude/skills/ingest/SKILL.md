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

**Pipeline completion (scoped):**
- Roster alone completes the label↔artist graph.
- Link enrichment + release-pass are **required for playable artist embeds** (optional only if you explicitly accept names-only / no-player). Names-only Shopify rosters (Sacred Bones, Dais) need link enrichment before release-pass.
- Artist tag rollup remains optional.

**Release-pass is the primary feeder of artist playable embeds.** Artist pages need `bandcamp_embed_url` = a Bandcamp `/album` or `/track` URL — a bare profile root in `social.bandcamp` is not enough by itself. Writing release `external_links` with an embeddable `/album|/track` URL (what [release-pass](references/release-pass.md) captures; conventionally `platform: "bandcamp"`) **fills empty** artist embeds on create/add-link (`release_derived`, fill-when-empty — first qualifying link wins; newer releases do not overwrite). Auto-derived embeds are recomputed only when a release/link is removed. Profile roots still help: link enrichment + the profile→embed resolver can fill when no release link exists yet, but do not treat that as a substitute for release-pass. Labels/venues do not get embeds today (deferred). Checklist: [release-pass.md § Embed verification](references/release-pass.md#embed-verification-after-roster--release-pass).

## Shared rules (all workflows)

1. **Dry-run → explicit user OK → `--confirm`** — never skip confirmation on bulk writes.
2. **Social URLs must be full on-platform URLs** — bare `@handles` are rejected. Bluesky does **not** go on `twitter` (422).
3. **Verify via detail endpoints** — list/roster projections omit `social`/`bandcamp`/`bandcamp_embed_url` (and release list omits `external_links`); use `GET /artists/{id}` and `GET /releases/{id}`, not `GET /labels/{id}/artists`, for link + embed checks.
4. **Artist-skip QA scan** on large rosters/calendars — map SKIPs to proposed names; pre-create distinct artists via `POST /admin/artists` (not `ph submit artist`) to beat 0.6 fuzzy false-matches.
5. **Don't auto-split `and`/`&`/`,` in band names** unless the source clearly lists separate acts.
6. **Register + stamp sources** after successful venue/label ingests so [catalog-refresh](references/catalog-refresh.md) can pick them up.
7. **Radio playlist linking** — playlist orange ● requires `radio_plays.artist_id`. `batch --confirm` auto-runs `ph radio rematch` (chunked); artist/label create also rematch async. See [troubleshooting.md](references/troubleshooting.md#radio-playlist-linking) and [radio-playlist.md](references/radio-playlist.md).


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
| [release-pass.md](references/release-pass.md) | Bandcamp `#music-grid` parser, workflow, embed verification, PSY-1173 gate |
| [artist-tag-rollup.md](references/artist-tag-rollup.md) | Release keywords → artist genre/locale tags |
| [tag-allowlist.md](references/tag-allowlist.md) | GENRES/LOCALES allowlist + promotion loop |
| [label-discography.md](references/label-discography.md) | CAT – Artist – Title catalogue pages |
| [catalog-refresh.md](references/catalog-refresh.md) | `ph sources stale` loop |
| [radio-playlist.md](references/radio-playlist.md) | Radio archive backfill (stub) |
| [troubleshooting.md](references/troubleshooting.md) | Cross-workflow gotchas |
