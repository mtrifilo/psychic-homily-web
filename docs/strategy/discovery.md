# Discovery Track

> Data ingestion pipeline for the Music Scene Index. Evolving from brittle Playwright venue scrapers toward an AI-first, multi-source pipeline that can scale to hundreds of venues worldwide with zero per-venue code.

## Current Status

**AI-first pipeline operational (March 2026).** Full end-to-end pipeline shipped: venue source config → tiered fetch (static HTTP / chromedp dynamic / chromedp screenshot) → ETag/hash change detection → AI extraction (Claude Haiku with `IsMusicEvent` classification) → non-music filtering → import with per-venue `auto_approve` control → batch admin review with rejection categories. Admin trigger via `POST /admin/pipeline/extract`.

**Shipped:** PSY-75 (venue source config + extraction runs tables), PSY-76 (calendar extraction prompt), PSY-77 (HTTP fetcher with change detection), PSY-78 (chromedp rendering), PSY-79 (pipeline orchestrator), PSY-80 (auto-approve wiring + non-music filtering), PSY-81 (batch review admin UI with rejection categories), PSY-83 (Huma fix).

**Remaining:** PSY-82 (rejection feedback loop — venue-level stats, extraction notes), PSY-34 (data provenance), PSY-36 (venue config admin UI).

**Key services:** `PipelineService` (`services/pipeline.go`) orchestrates extraction. `FetcherService` (`services/fetcher.go`) handles tiered rendering. `ExtractionService` (`services/extraction.go` + `extraction_calendar.go`) sends text/images to Claude Haiku. `DiscoveryService` (`services/discovery.go`) handles deduplication by `source_venue` + `source_event_id` and import with configurable initial status. `VenueSourceConfigService` (`services/venue_source_config.go`) manages per-venue configuration and extraction run history.

## Complete Pipeline Lifecycle

End-to-end flow for a venue, from discovery through ongoing extraction and adaptation. This is the narrative overview — detailed designs for each stage are in later sections.

### Stage 1: Venue Discovery & Onboarding

A venue enters the system through one of:
- **Admin adds manually** — enters venue name, address, and `calendar_url` via admin UI
- **Automated discovery** (Phase 4) — Google Maps API / OpenStreetMap query finds music venues in a city, admin reviews and approves
- **Festival bootstrapping** — a festival lineup references a venue we don't have yet, admin adds it during festival data entry
- **Community submission** (Phase 3) — a user submits a venue through the approval workflow

**On creation:** The venue gets a source config record with `calendar_url` and defaults. Everything else is auto-detected on the first extraction run.

### Stage 2: Initial Strategy Detection (First Run)

The pipeline processes the venue for the first time and figures out the best approach:

```
1. HTTP GET calendar_url
   ├─ 301 redirect? → update calendar_url to new location, continue
   ├─ 403/429? → skip static, try chromedp
   └─ 200? → continue

2. Check for structured feeds
   ├─ <link rel="alternate" type="text/calendar"> → save feed_url, set preferred_source = 'ical'
   ├─ <link rel="alternate" type="application/rss+xml"> → save feed_url, set preferred_source = 'ical'
   └─ <script type="application/ld+json"> with MusicEvent/Event → set preferred_source = 'jsonld'

3. If no structured source, determine render_method
   ├─ HTML body has event markers (dates, artist names, ticket links)? → render_method = 'static'
   ├─ HTML body is empty shell? → chromedp render → HTML has events? → render_method = 'dynamic'
   └─ chromedp HTML still empty? → chromedp screenshot → render_method = 'screenshot'

4. Run extraction with detected strategy
   └─ Record results in venue_extraction_runs
      └─ Set events_expected = events_extracted (initial baseline)
```

**Result:** Venue now has `preferred_source`, `render_method`, `feed_url` (if applicable), `events_expected`, and a first batch of extracted events.

### Stage 3: Data Processing & Import

After extraction, the raw events go through the import pipeline:

```
For each extracted event:
  1. Generate source_event_id (hash of venue + date + headliner name)
  2. Dedup check: does source_venue + source_event_id already exist?
     ├─ Yes → skip (or update if fields changed)
     └─ No → continue

  3. Artist matching
     ├─ Exact match (case-insensitive) → link to existing artist
     ├─ Fuzzy match candidates → attach as suggestions for admin review
     └─ No match → create as new artist (if auto_approve) or flag for review

  4. Venue matching (already known — this is a per-venue extraction)

  5. Bill position assignment
     ├─ AI confidence high on billing → use AI-determined set_type
     └─ Otherwise → first artist = headliner, rest = opener (heuristic)

  6. Show creation
     ├─ auto_approve = true → create as approved, visible to users immediately
     └─ auto_approve = false → create as pending_review, queued for admin
```

