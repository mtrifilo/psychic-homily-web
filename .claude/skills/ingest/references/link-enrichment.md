# Per-entity link enrichment

Roster / calendar ingests often land entities **names-only**. This pass fills missing external links on **already-ingested artists and venues** by web-research, verify, PATCH. No backend name→link discovery — write-back uses admin PATCH endpoints. **Dry-run → confirm**, never guess.

## Invoking

- **Tail of ingest:** "…then enrich the names-only entries' links."
- **Standalone:** `/ingest <env> — enrich the N artists missing links on label <id>` (or `venue <id>`, streaming worklist)

## Work-source (cap N ≈ 10–15/run)

- **Just-ingested:** entities created this run with no link
- **Label/venue:** `GET /labels/{id}/artists` → `GET /artists/{id}` **detail** for empty `social.*` (list omits social)
- **Show artists:** `GET /admin/streaming-worklist`

## Find links (per entity)

Full on-platform URLs (same host rules as [screenshot-batch.md](screenshot-batch.md)):

- **artist:** Bandcamp first, then Spotify, Instagram, website
- **venue:** website + Instagram (`"<name>" <city> venue`)

### Shopify internal-collections rosters (Sacred Bones, Dais, 4AD…)

Cards link to `/collections/<slug>` with **no external socials**:

1. **Label Bandcamp hub** — scrape `labelname.bandcamp.com/music` for `https://<sub>.bandcamp.com/album/…`; subdomain = artist root. Map via JSON-LD `byArtist` or normalize-and-match. (Dais: 53/83 gaps filled.)
2. **MusicBrainz url-rels** — `inc=url-rels`; **skip ambiguous short names** (DIY, Coil, Prurient, Iceage, Link, RAC…).
3. Manual subdomain checks for hub misses (`littleannieanxiety.bandcamp.com`, `youmusic.bandcamp.com`).

Collection pages rarely expose artist socials — don't bother scraping them.

## Verify before applying

Apply only when **name matches AND second signal corroborates**:

- **artist:** genre/hometown/release fits roster label context. Same-name collisions are rife — the dedup section's "Fan Club → Yot Club" lesson applies to web search too.
- **venue:** city **plus** independent signal on the candidate page — venue's own site stating city+state, capacity/booking, or cross-link from calendar source. `address`/`zipcode` redacted for unverified venues. **City alone does NOT clear the bar** for ambiguous names ("The Echo", "Lincoln Hall") → SKIP.

**Open the candidate page and confirm the corroborating signal on that page** — search snippets are not corroboration. Can't corroborate → **SKIP**.

Worklist-sourced artists only: `POST /admin/artists/{id}/streaming-discovery-status` → `linked` or `no_links_found`. Roster/venue-sourced entities have no worklist row — don't POST status for them.

## Dry-run → confirm → PATCH

1. Preview per entity: links + evidence; pause for OK
2. **artist:** `PATCH /admin/artists/{id}` (all social fields)
3. **venue:** `PUT /venues/{id}` (partial body)
4. Verify via detail endpoints

> Targeted PATCH beats re-ingest batch when links come from web research, not the source page.

## Pipeline position

Typically after [label-roster.md](label-roster.md) or [venue-events.md](venue-events.md), before [release-pass.md](release-pass.md).
