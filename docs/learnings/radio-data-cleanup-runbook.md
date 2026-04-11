# Radio Data Cleanup Runbook (PSY-276 / PSY-277)

> After fixing NTS and KEXP provider bugs, existing data in the database is
> invalid and needs to be cleaned up before re-importing.

## Background

Two critical bugs were found and fixed:

- **PSY-276 (NTS):** `FetchPlaylist` was reading the episode detail endpoint,
  which never includes tracklist data. All NTS episodes were imported with
  `play_count = 0` and zero `radio_plays` rows. Fixed to use the
  `/v2/shows/{alias}/episodes/{ep_alias}/tracklist` sub-endpoint.

- **PSY-277 (KEXP):** `FetchPlaylist` passed `show_id=` to the KEXP plays
  API, but that parameter is silently ignored. Every FetchPlaylist call
  returned the **same global plays list** instead of plays scoped to the
  episode's broadcast window. Fixed to use `airdate_after`/`airdate_before`
  based on the episode's start_time.

**WFMU data is unaffected** -- its `FetchPlaylist` (HTML scraping) was never
broken.

---

## 1. Assess the Damage

Run these queries to understand the scope of bad data. Connect to the
production database (read-only is fine for assessment).

### 1a. NTS episodes with zero plays

All NTS episodes should have `play_count = 0` since the provider was reading
the wrong endpoint. Many NTS episodes are DJ mixes that legitimately have no
tracklist, but the point is that *none* of them were ever fetched correctly.

```sql
-- Total NTS episodes
SELECT COUNT(*)
FROM radio_episodes e
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'nts_api';

-- NTS episodes with zero play_count (should be all of them)
SELECT COUNT(*)
FROM radio_episodes e
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'nts_api'
  AND e.play_count = 0;

-- Confirm NTS has zero radio_plays rows
SELECT COUNT(*)
FROM radio_plays rp
JOIN radio_episodes e ON rp.episode_id = e.id
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'nts_api';
```

### 1b. KEXP plays (all potentially wrong)

Every KEXP play may be assigned to the wrong episode because the provider was
not filtering by show. Plays may be duplicated across episodes or misattributed.

```sql
-- Total KEXP episodes
SELECT COUNT(*)
FROM radio_episodes e
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'kexp_api';

-- Total KEXP plays (all potentially incorrect)
SELECT COUNT(*)
FROM radio_plays rp
JOIN radio_episodes e ON rp.episode_id = e.id
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'kexp_api';

-- KEXP episodes with non-zero play_count (these have bad data)
SELECT COUNT(*)
FROM radio_episodes e
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'kexp_api'
  AND e.play_count > 0;
```

### 1c. WFMU plays (should be fine)

Verify WFMU data is untouched. Record these numbers before cleanup as a
baseline.

```sql
-- Total WFMU episodes
SELECT COUNT(*)
FROM radio_episodes e
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'wfmu_scrape';

-- Total WFMU plays
SELECT COUNT(*)
FROM radio_plays rp
JOIN radio_episodes e ON rp.episode_id = e.id
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'wfmu_scrape';
```

### 1d. Artist affinity impact

Artist affinity data computed from bad KEXP plays is also tainted. NTS had no
plays, so it didn't contribute to affinity. Check how much affinity data exists.

```sql
-- Total affinity rows
SELECT COUNT(*) FROM radio_artist_affinity;
```

### 1e. Import job history

Check for completed import jobs (these imported the bad data).

```sql
-- All import jobs grouped by status
SELECT station_id, status, COUNT(*), SUM(plays_imported), SUM(episodes_imported)
FROM radio_import_jobs
GROUP BY station_id, status
ORDER BY station_id, status;
```

---

## 2. Cleanup Strategy

### Option A (Recommended): Delete plays and episodes, re-import via jobs

This is the cleaner approach. The import job system (`runImportJob`) calls
`importEpisode`, which **skips episodes that already exist** (dedup by
`show_id + external_id`). Therefore, to re-import plays for existing episodes,
we must delete the episodes too -- otherwise the import job will skip them all
and import nothing.