### Stage 4: Post-Import Enrichment (Async)

After import, an async enrichment pipeline runs (event-driven, rate-limited):

```
For each newly imported show:
  1. MusicBrainz lookup — match artists to MBIDs, pull metadata (1 req/s)
  2. Bandcamp enrichment — find artist Bandcamp pages, link releases
  3. API cross-referencing — query SeatGeek/Bandsintown to validate event
     ├─ API confirms event → boost source_confidence
     ├─ API has pricing/genre data → enrich show record
     └─ API doesn't know this event → no action (we're the primary source)
  4. AI billing enrichment — if bill order is ambiguous, send event data
     to Claude for headliner/support/opener determination
```

This runs independently of extraction — even manually-added shows benefit from enrichment.

### Stage 5: Ongoing Scheduled Runs

The pipeline service runs once daily (configurable, can increase frequency later as venue count grows):

```
Once per day (default schedule, configurable):
  Query: venues WHERE last_extracted_at + scrape_schedule < NOW()

  For each due venue (via worker pool, 5-10 concurrent):
    1. HTTP GET calendar_url with If-None-Match / If-Modified-Since
       ├─ 304 Not Modified → update last_extracted_at, skip (free)
       └─ 200 → hash body, compare to last_content_hash
           ├─ Hash unchanged → update last_extracted_at, skip (free)
           └─ Hash changed → continue to extraction

    2. Route by preferred_source + render_method
       ├─ ical → parse feed (no AI cost)
       ├─ jsonld → parse structured data from HTML (no AI cost)
       └─ ai → render page per render_method → send to Claude

    3. Import new/changed events (Stage 3 flow)
    4. Record run in venue_extraction_runs
    5. Update events_expected (rolling average), reset consecutive_failures
```

**Cost reality:** On any given day, ~70% of venue pages haven't changed. Of the ~30% that have, some use iCal/JSON-LD (free). Only the changed pages that need AI extraction cost anything. For 500 venues, this is ~$45-360/month.

### Stage 6: Strategy Adaptation (Anomaly-Triggered)

After each run, the pipeline compares results against the venue's historical profile:

```
After each extraction run:
  Compare events_extracted to events_expected

  Anomaly detected?
  ├─ Event count < 50% of expected → trigger re-evaluation
  ├─ Zero events (venue normally has them) → trigger re-evaluation
  ├─ 3+ consecutive failures → trigger re-evaluation, alert admin
  ├─ HTTP 301 → auto-update calendar_url, re-run
  ├─ HTTP 403/429 → escalate render_method to next tier
  ├─ Content changed but zero events → page redesign, trigger re-evaluation
  └─ All normal → no action

  Re-evaluation (when triggered):
    1. Try all strategies: ical detection → jsonld detection → static → dynamic → screenshot
    2. Score each: events_extracted × avg_confidence, weighted toward cheaper tiers
    3. Pick best strategy, update venue config
    4. Log strategy change in venue_extraction_runs
    5. Alert admin via Discord if strategy changed
```

**Example:** A venue migrates from WordPress to Squarespace. The next scheduled run returns 0 events (HTML structure completely different). Anomaly detection fires, re-evaluation tries all tiers, discovers the new site has JSON-LD markup. Strategy auto-switches from `ai`/`dynamic` to `jsonld` — now free and more reliable.

### Stage 7: Periodic Re-evaluation (Monthly)

Even without anomalies, each venue is re-evaluated once a month:

```
Monthly (staggered: venue_id % 30 = day of month):
  1. Re-run feed detection — did the venue add an iCal/RSS feed?
  2. Re-run JSON-LD detection — did the venue add structured data?
  3. Re-test cheaper render_methods — did the venue remove anti-bot?
  4. If cheaper/better strategy found → switch and log
  5. Update last_strategy_eval_at
```

**Why:** Venues improve their sites over time. A venue that needed `screenshot` tier six months ago might have added JSON-LD since then. Without periodic re-evaluation, we'd keep paying for AI extraction when free structured data is available.

### Stage 8: Admin Monitoring & Intervention

The admin dashboard provides visibility into the entire pipeline:

