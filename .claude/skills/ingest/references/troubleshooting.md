# Ingest troubleshooting & gotchas

- **`event_date` is stored as a timestamp, not a bare date.** `YYYY-MM-DD` normalizes to **20:00 venue-local → UTC** (PSY-985/986). `2026-07-17` at a CA venue → `2026-07-18T03:00:00Z`. Expected — don't "correct" it.
  **Unless the batch states a `music_at`** (PSY-1947): a stated music time IS the show's start, so it anchors `event_date` in place of the 20:00 convention and the row reads `event_date == music_at`. Shows ingested before that change keep their 20:00 anchor; a re-ingest does not move them, and backfilling them to their stated music times is a separate pass that has not been built.
  The exception's exception: below **06:00 venue-local on a date-only listing** the source has not said which day the clock belongs to, so both times are refused and the 20:00 anchor stands.

- **`422 SHOW_CREATE_FAILED` on re-submit usually means duplicate.** Backend enforces unique `(artist, venue, event_date)`. Verify existence before assuming failure.
  Because `music_at` can move `event_date` off the 20:00 anchor, that uniqueness key is NOT stable across a change in the times a calendar publishes. The CLI's own day-window duplicate check is what actually guards a re-ingest: it resolves the venue-local day through the same reader and zone the writer uses, so it brackets the instant the writer is about to store even when `event_date` states its own zone.

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

- **SeeTickets calendars** (e.g. Walter Studios) — list page 1 is SSR; later pages come from `admin-ajax.php?action=get_seetickets_events` with the page-local `seetickets_ajax_obj.nonce`. Don't expect a useful `/wp-json/seetickets` events API. Re-read nonce each run (it rotates).


## Radio playlist linking

- **Orange ● on a playlist row** means `radio_plays.artist_id` is set — not merely that `/artists/{slug}` exists. Matching runs at import time; artists added later stay unlinked until rematch.
- **`batch --confirm`** runs chunked rematch via `ph radio rematch` after creates/updates (per artist name — avoids full-table gateway timeout). Artist/label/alias create also triggers async targeted rematch on the backend.
- **Exact normalized name + aliases** — punctuation variants need an alias (e.g. playlist `Worlds Worst` vs KG `World's Worst`):
  ```bash
  curl -s -X POST "$URL/admin/artists/{id}/aliases" \
    -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
    -d '{"alias":"Worlds Worst"}'
  ```
- **Collab strings** (`Astrid Sonne, Smerz`, `zzzahara, Winter`) — combined artist entity, alias, or collab matcher (PSY-1353). Skip DJ markers (`Music behind DJ: …`).
- **Chunked rematch** (ops / post-backfill) — paginates `/admin/radio/unmatched` and rematches per `artist_name` (stays under gateway timeout):
  ```bash
  cd cli && bun run src/entry.ts --env stage radio rematch
  bun run src/entry.ts --env stage radio rematch --show secret-canine-agents
  bun run src/entry.ts --env stage radio rematch --station 8 --dry-run
  ```
  Scoped single-name API still works: `POST /admin/radio/rematch` with `{"artist_name":"…"}`.
- **WFMU plays have no MusicBrainz artist IDs** — MBID matching (PSY-1354) helps KEXP etc., not WFMU.