Radio shows and stations are fine and do not need to be deleted.

**Order of operations:**

```sql
-- Step 1: Record WFMU baseline counts (for verification later)
-- Run the 1c queries above and save the numbers.

-- Step 2: Delete all KEXP radio_plays
-- CASCADE from radio_episodes handles this, but we delete plays explicitly
-- first so we can track the count.
DELETE FROM radio_plays
WHERE episode_id IN (
    SELECT e.id FROM radio_episodes e
    JOIN radio_shows s ON e.show_id = s.id
    JOIN radio_stations st ON s.station_id = st.id
    WHERE st.playlist_source = 'kexp_api'
);

-- Step 3: Delete all NTS radio_plays (should be 0, but be thorough)
DELETE FROM radio_plays
WHERE episode_id IN (
    SELECT e.id FROM radio_episodes e
    JOIN radio_shows s ON e.show_id = s.id
    JOIN radio_stations st ON s.station_id = st.id
    WHERE st.playlist_source = 'nts_api'
);

-- Step 4: Delete all KEXP episodes
DELETE FROM radio_episodes
WHERE show_id IN (
    SELECT s.id FROM radio_shows s
    JOIN radio_stations st ON s.station_id = st.id
    WHERE st.playlist_source = 'kexp_api'
);

-- Step 5: Delete all NTS episodes
DELETE FROM radio_episodes
WHERE show_id IN (
    SELECT s.id FROM radio_shows s
    JOIN radio_stations st ON s.station_id = st.id
    WHERE st.playlist_source = 'nts_api'
);

-- Step 6: Clear tainted artist affinity data
-- (Will be recomputed from correct plays after re-import)
TRUNCATE radio_artist_affinity;

-- Step 7: Clear old import job history (optional, but avoids confusion)
DELETE FROM radio_import_jobs
WHERE station_id IN (
    SELECT id FROM radio_stations
    WHERE playlist_source IN ('kexp_api', 'nts_api')
);

-- Step 8: Reset last_playlist_fetch_at so incremental fetch starts fresh
UPDATE radio_stations
SET last_playlist_fetch_at = NULL
WHERE playlist_source IN ('kexp_api', 'nts_api');
```

### Option B: Nuclear -- delete everything and start fresh

Delete all radio_plays, radio_episodes, and radio_shows for NTS and KEXP, then
run a full `ImportStation` to rediscover shows and episodes from scratch.
Heavier but guarantees no stale references.

**Order of operations:**

```sql
-- Step 1: Record WFMU baseline counts (same as Option A)

-- Step 2: Delete plays for KEXP + NTS
DELETE FROM radio_plays
WHERE episode_id IN (
    SELECT e.id FROM radio_episodes e
    JOIN radio_shows s ON e.show_id = s.id
    JOIN radio_stations st ON s.station_id = st.id
    WHERE st.playlist_source IN ('kexp_api', 'nts_api')
);

-- Step 3: Delete episodes for KEXP + NTS
DELETE FROM radio_episodes
WHERE show_id IN (
    SELECT s.id FROM radio_shows s
    JOIN radio_stations st ON s.station_id = st.id
    WHERE st.playlist_source IN ('kexp_api', 'nts_api')
);

-- Step 4: Delete shows for KEXP + NTS
DELETE FROM radio_shows
WHERE station_id IN (
    SELECT id FROM radio_stations
    WHERE playlist_source IN ('kexp_api', 'nts_api')
);

-- Step 5: Clear affinity data
TRUNCATE radio_artist_affinity;

-- Step 6: Clear import job history
DELETE FROM radio_import_jobs
WHERE station_id IN (
    SELECT id FROM radio_stations
    WHERE playlist_source IN ('kexp_api', 'nts_api')
);

-- Step 7: Reset last_playlist_fetch_at
UPDATE radio_stations
SET last_playlist_fetch_at = NULL
WHERE playlist_source IN ('kexp_api', 'nts_api');
```

