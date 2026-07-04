# Artist tag rollup (after release-pass)

The release-pass applies tags to **release** entities only. This pass backfills **artist** genre/locale tags from the same Bandcamp keywords without re-fetching.

**Prerequisite:** [release-pass.md](release-pass.md) completed; raw cache at `/tmp/releases-raw.json` (or equivalent).

## Algorithm

1. **Aggregate** from raw cache — count genre/locale tag frequency per `artist_name` across that artist's releases.
2. **Pick top tags:** up to **6 genres** on ≥15% of an artist's releases; **1 locale** if a city/region on ≥30% (same allowlist as [tag-allowlist.md](tag-allowlist.md)).
3. **Apply** via `POST /entities/artist/{id}/tags` with `{tag_name: "…"}` (409 = already tagged). Skip artists that already have artist tags.
4. **Verify** via `GET /entities/artist/{id}/tags` — not the roster list projection.

## Proven runs

- **Dais Records** (2026-07-03): **76/89** artists tagged; 13 had no release-pass data.