```
Pipeline Health Dashboard:
  ├─ Per-venue status cards
  │   ├─ Current strategy (preferred_source + render_method)
  │   ├─ Last run: timestamp, events extracted, success/fail
  │   ├─ Events: expected vs actual (with trend)
  │   ├─ Consecutive failures (highlighted if > 0)
  │   └─ Strategy history (last N changes with dates)
  │
  ├─ Aggregate metrics
  │   ├─ Total venues active / failing / locked
  │   ├─ Extraction cost this month (by tier)
  │   ├─ Events imported this week
  │   └─ Strategy distribution (how many venues on each tier)
  │
  └─ Admin actions
      ├─ Force re-evaluate (trigger immediate strategy detection)
      ├─ Strategy lock (prevent auto-switching)
      ├─ Edit calendar_url (when auto-detection can't find the new URL)
      ├─ Test extraction (dry run: fetch + extract + preview, don't import)
      └─ Disable venue (stop checking, keep historical data)
```

### Stage 9: Venue Lifecycle Events

Special cases the pipeline handles:

```
Venue closes permanently:
  → Shows stop appearing on calendar page
  → events_extracted drops to 0 over several runs
  → After N consecutive zero-event runs, alert admin
  → Admin marks venue as inactive, pipeline stops checking

Venue changes calendar URL:
  → HTTP 301 redirect → auto-update calendar_url
  → HTTP 404 → alert admin, hold current strategy
  → Admin manually updates URL → pipeline resumes

Venue gets acquired / rebranded:
  → Different name, same URL → extraction still works (AI doesn't care about branding)
  → Different URL → 301 redirect chain or admin manual update
  → May need artist merge/split if venue was tracking local acts

New city expansion:
  → Discover venues via Google Maps/OSM APIs
  → Admin reviews, approves, enters calendar_urls
  → Pipeline auto-detects strategies for all new venues
  → Festival data bootstraps the artist graph
  → Community fills gaps after initial pipeline pass
```

---

## Data Acquisition Strategy

### The core insight

**No single data source is reliable enough to depend on.** Playwright scrapers break on redesigns. APIs change terms, shut down, or degrade (Songkick). The resilient approach is multi-source with cross-referencing: if AI extraction says "Band X at Venue Y on March 15" and a ticketing API confirms it, confidence is high. If only one source has it, flag for review.

### Priority order (resilience + scalability)

| Tier | Source | Cost | Reliability | Dependency | Status |
|------|--------|------|-------------|------------|--------|
| 1 | **iCal/RSS feeds** | Free | Highest (structured, standard) | Venue publishes feed | To implement — detect feeds during venue onboarding |
| 2 | **HTTP JSON-LD** | Free | High (structured) | Venue has schema.org markup | Partially working (`jsonld` + `wix` providers) |
| 3 | **AI web extraction** | ~$0.01-0.03/page | High (universal) | Anthropic API | Core infrastructure — extraction service exists, needs screenshot pipeline |
| 4 | **Ticketing platform APIs** | Free tiers | Medium (TOS risk) | SeatGeek, Bandsintown, etc. | To evaluate — enrichment + validation, not primary |
| 5 | **Festival lineups** | Manual entry | High | Human time | Done (PSY-28) |
| 6 | **Community submissions** | Free | Variable | Active users | Planned (Phase 3) |

### How the tiers work together

For each venue in the system:

1. **Check for iCal/RSS feed first** — If the venue publishes a structured calendar feed, parse it. This is the cheapest, most reliable path. Detect feeds automatically during venue onboarding (look for `<link rel="alternate" type="application/rss+xml">`, `.ics` links, etc.).

2. **Check for JSON-LD structured data** — HTTP fetch the calendar page, look for `schema.org/MusicEvent` or `schema.org/Event` blocks. No browser needed. Already working for Van Buren, AZ Financial Theatre, Celebrity Theatre.

3. **AI extraction as universal fallback** — If no feed or structured data, take a screenshot of the venue calendar page and send it to Claude. The extraction service already exists (`services/extraction.go`) and works for flyer imports. Extending it to calendar pages is the key Phase 1.6 work. Cost: ~$0.01-0.03 per page, trivial at scale.

4. **API cross-referencing** — After importing from any source, query ticketing APIs (SeatGeek, Bandsintown) to validate and enrich with pricing, genre taxonomy, ticket URLs. APIs are a confidence multiplier, not a primary source.

### Change detection (cost optimization)

