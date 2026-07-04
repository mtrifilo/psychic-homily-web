# Ingest troubleshooting & gotchas

- **`event_date` is stored as a timestamp, not a bare date.** `YYYY-MM-DD` normalizes to **20:00 venue-local → UTC** (PSY-985/986). `2026-07-17` at a CA venue → `2026-07-18T03:00:00Z`. Expected — don't "correct" it.

- **`422 SHOW_CREATE_FAILED` on re-submit usually means duplicate.** Backend enforces unique `(artist, venue, event_date)`. Verify existence before assuming failure.

- **Don't verify shows via artist search count.** Use date window or per-artist endpoint:
  ```bash
  curl -s "$URL/shows?from_date=2026-07-18T00:00:00Z&to_date=2026-07-18T23:59:59Z" -H "Authorization: Bearer $TOKEN"
  curl -s "$URL/artists/<id>/shows?time_filter=upcoming&limit=50" -H "Authorization: Bearer $TOKEN"
  ```

- **`search show "<query>"` matches city only** — unreliable for existence checks.

- **Venue/artist show lists:** `GET /venues/{id}/shows` and `GET /artists/{id}/shows` return `total`; `limit` caps at **200**. Over-cap → **HTTP 422**, not truncation. Check HTTP status; naive `curl | node` on 422 reads empty `shows`.

- **Festival-named tour stops are festivals, not venues.** Mosswood Meltdown / Desert Fox Festival → `festival` entity. Pre-party at a real venue → separate titled `show`.

- **Label `twitter` is host-validated** — only `twitter.com` / `x.com`. Bluesky (`bsky.app`) **422s** on `twitter`; omit it.

- **Verify artist links via `GET /artists/{id}` detail** — roster/list projections omit `social`/`bandcamp`.

- **Release re-runs are NOT idempotent until PSY-1184 is deployed** — confirm PR #1210 is live before re-running release batches on large datasets.