After cleanup, you would use the "Discover Shows" admin button to re-create
the show catalog, then use import jobs for episodes. Option B is heavier
(re-discovers ~1,700+ NTS shows and ~41 KEXP programs) but results in a
completely clean slate.

---

## 3. Re-Import Steps

After running the cleanup SQL (Option A or B):

### 3a. Ensure show catalog is current

**If you chose Option A** (shows preserved), skip this step -- shows are
already in the database.

**If you chose Option B** (shows deleted), re-discover shows first:

1. Go to the admin Radio Stations management page.
2. For each station (KEXP, NTS), click the "Discover Shows" button.
3. Or via curl:

```bash
# Get station IDs
curl -s https://api.psychichomily.com/api/radio-stations | jq '.[] | {id, name, playlist_source}'

# Discover shows for KEXP (replace STATION_ID)
curl -X POST "https://api.psychichomily.com/api/admin/radio-stations/STATION_ID/discover" \
  -H "Cookie: auth_token=YOUR_TOKEN"

# Discover shows for NTS (replace STATION_ID)
curl -X POST "https://api.psychichomily.com/api/admin/radio-stations/STATION_ID/discover" \
  -H "Cookie: auth_token=YOUR_TOKEN"
```

### 3b. Create import jobs for each show

The async import job system (`POST /admin/radio-shows/{id}/import-job`)
creates a background job that fetches episodes and their playlists within a
date range. The job runs in a goroutine, tracks progress, and supports
cancellation.

**Important:** The import job system calls `importEpisode` internally, which
creates episodes AND fetches their playlists in a single pass. Since we
deleted the episodes in cleanup, the import job will recreate them with correct
playlist data from the fixed providers.

**Strategy:** Create one import job per show, covering the date range of data
that was previously imported. If you only had recent data (not a full
historical backfill), scope the `since`/`until` dates accordingly.

**Via admin UI:**
1. Navigate to the Radio Shows admin page.
2. For each show, click "Import" and set the date range.
3. The job will appear in the import jobs list with progress updates.

**Via curl (programmatic):**

```bash
# List all shows for a station
curl -s "https://api.psychichomily.com/api/radio-shows?station_id=STATION_ID" | \
  jq '.[] | {id, name}'

# Create an import job for a specific show
# Adjust since/until to match the range of data you need
curl -X POST "https://api.psychichomily.com/api/admin/radio-shows/SHOW_ID/import-job" \
  -H "Cookie: auth_token=YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"since": "2024-01-01", "until": "2026-04-06"}'

# Check job status
curl -s "https://api.psychichomily.com/api/admin/radio/import-jobs/JOB_ID" \
  -H "Cookie: auth_token=YOUR_TOKEN" | jq .
```

**Batch script to create import jobs for all shows of a station:**

```bash
#!/bin/bash
# Usage: ./reimport-station.sh STATION_ID SINCE UNTIL AUTH_TOKEN
STATION_ID=$1
SINCE=$2
UNTIL=$3
TOKEN=$4
API="https://api.psychichomily.com/api"

# Get all show IDs for the station
SHOW_IDS=$(curl -s "$API/radio-shows?station_id=$STATION_ID" \
  -H "Cookie: auth_token=$TOKEN" | jq -r '.[].id')

for SHOW_ID in $SHOW_IDS; do
  echo "Creating import job for show $SHOW_ID..."
  curl -s -X POST "$API/admin/radio-shows/$SHOW_ID/import-job" \
    -H "Cookie: auth_token=$TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"since\": \"$SINCE\", \"until\": \"$UNTIL\"}" | jq '{id: .id, status: .status}'

  # Note: The import job system prevents creating a job if one is already
  # pending/running for the same show. Jobs run sequentially in goroutines.
  # Wait a moment between creations so the server isn't overwhelmed.
  sleep 2
done
```

