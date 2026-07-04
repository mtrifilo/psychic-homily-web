# Radio playlist / historical show backfill

Import WFMU (and other provider) radio episodes + track plays into `radio_episodes` and `radio_plays` — **not** via `ph batch`. The backend has a dedicated WFMU provider + async sync-run pipeline.

## When to use

- Historic archive for a show already in `radio_shows` (discovered via station roster scrape)
- `/ingest stage — backfill WFMU playlists for show <slug>`
- Admin UI: **Admin → Radio → expand show → Backfill Episodes**

## Prerequisites

- Show exists with `external_id` = WFMU DJ code (e.g. Secret Canine Agents → `C3`)
- `archive_url` = `https://wfmu.org/playlists/{CODE}`
- Station `playlist_source` = `wfmu`

Verify on stage:

```bash
curl -s "$API/radio-shows/secret-canine-agents" | jq '{id, slug, external_id: .archive_url, episode_count}'
```

## WFMU archive structure

| Layer | URL | Notes |
| --- | --- | --- |
| Show archive index | `https://wfmu.org/playlists/{CODE}` | Single HTML page, **no pagination**; lists all episodes |
| Episode playlist | `https://wfmu.org/playlists/shows/{ID}` | Per-episode track list |

Parser (`parseWFMUArchivePage` in `radio_provider_wfmu.go`):

- Reads `.showlist` `<li>` rows with `See the playlist` → `/playlists/shows/{ID}`
- Skips placeholder rows (guest fill-ins, no playlist link)
- Skips pre-2009 legacy `Playlists/{ShowName}/xxx.html` links (not addressable by modern fetch)
- Rate limit: **1 req/sec** per provider instance
- `until` capped at today (WFMU-local) — future placeholder pages excluded

## Workflow (proven: Secret Canine Agents, 2026-07-04)

### 1. Baseline

```bash
# Before backfill: 17 episodes (incremental fetch only)
curl -s "$API/radio-shows/secret-canine-agents" | jq .episode_count
# WFMU archive link count:
curl -s -A "Mozilla/5.0" "https://wfmu.org/playlists/C3" | grep -o 'playlists/shows/[0-9]*' | sort -u | wc -l
# → 227
```

### 2. Trigger backfill (admin API)

```bash
curl -s -X POST "$API/admin/radio-shows/{show_id}/backfill" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"since":"2011-01-01","until":"2026-07-04"}'
# → RadioSyncRunResponse with run id, status: running
```

`since`/`until` filter by episode `air_date` (inclusive). Use a wide window for first full archive pull.

Poll status:

```bash
curl -s "$API/admin/radio/sync-runs/{run_id}" -H "Authorization: Bearer $TOKEN"
```

Watch: `status`, `episodes_imported`, `plays_imported`, `plays_matched`, `current_episode_date` (progress).

### 3. Results (Secret Canine Agents run #6427)

| Metric | Value |
| --- | --- |
| Episodes on WFMU | 227 |
| Episodes after backfill | 227 |
| Plays imported | 9,868 |
| Plays matched to KG artists | 889 (~9%) |
| Runtime | ~2 min |
| Status | `success` |

Show page: `https://stage.psychichomily.com/radio/wfmu/secret-canine-agents`

### 4. Verify

```bash
curl -s "$API/radio-shows/secret-canine-agents" | jq .episode_count
curl -s "$API/radio-shows/secret-canine-agents/episodes?limit=25&offset=200" | jq '.total, .episodes[-1].air_date'
curl -s "$API/radio-shows/secret-canine-agents/episodes/2026-07-02" | jq '.play_count, .plays[0:3]'
curl -s "$API/radio-shows/secret-canine-agents/top-artists?limit=10"
```

Episode list caps at `limit=100` per request. UI paginates at **25/page** (~10 pages for 227 episodes).

### 5. Post-backfill enrichment (optional)

Backfill imports **raw** `artist_name` on plays. KG linkage (`artist_id`/`artist_slug`) is sparse until artists exist **and** rematch runs.

After ingest creates artists (or after deploy of PSY-1347):

```bash
# Full rematch — links plays whose artist_name (or alias) now resolves
curl -s -X POST "$API/admin/radio/rematch" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{}'
```

`ph batch --confirm` calls the same endpoint automatically after roster/playlist screenshot ingests.

For name variants still unlinked, add artist aliases (triggers targeted rematch) — see [troubleshooting.md](troubleshooting.md#radio-playlist-linking).

DJ patter rows (e.g. `"Music behind DJ: Intro! Yeah! Party!!"`) import as plays — filter at display or matching layer, not ingest.

## Incremental vs backfill

| Mode | Trigger | Window |
| --- | --- | --- |
| **Scheduled fetch** | Station sync job | `since` = per-show `last_playlist_fetch_at` |
| **Manual backfill** | `POST …/backfill` or admin UI | Explicit `[since, until]` |

Re-running backfill over the same window is idempotent (episode dedup by `show_id` + `air_date` + `external_id`).

## Show registry

| Show | Slug | WFMU code | Archive URL | Stage show id | Notes |
| --- | --- | --- | --- | --- | --- |
| **Secret Canine Agents** | `secret-canine-agents` | `C3` | `https://wfmu.org/playlists/C3` | 2398 | Host: DJ Perro Caliente. Thu 00:00–03:00 ET. First full backfill 2026-07-04: 17→227 eps. Archive spans ~2022-07 → present (show is relatively new). |

## Open questions / known gaps

- **Stage UI dogfood blocked** by Vercel SSO login wall (2026-07-04) — API/backfill path works; browser QA needs auth bypass or local stack.
- **Show `description` null** on stage despite rich WFMU schedule blurb — may need discover/scrape enrichment.
- **Top-artists sidebar** mostly dead-ends (`artist_slug: null`) until play→artist matching improves.
- Pre-2009 legacy playlist HTML format skipped by parser (not relevant for SCA).

## Related

- Screenshot/single-playlist (one episode, manual): [screenshot-batch.md](screenshot-batch.md)
- Catalog refresh loop: [catalog-refresh.md](catalog-refresh.md) (radio uses sync runs, not `ph sources`)
- Backend: `radio_provider_wfmu.go`, `POST /admin/radio-shows/{id}/backfill`
