# Label release-pass (Bandcamp discography → releases)

After roster ingest + Bandcamp links, pull each artist's Bandcamp discography into `release` entities — title, year, type, playable Bandcamp link, genre/locale tags.

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
8. Optional: [artist-tag-rollup.md](artist-tag-rollup.md)

See [troubleshooting.md](troubleshooting.md) for PSY-1184 release dedup requirement on re-runs.
