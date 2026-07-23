# Label release-pass (Bandcamp discography → releases)

After roster ingest + Bandcamp links, pull each artist's Bandcamp discography into `release` entities — title, year, type, playable Bandcamp link, genre/locale tags.

**Why this matters for embeds:** each release's Bandcamp `/album` or `/track` URL (written as `external_links: [{ platform: "bandcamp", url: "…" }]`) is what powers the **artist** playable embed. On create / add-link, the backend **fills** `artists.bandcamp_embed_url` when empty (`release_derived`, fill-when-empty — first qualifying link wins; later releases do not overwrite an already-set embed). Auto-derived embeds are recomputed only when a release or its Bandcamp link is removed (so a deleted featured release does not leave a stale URL). A roster that only stores profile roots (`https://<artist>.bandcamp.com`) leaves artists without a reliable player until this pass (or the slower profile→embed resolver) runs. Capture `/album|/track` URLs here — not just roots.

**Prerequisites:** artists exist; Bandcamp roots on artists ([link-enrichment.md](link-enrichment.md) if needed).

**Proven:** 12XU (133), Feel It (407), Dais (927 / 501 new, 2026-07-03).

**Gate — tags need PSY-1173** (`phk_` rate-limit bypass) on target env. Releases apply without it; tags 429 without bypass.

## Per-artist extraction

1. **Discography = `#music-grid` ONLY.** Fetch `<artist>/music`. Parse `<ol id="music-grid">` `/album/` + `/track/` hrefs. **Do NOT regex whole page** — `*label` redirects explode tracklists into fake singles. No grid → single album from redirect/`og:url`.
2. **Fields from JSON-LD** `MusicAlbum`/`MusicRecording`: name, `datePublished` year, `numTracks`, `keywords`.
3. **`release_type`:** `/track` or ≤2 tracks → `single`; 3–6 → `ep`; 7+ → `lp`.
4. **Artist name must match stage entity exactly** — patch roster display vs stage form on `Unresolved artists:`.

## Multi-label

Bandcamp release pages don't expose record label — don't try. Cross-label links happen at **artist level** via multiple roster ingests.

## Tags

Allowlist + promotion loop — see [tag-allowlist.md](tag-allowlist.md).

## Workflow

1. Re-derive Bandcamp roots (roster list omits `bandcamp`) — re-parse hub or read artist detail
2. **Sample 3–5 artists first** (single-release `*label` page + own-domain)
3. Extract all → cache raw + batch + dropped-keyword report. **≥600ms between requests**, retry empty grids, **resume file** for interrupted runs
4. Promote tags, rebuild from cache
5. Dry-run → fix unresolved → 0 unresolved
6. Confirm (PSY-1173 for tags)
7. Verify tags via `GET /entities/release/{id}/tags`; verify Bandcamp links + artist embeds via the checklist below (artist releases *list* omits `external_links`)
8. **Embed verification** (below) — sample artists until embeds look right
9. Optional: [artist-tag-rollup.md](artist-tag-rollup.md)

See [troubleshooting.md](troubleshooting.md) for PSY-1184 release dedup requirement on re-runs.

## Embed verification (after roster + release-pass)

Lightweight checklist so a newly ingested label roster is confirmed to feed the playable-embed pipeline. Run steps 2–4 only after release-pass (or after you knowingly skipped it and accept no-player). List/roster projections omit `social`, `bandcamp`, `bandcamp_embed_url`, and release `external_links` — **always use detail endpoints**.

1. **Roster landed roots where the source had them** (post-roster / post-link-enrichment) — for a sample of artists: `GET /artists/{id}` → `social.bandcamp` is a profile root (`https://<slug>.bandcamp.com`) when the roster/hub exposed one. Names-only Shopify rosters (Sacred Bones, Dais) need [link-enrichment](link-enrichment.md) first.
2. **Release-pass wrote playable links** — discover release IDs via `GET /artists/{id}/releases`, then for each sampled ID: `GET /releases/{id}` → `external_links` includes an embeddable `/album/` or `/track/` URL (release-pass writes these as `platform: "bandcamp"`; the embed gate is URL shape, not the platform label). Prefer confirming at least one such release per sampled artist that has a Bandcamp discography.
3. **Artist embed filled** — same `GET /artists/{id}` → `bandcamp_embed_url` is a non-null `/album` or `/track` URL (any valid release link is fine — **first fill wins**, not necessarily the newest). Empty embed + present embeddable release link ⇒ check URL path is `/album|/track`, confirm the artist is credited on that release; if links predate the keep-fresh hooks, run `BackfillArtistBandcampEmbeds` (or add the link once via the release links endpoint) — do **not** recreate releases. Empty embed + no embeddable release links ⇒ release-pass still needed (profile resolver may fill asynchronously from the root; treat that as a fallback, not the primary path).
4. **Coverage sanity** — spot-check ~5–10 roster artists (mix of single-release and deep discographies). If many have roots but zero embeds and zero release Bandcamp links, the release-pass did not run or skipped that artist.

Do **not** treat label/venue Bandcamp fields as embed sources — artist embeds only.
