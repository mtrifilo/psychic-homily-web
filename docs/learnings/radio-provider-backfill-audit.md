# Radio Provider Historical Backfill Audit

> Research for PSY-274. Conducted 2026-04-06 via live API testing.

## Executive Summary

All three radio providers have substantial historical data available, but with very different access patterns and limitations. KEXP offers the richest, most structured data (25+ years, MusicBrainz IDs, ~2.6M+ plays) but has a pagination offset ceiling that requires time-based chunking. WFMU has 25+ years of HTML playlists (~162,000+ episodes across ~568 shows) but no API -- everything requires HTML scraping. NTS has a clean REST API with ~70,000 estimated episodes across 1,704 shows going back to 2016, but tracklists require a separate `/tracklist` endpoint not currently used by the provider, and many episodes (DJ mixes) have no tracklist at all.

---

## 1. KEXP (`api.kexp.org`)

### Data Range

- **Earliest play**: 2000-12-31 (airbreak), earliest trackplay in early 2001
- **Data range**: ~25.5 years (Jan 2001 to present)
- **API status**: Fully operational, public, no auth required

### Volume Estimates

| Entity    | Count    | Notes                                    |
|-----------|----------|------------------------------------------|
| Programs  | 41       | Distinct show series (The Morning Show, etc.) |
| Shows     | 66,270   | Individual broadcast episodes (4-7 per day) |
| Plays     | ~3.5M+   | Offset pagination stops at ~2.65M (mid-2020); remaining reachable via time-range queries |

Shows per year: ~2,400 (2001) to ~3,200 (2025), averaging ~2,600/year.

### API Endpoints Tested

| Endpoint | Works? | Pagination | Count Header? |
|----------|--------|------------|---------------|
| `GET /v2/programs/` | Yes | cursor (next URL), limit=100 | Yes (`count` field) |
| `GET /v2/shows/` | Yes | cursor (next URL), limit=100 | Yes (`count` field) |
| `GET /v2/plays/` | Yes | cursor (next URL), limit=100 | No count field |
| `GET /v2/hosts/` | Yes | cursor (next URL), limit=100 | Yes |

### Rate Limits

- No rate-limit headers observed (X-RateLimit-*, Retry-After, etc.)
- Served via Cloudflare (cf-cache-status: DYNAMIC)
- Provider currently self-throttles at 1 req/sec -- this is conservative and likely sufficient
- No evidence of 429 responses during testing

### Pagination Behavior

- Uses offset-based pagination (`?offset=N&limit=100`)
- **Hard offset ceiling**: Offset pagination stops returning results around offset ~2,649,000 (corresponding to ~Jan 2020 data)
- Data beyond that offset exists (2020-2026) and is reachable via time-range filters
- The `next` field in responses always provides the correct next URL

### Key Filter Parameters

- **Shows**: `start_time_after`, `start_time_before`, `program_id`, `ordering` (supports `start_time` and `-start_time`)
- **Plays**: `airdate_after`, `airdate_before`, `play_type` (e.g., `trackplay`), `ordering` (supports `airdate` and `-airdate`)
- **Plays do NOT support `show_id` or `show` filtering** -- these parameters are silently ignored. The existing provider code at line 153 (`show_id=%s`) does not actually filter by show.

### Data Quality

- Excellent. MusicBrainz artist IDs, recording IDs, and release IDs present on many plays (especially recent ones)
- Structured metadata: album, label, release_date, rotation_status, is_live, is_request, is_new
- Play types: `trackplay`, `airbreak`, `stationid`, etc. -- filter with `play_type=trackplay` for music
- Shows endpoint provides: program_id, program_name, host_names, start_time, image_uri
- No end_time on shows -- must infer from next show's start_time

### Bugs Found in Current Provider

1. **`FetchPlaylist` uses `show_id=` which is ignored by the API.** Plays are not actually filtered by show. This means every `FetchPlaylist` call returns the same global plays list. This should be fixed to use `airdate_after`/`airdate_before` based on the show's time window.

### Backfill Strategy

**Recommended: Time-based chunking (not offset pagination)**

