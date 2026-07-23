# Label release-pass (Bandcamp discography → releases)

After roster ingest + Bandcamp links, pull each artist's Bandcamp discography into `release` entities — title, year, type, playable Bandcamp link, genre/locale tags.

**Why this matters for embeds:** each release's Bandcamp `/album` or `/track` URL (written as `external_links: [{ platform: "bandcamp", url: "…" }]`) is what powers the **artist** playable embed. On create / add-link, the backend derives `artists.bandcamp_embed_url` when empty (`release_derived`) and keeps auto-derived embeds fresh as newer release links land. A roster that only stores profile roots (`https://<artist>.bandcamp.com`) leaves artists without a reliable player until this pass (or the slower profile→embed resolver) runs. Capture `/album|/track` URLs here — not just roots.

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
7. Verify `GET /artists/{id}/releases` + `GET /entities/release/{id}/tags`
8. **Embed verification** (below) — sample artists until embeds look right
9. Optional: [artist-tag-rollup.md](artist-tag-rollup.md)

See [troubleshooting.md](troubleshooting.md) for PSY-1184 release dedup requirement on re-runs.

## Embed verification (after roster + release-pass)

Lightweight checklist so a newly ingested label roster is confirmed to feed the playable-embed pipeline. List/roster projections omit `social`, `bandcamp`, `bandcamp_embed_url`, and release `external_links` — **always use detail endpoints**.

1. **Roster landed roots where the source had them** — for a sample of artists: `GET /artists/{id}` → `social.bandcamp` is a profile root (`https://<slug>.bandcamp.com`) when the roster/hub exposed one. Names-only Shopify rosters (Sacred Bones, Dais) need [link-enrichment](link-enrichment.md) first.
2. **Release-pass wrote playable links** — for those artists: `GET /releases/{id}` (not the artist releases *list*) → `external_links` includes `platform: "bandcamp"` with an `/album/` or `/track/` URL. Prefer confirming at least one release per sampled artist that has a Bandcamp discography.
3. **Artist embed filled** — same `GET /artists/{id}` → `bandcamp_embed_url` is a non-null `/album` or `/track` URL (often matching a recent release link). Empty embed + present release Bandcamp link ⇒ re-check the link shape or re-run create/link; empty embed + no release Bandcamp links ⇒ release-pass still needed (profile resolver may fill asynchronously from the root, but do not rely on it as the primary path).
4. **Coverage sanity** — spot-check ~5–10 roster artists (mix of single-release and deep discographies). If many have roots but zero embeds and zero release Bandcamp links, the release-pass did not run or skipped that artist.

Do **not** treat label/venue Bandcamp fields as embed sources — artist embeds only.