**Caveat:** Each import job runs in its own goroutine. Creating hundreds of
jobs simultaneously will spawn hundreds of goroutines. The jobs themselves are
rate-limited by the provider (1 req/sec), but the goroutine count could be a
concern. Consider creating jobs in batches (10-20 at a time) and waiting for
them to complete before creating more. Monitor via:

```bash
# List all active (running + pending) jobs
curl -s "https://api.psychichomily.com/api/admin/radio/import-jobs/active" \
  -H "Cookie: auth_token=$TOKEN" | jq 'length'
```

### 3c. Priority order

1. **KEXP first** -- richest data (MusicBrainz IDs, full metadata), fewer
   shows (41 programs), most impactful for artist matching.
2. **NTS second** -- clean API, but many episodes have no tracklist (DJ mixes).
   1,704 shows means many more import jobs.

---

## 4. Verification

Run these queries after re-import jobs complete to verify correctness.

### 4a. NTS episodes now have plays where tracklists exist

```sql
-- NTS episodes with non-zero play_count
-- (Should be 30-50% of total, since many NTS episodes are DJ mixes)
SELECT
    COUNT(*) FILTER (WHERE e.play_count > 0) AS episodes_with_plays,
    COUNT(*) FILTER (WHERE e.play_count = 0) AS episodes_without_plays,
    COUNT(*) AS total_episodes,
    ROUND(100.0 * COUNT(*) FILTER (WHERE e.play_count > 0) / NULLIF(COUNT(*), 0), 1) AS pct_with_plays
FROM radio_episodes e
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'nts_api';

-- Total NTS radio_plays (should be > 0 now)
SELECT COUNT(*)
FROM radio_plays rp
JOIN radio_episodes e ON rp.episode_id = e.id
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'nts_api';
```

### 4b. KEXP plays are correctly scoped to episodes

The key verification: each episode's plays should have `air_timestamp` values
within the episode's broadcast window (start_time to start_time + 5 hours).
Plays should NOT be duplicated across episodes.

```sql
-- Check for duplicate plays across KEXP episodes
-- Each play row should belong to exactly one episode. If the old bug was
-- present, the same track (same air_timestamp) might appear in multiple
-- episodes.
SELECT rp.air_timestamp, COUNT(DISTINCT rp.episode_id) AS episode_count
FROM radio_plays rp
JOIN radio_episodes e ON rp.episode_id = e.id
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'kexp_api'
  AND rp.air_timestamp IS NOT NULL
GROUP BY rp.air_timestamp
HAVING COUNT(DISTINCT rp.episode_id) > 1
LIMIT 20;
-- Expected: 0 rows (no duplicates)

-- Spot-check: plays for a random KEXP episode should have air_timestamps
-- close to the episode's air_date/air_time
SELECT e.id, e.air_date, e.air_time, e.play_count,
       MIN(rp.air_timestamp) AS earliest_play,
       MAX(rp.air_timestamp) AS latest_play
FROM radio_episodes e
JOIN radio_plays rp ON rp.episode_id = e.id
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'kexp_api'
  AND e.play_count > 0
GROUP BY e.id, e.air_date, e.air_time, e.play_count
ORDER BY e.air_date DESC
LIMIT 10;
-- The earliest_play and latest_play should be within a few hours of
-- the episode's air_date + air_time.

-- KEXP play count sanity check
-- Average should be ~40-50 plays per episode (typical 3-hour show)
SELECT
    COUNT(*) AS total_episodes,
    SUM(e.play_count) AS total_plays,
    ROUND(AVG(e.play_count), 1) AS avg_plays_per_episode,
    MAX(e.play_count) AS max_plays
FROM radio_episodes e
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'kexp_api'
  AND e.play_count > 0;
```

### 4c. WFMU data is unchanged

Compare these counts to the baseline recorded before cleanup. They should be
identical.