Don't re-extract every venue daily. For each venue calendar URL:
- Store HTTP `ETag` and `Last-Modified` headers from previous fetch
- On scheduled run, send `If-None-Match` / `If-Modified-Since` headers
- If `304 Not Modified`, skip extraction entirely
- If content changed, hash the page body and compare to stored hash (handles servers that don't support conditional requests)
- Only run AI extraction (the expensive step) on pages that actually changed

This reduces AI API costs dramatically — most venue calendars only change a few times per week.

### Venue discovery

How to find venues in a new city (pipeline component, not manual):
- **Google Maps API** — Query `type=night_club` or keyword "live music venue" in a city's bounding box
- **OpenStreetMap** — Query `amenity=music_venue` or `live_music=yes` via Overpass API
- **Festival lineups** — Artist pages → "upcoming shows" → discover venues they've played
- **SeatGeek venue catalog** — Query by city/state for music venues
- **Wikidata** — Venue entities with coordinates, capacity, official URLs

### Enrichment sources (parallel lookups on imported data)

| Source | Data provided | Constraint | Status |
|--------|--------------|------------|--------|
| **Ticketmaster Discovery API** | Genre/classification hierarchy, images, pricing | **No caching allowed** — must display live, cannot store in DB | To evaluate |
| **SeatGeek API** | Performer genres, pricing stats, ticket URLs | Must attribute SeatGeek, free tier | To evaluate |
| **Bandsintown API** | Artist tour dates, ticket links | Requires approval, commercial needs separate approval | To apply |
| **Setlist.fm API** | Historical setlists for shows/artists | Non-commercial free; 50K req/day | To apply |
| **MusicBrainz API** | Artist metadata, MBIDs, artist-label relationships | Open data, 1 req/s | Already using |
| **Bandcamp** | Release links, artist pages | HTTP scraping | Already using (CLI) |

### APIs evaluated and rejected

| API | Reason |
|-----|--------|
| **Songkick** | Acquired by Suno (Nov 2025). API was $500/mo before acquisition. Future uncertain. |
| **Eventbrite** | Event search endpoint deprecated Feb 2020. Can only fetch by known ID. |
| **PredictHQ** | Enterprise pricing, demand intelligence product. Wrong use case. |
| **JamBase** | 14-day trial only, commercial license required. Unknown pricing. Worth a trial for coverage evaluation. |

## Additional Data Source Ideas

### Social media extraction
Venues post shows on Instagram, Facebook, and Twitter. AI can extract event data from social posts and story images — a natural extension of the flyer extraction that already works. Could be community-powered: "forward this venue's Instagram post to import."

### Email newsletter parsing
Venues send weekly/monthly email digests with upcoming shows. If a user forwards a venue newsletter, AI extracts all events from it. Zero scraping, community-powered data source.

### Web search synthesis ("Perplexity model")
Instead of visiting one venue at a time, search "upcoming shows at [venue] [city] [month]" → synthesize structured events from multiple search results. One query, multiple signals.

## AI Extraction Architecture

### Why AI extraction is core infrastructure, not an experiment

The extraction service (`services/extraction.go`) already:
- Accepts text or images
- Sends to Claude Haiku via Anthropic API
- Returns structured JSON: artists (with `is_headliner`), venue, date, time, cost, ages
- Works today for admin flyer imports

**What's needed to make it the primary pipeline:**

1. **Tiered rendering** — Most venue sites have anti-scraping protections and/or render calendars client-side via JavaScript (React/Vue SPAs, embedded ticketing widgets like DICE/Eventbrite/See Tickets, WordPress AJAX calendar plugins, Squarespace/Wix dynamic templates). A plain HTTP GET returns an empty shell with no event data. The pipeline must "see" what users see, which requires a headless browser for JS-rendered pages. See "Rendering Strategy" below for the tiered approach.

2. **Calendar page understanding** — Extend the extraction prompt to handle full calendar pages (multiple events), not just single flyers. Return an array of events.

3. **Multi-page navigation** — Some venue calendars paginate. For Phase 1.6, start with single-page extraction (tiers 1-3). Phase 2 adds Anthropic's Computer Use API (tier 4) — Claude navigates paginated calendars the way a user would, clicking "Next Month" / "Load More" with zero per-venue navigation code. See "Computer Use — Phase 2 Upgrade Path" below.

4. **Confidence scoring** — AI extraction should return confidence per field. "Date: 2026-03-15 (high)" vs "Price: $25 (medium, inferred from 'tickets start at $25')."

### Rendering Strategy

**Core assumption:** Most venue websites have anti-scraping protections, JS-rendered content, or both. The pipeline must render pages the way a real user sees them. We use a tiered approach to minimize cost while ensuring universal coverage.

**Rendering tiers (per-venue, auto-detected on first run, cached):**

| Tier | `render_method` | When to use | Tool | Cost | Phase |
|------|----------------|-------------|------|------|-------|
| 1 | `static` | Plain HTML contains event data (HTML-first sites, SSR pages) | HTTP GET | Cheapest — no browser | 1.6 |
| 2 | `dynamic` | JS-rendered calendar (SPAs, ticketing widgets, AJAX plugins) | chromedp (headless Chrome, pure Go) | Moderate — browser render | 1.6 |
| 3 | `screenshot` | DOM is obfuscated, Canvas-rendered, or chromedp HTML still unreadable | chromedp + full-page screenshot | Expensive — image tokens | 1.6 |
| 4 | `computer_use` | Multi-page calendars with pagination, aggressive anti-bot, CAPTCHAs, cookie walls | Anthropic Computer Use API (beta) | Most expensive — multi-turn API | 2 |

**Auto-detection on first run:**

1. HTTP GET the venue's `calendar_url`
2. If HTML body > 5KB and contains recognizable event markers (dates, artist names, ticket links) → `render_method = 'static'`
3. If HTML body is mostly empty (`<div id="root">`, `<noscript>`, `<script>` bundles, body < 5KB of content) → render with chromedp, then check rendered HTML → `render_method = 'dynamic'`
4. If chromedp-rendered HTML still lacks recognizable event data (Canvas-rendered, heavy obfuscation) → fall back to screenshot → `render_method = 'screenshot'`
5. Save `render_method` on the venue source config — subsequent runs skip auto-detection

**chromedp over Playwright:**

We use [chromedp](https://github.com/chromedp/chromedp) (pure Go, no Node/Playwright dependency). Critical difference from the broken Playwright scrapers: **never use `networkidle`**. Instead:
- Wait for a specific CSS selector (event container) with a timeout, OR
- Wait a fixed duration (5-8 seconds) — sufficient for most JS calendars

The Playwright scrapers failed because `networkidle` never fires on pages with analytics/ad trackers. That's a wait strategy problem, not a rendering problem. chromedp with targeted waits solves this.

**Infrastructure cost (chromedp worker pool):**

| Concurrent renders | RAM | Throughput |
|---|---|---|
| 1 | ~200MB | Sequential, ~60 venues/hour |
| 5 | ~1GB | ~300 venues/hour |
| 10 | ~2GB | 500+ venues/hour — comfortable for daily checks |

Only JS-rendered venues (estimated 40-60% of total) need the chromedp path. Static HTML venues use plain HTTP — zero browser cost.

**What this handles:**
- Venue redesigns → AI reads new HTML/screenshot, doesn't care about CSS/structure changes
- Platform migrations (WordPress → Squarespace) → AI adapts, zero code changes
- Ticketing widget swaps (Eventbrite → DICE) → still renders in browser, still extracted
- Anti-scraping protections → chromedp runs real Chrome, indistinguishable from a user

**What breaks this:** Venue changes their calendar URL. That's why `calendar_url` is the stable primitive we track. URL changes can be detected via 301 redirects → auto-update stored URL.

### Computer Use — Phase 2 Upgrade Path

Anthropic's [Computer Use API](https://docs.anthropic.com/en/docs/agents-and-tools/computer-use) (beta) gives Claude direct control of a virtual desktop: it receives screenshots and can execute mouse clicks, keyboard input, and scrolling. This is the programmatic equivalent of Claude's Chrome browser extension, but callable from our Go backend.

**Why it matters for the pipeline:**

Tiers 1-3 handle single-page extraction well, but some venue calendars paginate across multiple pages ("Next Month", "Load More", infinite scroll). Building custom navigation logic per pagination pattern is the same per-venue brittleness we're escaping. Computer use solves this generically — Claude navigates the calendar the way a user would, clicking through pages and extracting events as it goes.

**How it would work:**
1. Spin up a lightweight container with a browser + virtual display (Docker + VNC/Xvfb)
2. Send Claude the venue calendar URL via the computer use tool
3. Claude navigates the page, scrolls through the calendar, clicks "Next Month" / "Load More", handles cookie consent dialogs and CAPTCHAs
4. Claude extracts all events across all pages as structured data
5. One API conversation = complete multi-page extraction

**When to use (Tier 4):**
- Venues with paginated calendars where tiers 1-3 only capture the first page
- Sites with aggressive anti-bot protections that detect and block headless Chrome
- Sites requiring interaction (cookie consent, age gates) before showing content
- Complex navigation patterns that would require per-venue custom code

**Trade-offs vs chromedp (tiers 2-3):**
- **Slower:** ~15-30s per venue vs 5-8s (multiple API roundtrips: screenshot → action → screenshot → action)
- **More expensive:** Multiple API calls per venue, image tokens each roundtrip
- **More capable:** Handles pagination, CAPTCHAs, multi-step navigation with zero custom code
- **API maturity:** Still in beta — may change. Phase 2 timeline gives it time to stabilize.

**Infrastructure:** Requires a container with a display server. Options: Docker with Xvfb, or a lightweight VM. ~512MB RAM per container. Can run alongside chromedp worker pool or replace it for specific venues.

**Phase 2 plan:** Evaluate computer use for the ~10-20% of venues that paginate or resist headless Chrome. Keep chromedp as the workhorse for the majority. Per-venue `render_method = 'computer_use'` in the source config, same auto-detection pattern — if tiers 1-3 only capture partial data (suspiciously few events for a busy venue), auto-escalate to tier 4.

### Strategy Adaptation & Self-Healing

The pipeline must remember what works per venue and automatically adapt when venues change. A one-time auto-detection is not enough — venue websites get redesigned, switch platforms, add/remove anti-bot protections, change URLs, and paginate differently over time. The pipeline needs a feedback loop.

#### Extraction Run History

Every extraction attempt is recorded in a `venue_extraction_runs` table:

```
venue_extraction_runs:
  id, venue_id, run_at,
  render_method,          -- which tier was used
  preferred_source,       -- which source type was used
  events_extracted,       -- count of events found
  avg_confidence,         -- average AI confidence score across fields
  artist_match_rate,      -- % of extracted artists that matched existing DB artists
  content_hash,           -- page body hash (for change detection)
  http_status,            -- response status code
  error,                  -- error message if failed
  duration_ms             -- how long the extraction took
```

This gives a complete audit trail per venue: what was tried, what worked, how well it worked, and when things changed.

#### Venue Strategy Profile

The venue source config (PSY-36) gets additional fields derived from run history:

```
events_expected,          -- rolling average of events_extracted (updated after each successful run)
consecutive_failures,     -- counter, reset on success
last_strategy_eval_at,    -- when we last re-evaluated the render_method / preferred_source
strategy_locked,          -- admin override to prevent auto-switching (for venues with known quirks)
```

`events_expected` is the key metric. A venue that consistently returns 20-25 events per run has an expected baseline. If a run returns 3 events, something changed.

#### Anomaly Detection Triggers

After each extraction run, compare results against the venue's historical profile:

| Signal | Trigger | Action |
|--------|---------|--------|
| **Event count drop** | `events_extracted < events_expected * 0.5` | Flag for re-evaluation |
| **Zero events** | `events_extracted = 0` on a venue that normally has events | Immediate re-evaluation |
| **Consecutive failures** | 3+ runs with errors | Re-evaluate, then alert admin if still failing |
| **HTTP status change** | 301 redirect | Auto-update `calendar_url` to new location |
| **HTTP status change** | 403/429 | Escalate render_method (static → dynamic → screenshot → computer_use) |
| **Confidence drop** | `avg_confidence` drops below 0.5 | Flag for review — extraction is working but data quality is poor |
| **Content changed, zero events** | `content_hash` changed but `events_extracted = 0` | Page redesign likely broke current strategy — re-evaluate |

#### Re-evaluation Flow

When triggered (by anomaly or periodic schedule), the pipeline re-runs the original auto-detection sequence:

1. **Try all tiers in order:** static → dynamic → screenshot (→ computer_use in Phase 2)
2. **Score each tier:** events extracted, confidence, artist match rate, cost, duration
3. **Pick the best:** Highest events × confidence, weighted toward cheaper tiers when results are comparable
4. **Compare to current:** If a cheaper tier now works (e.g., venue added JSON-LD), downgrade. If current tier broke, upgrade.
5. **Update venue config:** Set new `render_method`, `preferred_source`, `last_strategy_eval_at`
6. **Log the change:** Record strategy switch in extraction runs for audit trail

**Example scenarios:**
- Venue migrates from React SPA to static SSR → re-eval detects HTML now has event data → downgrade from `dynamic` to `static` (cheaper)
- Venue adds Cloudflare anti-bot → `dynamic` starts getting 403s → escalate to `screenshot` or `computer_use`
- Venue adds an iCal feed → periodic re-eval detects `<link rel="alternate" type="text/calendar">` → switch `preferred_source` from `ai` to `ical` (free)
- Venue changes calendar URL → 301 detected → auto-update URL, re-run extraction at new location

#### Periodic Re-evaluation

Even without anomalies, re-evaluate each venue monthly:
- A venue might have added structured data (iCal, JSON-LD) since we last checked — switch to the cheaper source
- A venue might have removed anti-bot protections — downgrade from `screenshot` to `dynamic`
- A venue might have started paginating — check if single-page extraction is still getting full coverage

**Schedule:** Stagger re-evaluations across the month (e.g., venue_id % 30 = day of month). Don't re-evaluate all venues on the same day.

#### Admin Controls

- **Strategy lock:** Admin can lock a venue's `render_method` / `preferred_source` to prevent auto-switching (useful for venues with known quirks)
- **Force re-evaluate:** Admin button to trigger immediate re-evaluation for a specific venue
- **Strategy history:** Admin view showing all strategy changes for a venue over time (from extraction runs table)
- **Pipeline health dashboard:** Per-venue status showing current strategy, last run result, events expected vs actual, consecutive failures, days since last strategy change

### Cost model

- Claude Haiku per page (text): ~$0.01-0.03
- Claude Haiku per page (screenshot): ~$0.03-0.08
- 200 venues × 1 extraction/day × 30 days = $60-180/month (text) to $180-480/month (all screenshots)
- With change detection (only ~30% of pages change daily): $18-144/month for 200 venues
- 500 venues with change detection: $45-360/month
- chromedp infrastructure: negligible (runs on existing server, ~1-2GB RAM for worker pool)
- Trivially cheap compared to maintaining per-venue Playwright scrapers

## Phase 1.6: Discovery Pipeline & Data Quality

Linear project: PSY-29 through PSY-36. **Revised strategy: AI-first, multi-source pipeline.**

### Revised build order

1. ~~**Provider reliability audit** (PSY-29)~~ — DONE. 2/7 passing. Conclusion: pivot away from Playwright.
2. **AI extraction pipeline** (PSY-32, elevated from experiment to core) — Build the tiered rendering pipeline (static HTTP → chromedp dynamic → chromedp screenshot) with auto-detection. Most venue sites are JS-rendered or have anti-scraping protections, so chromedp (pure Go headless Chrome) is the default rendering path. Extend extraction service to handle calendar pages (multiple events per page). Add HTTP change detection (ETag/hash). Start with 3-5 Phoenix venues. This is the foundation everything else builds on.
3. **iCal/RSS feed detection** (new) — During venue onboarding, auto-detect structured feeds. Parse iCal/RSS as the cheapest, most reliable data path.
4. **Data provenance tracking** (PSY-34) — `data_source`, `source_confidence`, `last_verified_at` on core entity tables. Foundation for multi-source cross-referencing.
5. **AI billing enrichment** (PSY-30) — Extend extraction to determine headliner/support/opener from event data. Expand `set_type` vocabulary.
6. **Automated scheduling** (PSY-31) — Scheduled extraction runs with change detection. Discord/Sentry alerts on failures. Admin dashboard shows source status.
7. **API enrichment integration** (part of PSY-35) — SeatGeek and/or Bandsintown as cross-referencing/enrichment sources. Query after AI extraction to validate and add pricing/genre data.
8. **Post-import enrichment pipeline** (PSY-35) — Automatic artist matching, MusicBrainz lookup, API cross-referencing, AI billing enrichment. Event-driven, async, rate-limited.
9. **Consolidate discovery UI** (PSY-33) — Retire standalone discovery app, add "Data Pipeline" admin tab showing all sources.
10. **Venue-level source config** (PSY-36) — Per-venue config in DB: preferred source (ical/jsonld/ai/api), calendar URL, feed URL, API venue IDs, schedule, change detection state, `artist_order_hint`, `auto_approve`.

### Deprioritized

- **Fixing broken Playwright scrapers** — Not worth investment. Keep `jsonld` HTTP-only path as a fast-path for venues with structured data. Let Playwright scrapers bitrot.
- **SeatGeek as primary source** — Useful for enrichment/validation, but not the primary pipeline. We don't want to depend on any single API.

## Legacy Providers (Playwright)

Kept for reference but no longer actively maintained.

| Provider | Type | Venue(s) | Audit Status (March 2026) |
|----------|------|----------|--------------------------|
| `ticketweb` | Playwright | Valley Bar, Crescent Ballroom | Preview PASS, Scrape TIMEOUT (`networkidle` hangs) |
| `jsonld` | HTTP+Playwright | The Van Buren, AZ Financial Theatre | PASS (both preview and scrape) |
| `seetickets` | Playwright | The Rebel Lounge | Preview PASS, Scrape TIMEOUT |
| `emptybottle` | Playwright | The Empty Bottle | Preview PASS, Scrape TIMEOUT |
| `wix` | HTTP-only | Celebrity Theatre | Preview TIMEOUT (551 sitemap pages), data eventually loads |

**Root cause of scrape failures:** `waitUntil: 'networkidle'` — pages with analytics/ad trackers never reach network idle state. The `jsonld` provider succeeds because its HTTP pass doesn't use Playwright for the main data, only for optional enrichment.

## Roadmap

### Phase 1.6: AI-First Pipeline (Now)

- [ ] AI extraction pipeline (PSY-32, elevated) — tiered rendering (static/dynamic/screenshot) → Claude → structured events for calendar pages
- [ ] iCal/RSS feed detection — auto-detect and parse structured venue feeds
- [ ] Change detection — HTTP ETag/hash-based, skip unchanged pages
- [ ] AI billing enrichment (PSY-30) — headliner/support determination
- [ ] Automated scheduling (PSY-31) — scheduled runs with change detection + alerting
- [ ] Data provenance (PSY-34) — source tracking on all imported data
- [ ] API enrichment (part of PSY-35) — SeatGeek/Bandsintown for validation + pricing/genre
- [ ] Post-import enrichment pipeline (PSY-35) — async artist matching + metadata + cross-referencing
- [ ] Consolidate discovery UI (PSY-33) — retire standalone app
- [ ] Venue-level source config (PSY-36) — per-venue source preferences + feed URLs + API IDs
- [x] ~~Provider audit (PSY-29)~~ — DONE. Conclusion: AI-first, not scraper-first.
- [x] ~~Festival data entry~~ — DONE (PSY-28)
- [x] ~~MusicBrainz seeding~~ — DONE (PSY-14)
- [x] ~~Bandcamp enrichment~~ — DONE (PSY-15)

### Phase 2: Multi-Source + Community

- [ ] Bandsintown API — artist-centric tour date discovery
- [ ] Setlist.fm integration — historical setlist enrichment
- [ ] AI agent navigation — multi-page venue calendars, pagination handling
- [ ] Social media extraction — AI reads venue Instagram/Facebook posts
- [ ] Email newsletter parsing — AI extracts events from forwarded venue emails
- [ ] Community show/festival submission through approval workflow
- [ ] Discogs catalog data enrichment
- [ ] Admin UI: reorder artists and change set types on show edit page
- [ ] Unified entity matching across all sources

### Phase 3: Community as Data Source

- [ ] Community contributions flow into the same pipeline
- [ ] Community bill position correction (through pending-edit workflow)
- [ ] Community festival lineup corrections and enrichment
- [ ] Merge/deduplicate logic across all source types
- [ ] Confidence scoring (multi-source cross-referencing + community verification)

### Phase 4+: Scale

- [ ] **Venue discovery automation** — Google Maps API + OpenStreetMap to find music venues in new cities
- [ ] New city onboarding: AI extraction covers any venue with a calendar URL, festivals bootstrap artist graph, APIs validate
- [ ] Partner data feeds (venues/promoters submitting directly)
- [ ] Web search synthesis — "Perplexity model" for event discovery

## Key Files

| Area | Files |
|------|-------|
| AI extraction service | `backend/internal/services/extraction.go` |
| Backend discovery service | `backend/internal/services/discovery.go` |
| Backend admin handler | `backend/internal/api/handlers/admin.go` |
| Rendering / fetcher (planned) | `backend/internal/services/fetcher.go` (chromedp + HTTP tiered rendering) |
| Pipeline scheduler (planned) | `backend/internal/services/pipeline.go` (ticker-based worker pool) |
| Legacy providers | `discovery/src/server/providers/` |
| Legacy import UI | `discovery/src/` (React + Bun server) |
| Provider audit script | `discovery/audit-providers.ts` |
| Audit results | `discovery/audit-results.json` |