1. Fetch all 66,270 shows via `/v2/shows/?ordering=start_time&limit=100` (663 API calls, ~11 minutes at 1 req/sec)
2. For each show, determine time window (start_time to next show's start_time)
3. Fetch plays within that window via `/v2/plays/?airdate_after=X&airdate_before=Y&limit=100&ordering=airdate`
4. Average ~40-50 trackplays per show = 1-2 pages per show

**Estimated API calls**: 66,270 shows x 2 pages avg = ~133,000 calls for plays + 663 for shows = ~134,000 total
**Estimated wall-clock time at 1 req/sec**: ~37 hours

**Alternative**: Paginate all plays chronologically in daily/weekly chunks using `airdate_after`/`airdate_before`. This avoids the offset limit and requires fewer API calls (~35,000 pages at 100/page for ~3.5M plays = ~10 hours).

---

## 2. WFMU (`wfmu.org`)

### Data Range

- **Earliest accessible playlist**: ID 50, dated March 3, 2000
- **Latest accessible playlist**: ID ~162,800, dated April 7, 2026
- **Data range**: ~26 years (March 2000 to present)
- **No API** -- all data is via HTML scraping

### Volume Estimates

| Entity     | Count     | Notes                                        |
|------------|-----------|----------------------------------------------|
| Shows      | ~568      | Unique show codes on /playlists/ index page  |
| Episodes   | ~162,000+ | Sequential IDs from ~50 to ~162,800 (with gaps) |
| Tracks/ep  | ~40-60    | Typical playlist has 40-60 tracks in table rows |

Playlist ID to date mapping (sampled):
- ID 50: March 2000
- ID 1,000: August 2001
- ID 5,000: September 2002
- ID 10,000: January 2004
- ID 50,000: ~2012 (estimated)
- ID 100,000: January 2021
- ID 150,000: March 2025
- ID 160,000: January 2026
- ID 162,800: April 2026

### Data Sources

| Source | Coverage | Notes |
|--------|----------|-------|
| RSS feeds (`/playlistfeed/{CODE}.xml`) | Last 10 episodes only | Not useful for backfill |
| Show archive page (`/playlists/{CODE}`) | ALL episodes for a show | Lists all episode IDs with links |
| Playlist page (`/playlists/shows/{ID}`) | Full track listing | HTML table with artist, track, album, label, year, format, comments |

### Episode Discovery

The **show archive page** (`/playlists/{CODE}`) is the key resource. For example, Brian Turner's show (`/playlists/BT`) lists 824 episodes spanning from February 2001 to present, with direct links to each playlist page. This is far more reliable than the RSS feed (only 10 items).

### Rate Limits

- No rate-limit headers observed
- Served via Cloudflare
- Provider self-throttles at 1 req/sec
- All tested pages (including very old IDs) returned 200 OK promptly

### Data Quality

- Structured HTML tables with columns: Artist, Track, Album, Label, Year, Format, Comments, Images, New, Start Time
- Quality varies by DJ -- some DJs meticulously fill all columns, others only provide artist/track
- Very old playlists (pre-2005) may have simpler HTML structures
- No MusicBrainz IDs or external identifiers
- Comments field sometimes contains DJ notes and context

### Backfill Strategy

**Recommended: Two-phase HTML scraping**

**Phase 1 -- Episode Discovery** (~568 requests, ~10 minutes):
1. Parse `/playlists/` index page to get all show codes
2. For each show code, fetch `/playlists/{CODE}` to get all episode IDs
3. Build a complete episode inventory with IDs and dates

**Phase 2 -- Playlist Scraping** (~162,000 requests, ~45 hours at 1 req/sec):
1. For each episode ID, fetch `/playlists/shows/{ID}`
2. Parse the HTML table to extract track data
3. The existing `parseWFMUPlaylistPage()` function handles this

**Estimated wall-clock time**: ~45 hours at 1 req/sec (episode inventory is negligible)

### Provider Interface Impact

The current `FetchNewEpisodes(showExternalID, since)` uses RSS feeds which only return 10 episodes. For backfill, the provider needs a new method or the existing method needs to fall back to parsing the show archive page when `since` is far in the past.

---

## 3. NTS Radio (`nts.live/api`)

### Data Range

- **Earliest episode found**: November 2016 (astral-plane show)
- **NTS launched**: 2011, but API data appears to start around 2016-2017
- **Data range**: ~10 years of API-accessible data

### Volume Estimates

| Entity     | Count     | Notes                                        |
|------------|-----------|----------------------------------------------|
| Shows      | 1,704     | Via metadata.resultset.count                 |
| Episodes   | ~70,000   | Estimated from sampling (avg ~41 episodes/show) |
| Tracks/ep  | 0-30      | Many episodes have 0 (DJ mixes); music shows average ~20 |

### API Endpoints Tested

| Endpoint | Works? | Pagination | Count? |
|----------|--------|------------|--------|
| `GET /v2/shows` | Yes | offset/limit | Yes (metadata.resultset.count) |
| `GET /v2/shows/{alias}/episodes` | Yes | offset/limit, max 12 per page | Yes (count in metadata) |
| `GET /v2/shows/{alias}/episodes/{ep_alias}` | Yes | N/A | N/A |
| `GET /v2/shows/{alias}/episodes/{ep_alias}/tracklist` | Yes | N/A | Yes (metadata.resultset.count) |

### Critical Finding: Separate Tracklist Endpoint

The episode detail endpoint (`/v2/shows/{alias}/episodes/{ep_alias}`) does NOT include tracklists. Tracklists are served from a **separate endpoint**: `/v2/shows/{alias}/episodes/{ep_alias}/tracklist`. The current NTS provider code fetches the episode detail and reads `detail.Tracklist`, which will always be empty. **The provider needs to be updated to use the `/tracklist` sub-endpoint.**

The tracklist response includes rich data:
```json
{
  "artist": "Keppel",
  "title": "Thursday Morning",
  "uid": "0de0ee4e-d6e2-481f-9854-9f28be1f7697",
  "offset": 11,
  "duration": 189,
  "offset_estimate": null,
  "duration_estimate": null
}
```

The `offset` and `duration` fields (in seconds) are valuable -- they indicate exact playback position within the episode audio.

### Pagination Behavior

- Shows endpoint: offset/limit works correctly, respects requested limit
- Episodes endpoint: **max page size is 12**, regardless of requested limit. The provider code sets `ntsPageLimit = 100` but only gets 12 results per page. Offset pagination works correctly.
- Episode ordering: newest first (descending broadcast date)

### Rate Limits

- No rate-limit headers observed
- No evidence of throttling during testing

### Tracklist Coverage

Tracklists are highly variable across shows. DJ mix shows (the majority of NTS content) typically have **zero tracklists** via the API. Music curation shows that play individual tracks tend to have tracklists. Based on sampling, estimated tracklist coverage is **30-50% of episodes** (varies significantly by show type).

Historical coverage tested:
- 2016 episode (astral-plane): 23 tracks -- tracklists available from earliest data
- 2017 episode (astral-plane): 28 tracks
- 2026 episode (foodman): 20 tracks

### Data Quality

- Artist and title only (no album, label, or year in tracklist endpoint)
- No MusicBrainz IDs
- Rich show metadata: genres, moods, location, host info, Mixcloud links
- Episodes have broadcast timestamps and duration

### Backfill Strategy

**Recommended: Three-phase approach**

**Phase 1 -- Show Discovery** (~17 API calls at 100/page):
1. Paginate `/v2/shows` to get all 1,704 show aliases

**Phase 2 -- Episode Discovery** (~5,800+ API calls):
1. For each show, paginate `/v2/shows/{alias}/episodes` at 12 per page
2. Average ~41 episodes/show / 12 per page = ~3.4 pages per show
3. 1,704 shows x 3.4 pages = ~5,800 API calls

**Phase 3 -- Tracklist Fetch** (~70,000 API calls):
1. For each episode, fetch `/v2/shows/{alias}/episodes/{ep_alias}/tracklist`
2. Many will return empty (DJ mixes), but the call is needed to discover which ones have data

**Total estimated API calls**: ~76,000
**Estimated wall-clock time at 1 req/sec**: ~21 hours

---

## 4. Provider Interface Recommendations

### Current Interface

```go
type RadioPlaylistProvider interface {
    DiscoverShows() ([]RadioShowImport, error)
    FetchNewEpisodes(showExternalID string, since time.Time) ([]RadioEpisodeImport, error)
    FetchPlaylist(episodeExternalID string) ([]RadioPlayImport, error)
}
```

### Recommended Changes

#### 4.1 Add `until` Parameter to `FetchNewEpisodes`

The current `since`-only parameter is insufficient for bounded backfill. All three providers return episodes in **newest-first** order (NTS, WFMU) or support ordering (KEXP). Adding an `until` parameter enables chunked backfill:

```go
FetchNewEpisodes(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error)
```

For KEXP, this maps directly to `start_time_after` / `start_time_before`. For NTS, it bounds the pagination loop. For WFMU, it filters the episode list from the archive page.

#### 4.2 Add `FetchAllEpisodes` for Backfill

For WFMU, the RSS feed only returns 10 episodes. Backfill requires parsing the show archive page. A separate method avoids complicating the incremental-fetch path:

```go
FetchAllEpisodes(showExternalID string) ([]RadioEpisodeImport, error)
```

#### 4.3 Fix NTS `FetchPlaylist` to Use `/tracklist` Endpoint

The current implementation reads `detail.Tracklist` from the episode detail endpoint, which is always empty. It must be changed to call `/v2/shows/{alias}/episodes/{ep_alias}/tracklist`.

#### 4.4 Fix KEXP `FetchPlaylist` Show Filtering

The current implementation uses `show_id=` parameter which is silently ignored by the KEXP API. It should use `airdate_after`/`airdate_before` based on the show's time window, or alternatively just paginate all plays by time range for the backfill.

### Episode Ordering by Provider

| Provider | Default Order | Configurable? | Implications |
|----------|--------------|---------------|--------------|
| KEXP     | Unspecified  | Yes (`ordering=start_time` or `-start_time`) | Use ascending for backfill |
| WFMU     | Newest first (RSS), All listed (archive page) | No (RSS) / N/A (archive) | Archive page is unordered; sort by ID |
| NTS      | Newest first | No | Must paginate to end for oldest episodes |

---

## 5. Estimated Wall-Clock Time for Full Historical Backfill

| Provider | API Calls | Time @ 1 req/sec | Time @ 2 req/sec | Notes |
|----------|-----------|-------------------|-------------------|-------|
| KEXP     | ~35,000-134,000 | 10-37 hours | 5-19 hours | Range depends on strategy (bulk plays vs per-show) |
| WFMU     | ~163,000 | ~45 hours | ~23 hours | Dominated by individual playlist page fetches |
| NTS      | ~76,000  | ~21 hours | ~11 hours | Many tracklist calls will return empty |
| **Total**| ~274,000-373,000 | **76-103 hours** | **39-53 hours** | ~3-4 days continuous at 1 req/sec |

### Parallelism Opportunity

Since the three providers are completely independent, all three can run simultaneously. With 1 req/sec per provider, the wall-clock time is limited by the slowest provider (WFMU at ~45 hours).

---

## 6. Risks and Blockers

### High Risk

1. **NTS tracklist endpoint not used by current provider** -- the `FetchPlaylist` method returns 0 tracks for every episode. Must be fixed before any backfill or even ongoing fetch is meaningful.

2. **KEXP play filtering is broken** -- `FetchPlaylist` does not filter by show. Every call returns the same global plays list. Must be fixed.

3. **WFMU RSS limited to 10 episodes** -- `FetchNewEpisodes` can only discover the most recent 10 episodes per show. Historical backfill requires a fundamentally different approach (archive page parsing).

### Medium Risk

4. **KEXP offset pagination ceiling (~2.65M)** -- can't reach data after mid-2020 via pure offset pagination. Time-based chunking works around this.

5. **NTS episodes max page size is 12** -- provider code assumes 100 per page. Not a blocker, but increases API calls by ~8x vs expected.

6. **WFMU HTML structure changes over time** -- very old playlists (pre-2005) may have different HTML structures than the current parser handles.

7. **No rate limit headers from any provider** -- we're self-throttling at 1 req/sec as a courtesy, but have no feedback loop if we're being throttled or approaching a limit.

### Low Risk

8. **NTS tracklist coverage is sparse** -- many episodes are DJ mixes with no tracklist. This is inherent to the content type, not a technical limitation. Estimated 30-50% coverage.

9. **WFMU data quality varies by DJ** -- some shows have meticulously tagged playlists, others have minimal data. This is expected and acceptable.

---

## 7. Recommended Backfill Strategy (Overall)

### Phase 1: Fix Critical Provider Bugs (prerequisite)

Before any backfill work:
1. Fix NTS `FetchPlaylist` to use the `/tracklist` sub-endpoint
2. Fix KEXP `FetchPlaylist` to use time-range filtering instead of `show_id`
3. Update NTS pagination to handle 12-per-page reality
4. Add WFMU archive page parsing as alternative to RSS for episode discovery

### Phase 2: Backfill Infrastructure

1. Add `until` parameter to `FetchNewEpisodes` interface (or add `FetchAllEpisodes`)
2. Build a backfill orchestrator that:
   - Processes one provider at a time (or all three in parallel)
   - Tracks progress (last-processed show/episode) for resumability
   - Respects rate limits (1 req/sec per provider)
   - Handles errors gracefully (retry with backoff, skip and continue)
   - Reports progress (Discord notifications, admin dashboard)

### Phase 3: Execute Backfill (per provider)

**Order recommendation**: KEXP first (richest data, best structured), then NTS (clean API), then WFMU (most scraping work).

| Step | KEXP | NTS | WFMU |
|------|------|-----|------|
| 1. Discover shows | 663 calls | 17 calls | 568 calls |
| 2. Discover episodes | Included in shows | 5,800 calls | 568 calls (archive pages) |
| 3. Fetch playlists | 35,000-134,000 calls | 70,000 calls | 162,000 calls |
| 4. Match artists | Partially pre-matched (MusicBrainz IDs) | Name matching only | Name matching only |

### Phase 4: Ongoing Incremental Fetch

After backfill, the existing `since`-based incremental fetch continues to work for new data. The backfill infrastructure can be repurposed for periodic catch-up if gaps are detected.