```sql
-- WFMU episode count (should match pre-cleanup baseline)
SELECT COUNT(*)
FROM radio_episodes e
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'wfmu_scrape';

-- WFMU play count (should match pre-cleanup baseline)
SELECT COUNT(*)
FROM radio_plays rp
JOIN radio_episodes e ON rp.episode_id = e.id
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
WHERE st.playlist_source = 'wfmu_scrape';
```

### 4d. Artist matching is working

After re-import, the matching engine should have linked plays to known artists.

```sql
-- Matched vs unmatched plays by station
SELECT
    st.name AS station,
    COUNT(*) AS total_plays,
    COUNT(rp.artist_id) AS matched,
    ROUND(100.0 * COUNT(rp.artist_id) / NULLIF(COUNT(*), 0), 1) AS match_pct
FROM radio_plays rp
JOIN radio_episodes e ON rp.episode_id = e.id
JOIN radio_shows s ON e.show_id = s.id
JOIN radio_stations st ON s.station_id = st.id
GROUP BY st.name;
```

### 4e. Affinity data is being recomputed

Affinity is computed by the background `RadioFetchService` on a daily cycle.
After re-import, wait for the next affinity run (or trigger it manually if
there's an admin endpoint). Then check:

```sql
-- Affinity rows should be populated from correct data
SELECT COUNT(*) FROM radio_artist_affinity;
```

---

## 5. Time Estimates

Based on the PSY-274 backfill audit findings, adjusted for the fact that we
only need to re-import **currently imported data** (not a full 20-year backfill).

### What we're re-importing

The scope depends on how much data was imported before the bugs were found. If
only recent data (e.g., last 30-90 days), the re-import is fast. If a large
historical backfill was already run, it takes longer.

### Per-episode cost

Each episode requires:
1. `FetchNewEpisodes` call to discover episodes (paginated, batched)
2. `FetchPlaylist` call per episode (1 API request each)
3. Matching engine run per episode (local, fast)

The bottleneck is the per-episode `FetchPlaylist` call at 1 req/sec.

### KEXP estimates

- **~66K total episodes in KEXP's API** (from audit)
- Each import job fetches episodes for a single show within a date range
- 41 KEXP programs, average ~1,600 episodes per program
- At 1 req/sec: ~1,600 seconds (~27 min) per program
- **Full re-import of all 66K episodes: ~18-37 hours**
  - 18 hours if fetching plays in bulk by time range
  - 37 hours if per-episode playlist fetch (current approach)
- If only recent data (e.g., 1 year = ~3,200 episodes): **~1 hour**

### NTS estimates

- **~70K total episodes in NTS's API** (from audit)
- 1,704 NTS shows, average ~41 episodes per show
- Each episode needs a separate `/tracklist` API call
- At 1 req/sec: ~70,000 seconds = **~19 hours** for full backfill
- Many episodes return empty tracklists (DJ mixes) -- the API call is still
  needed to check
- If only recent data (e.g., 1 year): **~2-3 hours**

### WFMU (no re-import needed)

WFMU data is not affected. No time cost.

### Total

| Scenario | KEXP | NTS | Total |
|----------|------|-----|-------|
| Recent data only (last year) | ~1 hour | ~2-3 hours | ~3-4 hours |
| Moderate backfill (last 5 years) | ~8-15 hours | ~8-10 hours | ~16-25 hours |
| Full historical | ~18-37 hours | ~19 hours | ~37-56 hours |

KEXP and NTS import jobs run independently (separate goroutines, separate API
providers), so wall-clock time is the max of the two, not the sum. With both
running in parallel:

| Scenario | Wall-clock time |
|----------|-----------------|
| Recent data only | ~3 hours |
| Moderate backfill | ~15 hours |
| Full historical | ~37 hours |

### Monitoring during re-import

Watch the import job progress in the admin UI or via:

```bash
# Poll a specific job's progress
watch -n 30 'curl -s "https://api.psychichomily.com/api/admin/radio/import-jobs/JOB_ID" \
  -H "Cookie: auth_token=TOKEN" | jq "{status, episodes_found, episodes_imported, plays_imported, plays_matched, current_episode_date}"'
```
