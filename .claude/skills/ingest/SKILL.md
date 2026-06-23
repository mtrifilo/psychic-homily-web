---
name: ingest
description: Extract entities from screenshots (show flyers, WFMU playlists, tour announcements, festival lineups) and import them into the Psychic Homily knowledge graph via the ph CLI.
argument-hint: "[dev|stage|prod] [screenshot/post, or a venue events-page URL]"
---

# Ingest: Screenshot to Knowledge Graph

Extract structured entity data from screenshots and import into Psychic Homily using the `ph` CLI.

## Environment Targeting

By default, commands use whichever environment is set as default in `~/.psychic-homily/config.json`.

**Shorthand:**
- `/ingest dev ...` — targets local dev
- `/ingest stage ...` — targets staging
- `/ingest prod ...` — targets production
- `/ingest ...` — uses default environment

**Full form also works:** `/ingest --env <name> ...`, where `<name>` is a configured environment (e.g. `/ingest --env production ...`).

**Parsing rule:** If the first word of the argument is `dev`, `local`, `stage`, `staging`, `prod`, or `production`, treat it as the environment and strip it from the rest of the input. When an environment is specified (by shorthand or `--env`), append `--env <name>` to ALL `ph` commands in this workflow.

**Resolve the shorthand against the actual configured environment names — do not assume them.** The CLI does an exact-name match, so a wrong name fails hard with `Environment "X" not found`. Run `config show` (below) and map the shorthand to whichever configured env matches: `dev`/`local` → the local env, `stage`/`staging` → the staging env, `prod`/`production` → the production env. As of this writing the configured names are `local`, `stage`, and `production` — note the staging env is named **`stage`, not `staging`**, so `/ingest stage` → `--env stage`. Don't hardcode a name that `config show` doesn't list.

Check current default with:
```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts config show
```

## Prerequisites

The `ph` CLI must be configured. Check with:
```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts status
```

If not configured, set up with:
```bash
# Generate API token (local dev)
cd /Users/mtrifilo/dev/psychic-homily-web/backend && go run ./cmd/gen-api-token --make-admin

# Configure CLI (use the token from above)
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts init --url http://localhost:8080 --token phk_xxx --name local
```

## Workflow

### Step 1: Extract Data from Screenshot/Post

> The extraction rules below are reified (with a JSON schema + eval harness) at `cli/eval/extraction-prompt.md` and `cli/eval/batch-schema.json` — the source of truth shared with the regression evals (`cli/eval/README.md`). Keep this prose and that artifact in sync.

When the user provides a screenshot or post (show flyer, WFMU playlist, tour poster, festival lineup, Instagram post, etc.), analyze ALL available sources of information:

- **Image/flyer**: Extract visible text, artist names, dates, venues, prices
- **Caption/text**: Parse any accompanying text (Instagram captions, tweet text, post body) for additional show data, dates, venues, @handles, and ticket links
- **Both together**: Cross-reference image and caption — captions often contain details not on the flyer (tour dates, @handles, ticket links)

**For WFMU playlists** — extract: artists, tracks, albums (→ releases), labels, years
**For show flyers** — extract: artists (with headliner/opener), venue, date, city/state, price
**For tour announcements / multi-show posts** — extract: ALL shows listed. Create one show entry per date, each with its own venue, city, state, and full artist lineup. A single Instagram post may contain 5-20 shows.
**For festival lineups** — extract: festival name, dates, artists with billing tiers, venue(s)

#### Multi-show extraction

Instagram posts, tour announcements, and promotional posts frequently list multiple shows. Always look for:
- Tour date lists in captions (e.g., "4/15 Phoenix, AZ @ Valley Bar / 4/16 Tucson, AZ @ 191 Toole")
- Multiple dates on a flyer image
- Separate flyers in a carousel (user may provide multiple screenshots)

Each date becomes its own show entry in the batch JSON. The artist lineup is typically the same across all dates unless specified otherwise. Example of a tour post producing multiple shows:

```json
[
  {"entity_type": "artist", "name": "La Witch", "city": "Los Angeles", "state": "CA", "instagram": "https://instagram.com/la_witch"},
  {"entity_type": "venue", "name": "Valley Bar", "city": "Phoenix", "state": "AZ"},
  {"entity_type": "venue", "name": "191 Toole", "city": "Tucson", "state": "AZ"},
  {"entity_type": "show", "event_date": "2026-04-15", "city": "Phoenix", "state": "AZ", "artists": [{"name": "La Witch", "is_headliner": true}], "venues": [{"name": "Valley Bar", "city": "Phoenix", "state": "AZ"}]},
  {"entity_type": "show", "event_date": "2026-04-16", "city": "Tucson", "state": "AZ", "artists": [{"name": "La Witch", "is_headliner": true}], "venues": [{"name": "191 Toole", "city": "Tucson", "state": "AZ"}]}
]
```

#### Social links (Instagram + other platforms)

Posts and roster / venue pages expose social links for artists, venues, and labels — as `@handles` in captions/tags/image text, or as full links on a page. Map them to **full on-platform URLs** (the backend rejects bare handles):

- `@la_witch` → `"instagram": "https://instagram.com/la_witch"`
- `@sidthecatauditorium` → `"instagram": "https://instagram.com/sidthecatauditorium"`
- a Twitter/X `@handle` → `"twitter": "https://twitter.com/handle"` (an `x.com` link is also valid on the `twitter` field)
- Facebook / YouTube / Spotify / SoundCloud / Bandcamp links → capture the full URL on the field whose host matches: `facebook` (`facebook.com`), `youtube` (`youtube.com`/`youtu.be`), `spotify` (`open.spotify.com`), `soundcloud` (`soundcloud.com`), `bandcamp` (`*.bandcamp.com`); any other off-platform link → `website`.

Set the matching social field on artist / venue / label batch items when a link is identified. Only include links that clearly correspond to an entity being created. Example:

```json
[
  {"entity_type": "artist", "name": "La Witch", "city": "Los Angeles", "state": "CA", "instagram": "https://instagram.com/la_witch"},
  {"entity_type": "venue", "name": "Sid the Cat Auditorium", "city": "Phoenix", "state": "AZ", "instagram": "https://instagram.com/sidthecatauditorium"}
]
```

**Matching handles to entities**: Use context clues — handle text usually resembles the artist/venue name (underscores for spaces, abbreviations). When a handle clearly maps to an entity in the post, include it. When ambiguous, skip it.

**Post-author handle ≠ artist name is common — confirm, don't guess.** A post's author handle is frequently the performing artist's own social handle under a different moniker, even when it bears no resemblance to the band name (e.g. `@mercury_tracer` is the handle for the artist *Midwife*). When the author handle doesn't obviously match the named artist, do NOT silently create a separate entity for the handle, and do NOT silently discard it — ask the user whether the handle belongs to the artist. If it does, attach it as that artist's `instagram` rather than minting a second artist/venue.

### Step 2: Build Batch JSON

Create a JSON file at `/tmp/ph-ingest.json` with the extracted data. Use this format:

```json
[
  {"entity_type": "label", "name": "Label Name", "country": "US", "website": "https://..."},
  {"entity_type": "artist", "name": "Artist Name", "city": "City", "tags": ["genre-tag", {"name": "Japanese", "category": "locale"}]},
  {"entity_type": "release", "title": "Album Title", "release_type": "lp", "release_year": 2025, "artists": [{"name": "Artist Name"}]},
  {"entity_type": "venue", "name": "Venue Name", "city": "City", "state": "ST", "website": "https://..."},
  {"entity_type": "show", "event_date": "2026-04-15", "city": "Phoenix", "state": "AZ", "artists": [{"name": "Artist Name", "is_headliner": true}], "venues": [{"name": "Venue Name", "city": "Phoenix", "state": "AZ"}]},
  {"entity_type": "festival", "name": "Fest Name 2026", "series_slug": "fest-name", "edition_year": 2026, "start_date": "2026-06-01", "end_date": "2026-06-03", "artists": [{"name": "Artist", "billing_tier": "headliner"}]}
]
```

#### Entity schemas

**artist**: `name` (required), `city`, `state`, `country`, `instagram`, `facebook`, `twitter`, `youtube`, `spotify`, `soundcloud`, `bandcamp`, `website`, `description`, `tags`, `label` (name of a label to link this artist to — resolved by exact name after create; links via `POST /admin/labels/{id}/artists`)
**venue**: `name` (required), `city` (required), `state` (required), `address`, `zipcode`, `country`, `instagram`, `facebook`, `twitter`, `youtube`, `spotify`, `soundcloud`, `bandcamp`, `website`, `description`, `tags`
**show**: `event_date` (required, YYYY-MM-DD), `city` (required), `state` (required), `title`, `price`, `ticket_url` (URL for ticket purchase -- extract from flyers when visible), `artists` (required, array of `{name, is_headliner?}`), `venues` (required, array of `{name, city, state}`)
**release**: `title` (required), `release_type` (lp/ep/single/compilation/live/remix/demo), `release_year`, `artists` (required), `external_links` ([{platform, url}]), `tags`
**label**: `name` (required), `city`, `state`, `country`, `founded_year`, `instagram`, `facebook`, `twitter`, `youtube`, `spotify`, `soundcloud`, `bandcamp`, `website`, `description`, `tags`, `artists` (optional inline roster — see "Label roster-page ingest" below). Use `bandcamp` (the canonical field); the legacy `bandcamp_url` alias is accepted but normalized to `bandcamp` before submit.
**festival**: `name` (required), `series_slug` (required), `edition_year` (required), `start_date` (required), `end_date` (required), `city`, `state`, `artists` ([{name, billing_tier}]), `tags`

#### Tag format

Tags can be strings (defaults to genre) or objects with category:
```json
"tags": ["punk", "noise rock", {"name": "Japanese", "category": "locale"}]
```

Categories: `genre`, `locale`, `other`. String tags default to genre. Use `locale` for nationality/origin tags (e.g., Japanese, Irish). Use `other` for non-genre descriptors.

#### Processing order

The batch command processes in dependency order: labels → artists → releases → venues → festivals → shows. Put entities in any order — the CLI handles sequencing.

#### Guidelines for data extraction

- **Artist names**: Use the exact spelling from the source. Don't correct or normalize.
- **Release types**: `lp` for full albums, `ep` for EPs, `compilation` for comps/anthologies, `live` for live recordings, `single` for singles.
- **Release years**: Use the original release year when available. For reissues, use the reissue year.
- **Tags**: Add genre and locale tags where you can confidently identify them. Common genres: punk, post-punk, noise rock, psychedelic, electronic, industrial, experimental, ambient, folk, gospel, funk, disco, synth pop, avant-garde, hip-hop, jazz, metal. Locale tags use `category: "locale"`: Japanese, German, Spanish, Russian, Thai, Brazilian, etc.
- **Billing tiers** (festivals): headliner, sub_headliner, mid_card, undercard, local, dj, host.
- **Skip non-music entries**: DJ interludes, radio commercials, compilation album titles without a distinct artist, trivia nights.
- **@handles**: When processing Instagram or social media posts, extract @handles from captions and map them to Instagram URLs on the corresponding artist or venue entities.

### Step 3: Dry Run

Run the batch in dry-run mode and show the user the preview. If `--env` was specified in the `/ingest` arguments, include it on all commands:
```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts batch /tmp/ph-ingest.json
# Or with explicit environment:
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts --env production batch /tmp/ph-ingest.json
```

Present the output to the user and ask for confirmation. Highlight:
- How many entities of each type will be created/updated/skipped
- Any fuzzy tag matches (where existing similar tags were found)
- Any unresolved artists (for releases/shows)
- Any validation errors

### Step 4: Confirm

After user approval (use same `--env` flag if specified):
```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts batch --confirm /tmp/ph-ingest.json
# Or with explicit environment:
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts --env production batch --confirm /tmp/ph-ingest.json
```

Report the results: how many created, updated, skipped, errored.

### Step 5: Fix-ups (if needed)

If any entities failed (e.g., unresolved artists for releases), fix them individually (use same `--env` flag if specified):
```bash
# Create the missing artist
bun run src/entry.ts submit artist --confirm '[{"name": "Missing Artist"}]'

# Retry the release
bun run src/entry.ts submit release --confirm '[{"title": "Album", "artists": [{"name": "Missing Artist"}]}]'
```

## Troubleshooting & gotchas

- **`event_date` is stored as a timestamp, not a bare date.** A date-only `event_date` (`YYYY-MM-DD`) is normalized to **20:00 venue-local time → UTC** (timezone from the venue's state; PSY-985/986). So `2026-07-17` at a CA venue is stored as `2026-07-18T03:00:00Z`. This is expected — don't "correct" it.

- **A `422 SHOW_CREATE_FAILED` when (re-)submitting a show usually means the show already exists.** The backend enforces a unique `(artist, venue, event_date)` key. If a confirmed run already created the show, re-running hits that constraint. Before assuming a real failure, verify existence (see below). *(The CLI's client-side dedup pre-check was timezone-corrected so legitimate re-runs now report `DUPLICATE (ID: …) — skipping` cleanly instead of erroring — but older shells/builds may still surface the 422.)*

- **Verify whether a show exists by date, not by the artist's "Shows" count.** `search artist` / `/artists/search` does **not** populate an upcoming-show count, so it is *not* evidence an artist has no shows. To check real shows, query the date window or the per-artist endpoint:
  ```bash
  # Shows on a given UTC day (note the venue-local→UTC shift: an evening show on
  # 7/17 local lands in the 7/18 UTC window)
  curl -s "$URL/shows?from_date=2026-07-18T00:00:00Z&to_date=2026-07-18T23:59:59Z" -H "Authorization: Bearer $TOKEN"
  # All upcoming shows for an artist (authoritative count)
  curl -s "$URL/artists/<id>/shows?time_filter=upcoming&limit=50" -H "Authorization: Bearer $TOKEN"
  ```

- **`search show "<query>"` matches by city only** (not title/artist/venue), and is unreliable for existence checks — prefer the date-window query above.

- **Verify a venue's/artist's shows by the response `total`, and don't let a 422 read as "0 shows".** `GET /venues/{id}/shows` and `GET /artists/{id}/shows` return the real count in a `total` field and cap `limit` at **200** (raised from 50 in PSY-1031). A `limit` over the cap returns **HTTP 422**, not truncated data — and a naive `curl … | node -e 'd.shows||[]'` reads the 422 error body (which has no `shows`) as an empty array, i.e. "this venue/artist has no shows" when it actually has dozens. Always check the HTTP status, keep `limit ≤ 200`, and trust `total`. (This briefly fooled the Thalia Hall verification into thinking 64 just-created shows hadn't attached.)

- **Festival-named tour stops are festivals, not venues.** When a tour flyer lists a stop like "Mosswood Meltdown" or "Desert Fox Festival", create a `festival` entity (with the touring act on the bill) rather than a venue/show. A festival's own **pre-party/aftershow** at a real venue *is* a separate titled `show` (use the `title` field, e.g. "Mosswood Meltdown Pre-Party").

## Venue events-page periodic ingest

For keeping a venue's whole calendar current (re-run every few weeks). Proven on First Avenue (2026-06-07, ~330 shows / 7 months in one pass).

**Design principle — agent-driven extraction over brittle scrapers.** Venue sites redesign / swap frameworks occasionally. Do NOT hardcode a CSS-selector scraper that silently breaks. Instead: render the page in a browser, have the agent inspect the live DOM each run and adapt, and keep only a tiny per-venue *config* (URL, pagination hint, room→city map, filter prefs) below. When a site changes, you usually just re-inspect — no code to fix.

**Re-runs are safe (idempotent).** Existing artists/venues/shows skip cleanly on `ph batch` thanks to entity dedup + the show-dedup timezone-window fix (PSY-999) — a periodic re-run only adds genuinely new shows.

### Invoking (what the user types)

- **Add a new venue:** `/ingest <env> — add a new venue from its events page: <URL>. Run the venue events-page workflow: extract all upcoming months, music concerts only, correct city/state, dry-run + both QA scans, pause for my OK, then write, add a registry row, AND register it in the source registry so it joins stale-first refresh.`
- **Refresh an existing venue:** `/ingest <env> — refresh <venue name>'s listings using its registry row below. Re-scrape all upcoming months, dry-run (idempotent → only new shows), both QA scans, my OK, then write and stamp the refresh.`

Either way: always dry-run + the two QA scans (step 6) and get explicit confirmation before `--confirm`.

### Workflow

1. **First, look for an upstream data API — it usually beats scraping the DOM.** Many venue sites inject events client-side from a JSON API via a widget script. `curl` the page; if the shows aren't in the server HTML, `grep` it for `<script src=…>`, fetch that JS, and `grep` *it* for `fetch(` / `ajax` / API hostnames. **If it calls a structured API (Ticketmaster Discovery, SeeTickets, DICE, a WordPress REST route…), hit that JSON directly** — it already has clean artist entities, dates, prices, and ticket URLs, so you skip DOM parsing, pagination, and year-inference entirely (see the Thalia Hall / 16-on-Center registry row). Then **cross-check**: render the page in the browser MCP and confirm the visible card count matches the API result 1:1 (catches a second feed or a filter the API view misses).
2. **Otherwise, render + scrape.** Server-rendered page → `curl -s <url> -A "Mozilla/5.0"`. JS-rendered/paginated → browser MCP (`chrome-devtools`): `new_page(url)` then `evaluate_script`. (First Avenue is JS — current month is server-rendered, later months load via JS.)
3. **Discover structure once** (scrape path only). One `evaluate_script` to find the show-card selector + sub-fields (headliner / supports / venue / date) and the pagination mechanism (URL param vs. a "next" control). Look at the rendered DOM, don't assume.
4. **Extract all pages.** If pagination is a URL param, loop same-origin `fetch()` inside one `evaluate_script` (no CORS), `DOMParser` each response, accumulate. Else click the next control and re-extract per page. Stop after 2 consecutive empty pages. **Dates often lack a year** (Empty Bottle) — infer it: start at the current year and bump it whenever a card's month number drops below the previous card's (Dec→Jan rollover). Save raw output via `evaluate_script` `filePath` to a **workspace-internal scratch path you will delete afterward** (`/tmp` is rejected by the MCP; repo root works if you `rm` it, or use a gitignored dir). **Scrub all scratch files before finishing** so nothing lands in a commit.
5. **Transform programmatically** (never hand-transcribe hundreds of rows) — start from the **skeleton + shared rulesets below**. Apply the room→city/state map, the shared music-only **exclusion** + **headliner-cleaning** rulesets, parse headliner + supports, emit `/tmp/ph-ingest.json`. **Keep co-billed headliners as a single entity** ("X and Y") rather than auto-splitting `and`/`&` (would break real names like "Amyl and The Sniffers"); list them for a manual split pass afterward.
6. **Dry-run + two QA scans.** `ph batch --env <env> /tmp/ph-ingest.json`. (a) **Artist-skip scan** — check the skip list for fuzzy false-positives (0.6 batch threshold — Casket Cassette / Automatic; pre-create the distinct artist via `POST /admin/artists` so its 1.0 exact match wins). (b) **Headliner sanity scan** — grep kept headliners for leaked presenter/billing tokens: `/presents|present:|featuring| with |aftershow| pass$|hosted by|celebrates| w\/?$/i`. Any hit = the cleaning missed a presenter/theme line (this caught ~20 garbled Empty Bottle entries) → fix the transform and re-run.
7. **Confirm + ingest.** A 0-show or garbage result means the site changed → re-inspect (steps 1–3). Confirm counts are non-zero and plausible, then `--confirm`. Clean up scratch files.
8. **Register + stamp for stale-first refresh** (so the venue auto-joins the `ph sources stale` worklist — this is what makes "add a new venue" one step). Resolve the id, register the calendar URL once, and stamp this run:
   ```bash
   bun run src/entry.ts --env <env> search venue "<name>"                 # -> id
   bun run src/entry.ts --env <env> sources register venue <id> "<events_url>"
   bun run src/entry.ts --env <env> sources refresh venue <id>            # stamp last_refreshed_at
   ```
   Multi-room org (one calendar → several venues): register each venue the ingest created shows for, all pointing at the shared URL, and stamp each. On a **refresh** of an already-registered venue, skip `register` — just `sources refresh`.

Manual fix-ups via API: rename an artist `PATCH /admin/artists/{id} {"name":...}`; re-link a show's artists `PUT /shows/{id} {"artists":[{"id","is_headliner"}]}`; create one `POST /admin/artists {"name":...}` (exact find-or-create); delete an orphan (0-show) artist `DELETE /artists/{id}`. (Admin token required.)

When the calendar didn't expose the venue's own website/socials, or to fill links on the artists it listed names-only, run **[Per-entity link enrichment (follow)](#per-entity-link-enrichment-follow)** after the ingest.

### Reusable transform skeleton + shared rulesets

Each venue's transform = these shared parts + a small venue-specific `cleanAct` and city map. Extend the rulesets per venue (the registry "Notes" column records the deltas).

- **Exclude (non-music / non-show)** — test the raw headliner/title: `private event, karaoke, trivia, bingo, sing-along, dance party, drag, burlesque, pride party|edition, wrestling, comedy, zine fest, *moved to*, ticket-bundle ("…two day pass"), cancelled`. **Keep** `★ Local Show ★` and free/residency *live* shows (First Ave "Free Monday" band nights; Empty Bottle's weekly Hoyle Brothers residency).
- **Clean names** — strip prefixes (`<x> presents:`, `<venue> presents:`, `FREE MONDAY w/`, `Hard Country Honky Tonk with`, `Beyond the Gate featuring`, `… Anniversary with`, `Plantasia - Day N with`, `Special performance by`, album-anniversary `… 20 -`, `… Celebrates …`), strip suffixes (`(Album/Record/EP Release)`, `(… Afterparty)`, `(of <band>)`, `- Lollapalooza Aftershow`, `+more`, leading `*SOLD OUT*` / `*MOVED*`), then **drop pure presenter/theme tokens** (`FREE MONDAY w`, `<x> presents`) and promote the next act to headliner. Comma-split joined acts.

```js
// node: reads <venue>-raw.json [{date, acts:[...]}] -> writes /tmp/ph-ingest.json
const VENUE = {name:'', city:'', state:'', address:'', website:''};        // single-venue; OR a {roomName:[city,state]} map
const reExclude = /private event|karaoke|trivia|sing-?along|dance party|\bdrag\b|burlesque|pride (party|edition)|wrestling|comedy|zine fest|two day pass|\*moved to/i;
const cleanAct = s => s.replace(/^\*[^*]+\*\s*/,'').replace(/^:\s*/,'')
  /* + venue-specific prefix/suffix strips */
  .replace(/\s*\((album release|record release|ep release|of [^)]+|[^)]*afterparty|sold out)\)\s*$/ig,'')
  .replace(/\s*\+\s*more\s*$/i,'').replace(/\s+/g,' ').trim();
const isJunk = s => !s || /\bpresents$|^free monday\s*w\/?$|two day pass$/i.test(s.trim());
// per raw show: const acts = s.acts.flatMap(a=>cleanAct(a).split(/\s*,\s*/)).filter(a=>!isJunk(a));
//   if (reExclude.test(s.acts[0]) || !acts.length) -> exclude
//   headliner=acts[0] {is_headliner:true}; supports=acts.slice(1); venue from map/VENUE
// emit: [...uniqueArtists.map(name=>({entity_type:'artist',name})), {entity_type:'venue',...}, ...shows]
```

### Venue registry

| Venue org | Events URL | Render | Pagination | Room → city/state | Notes |
| --- | --- | --- | --- | --- | --- |
| **First Avenue** (MN) | `https://first-avenue.com/shows/` | JS (WordPress) — browser MCP | GET `?post_type=event&start_date=YYYYMM01` (same-origin `fetch` loop) | First Avenue / 7th St Entry / Fine Line / The Cedar Cultural Center / Orpheum Theatre / Surly Brewing Festival Field / Armory / State Theatre / icehouse MPLS → **Minneapolis, MN**; Turf Club / Palace Theatre / The Fitzgerald Theater / Amsterdam Bar & Hall / Grand Casino Arena → **St. Paul, MN** | Card `.show_list_item`; headliner `.show_name h4`, tour title `.show_name h6`, supports `.show_name h5` (names are the text-nodes between `<em>` connectors), venue `.venue_name`, date `.date .month`+`.day`. **Note the abbreviated name "7th St Entry" dedups to the existing "7th Street Entry".** List view has no per-show price/age. |
| **Empty Bottle** (Chicago, IL) | `https://www.emptybottle.com/` (homepage; `/ebp-events` is empty) | JS (Squarespace + Hive widget) — browser MCP | None — the homepage lists ALL upcoming on one page (~99 cards, year rolls over to next Jan); infer year by month-decrease | Single venue → Chicago, IL (1035 N Western Ave) | Card `.show-details`; date `.date`+`.start-time`, lineup `ul.performing li` (first li = headliner, rest = supports). **Messier than First Avenue — the `li` list mixes in presenter/theme lines** (e.g. "FREE MONDAY w", "X presents", "Empty Bottle and c3 Present:", album-anniversary billings, "- Lollapalooza Aftershow", "+more", "Plantasia TWO DAY PASS"). Strip those prefixes/suffixes, drop presenter/junk lis and promote the real first act, comma-split joined acts, and exclude drag/zine-fest/ticket-bundle/`*MOVED TO*` rows. ALWAYS dry-run and scan for leaked "presents/featuring/with/aftershow/pass" headliners before `--confirm`. |
| **Thalia Hall** + **Tack Room** (Chicago, IL) | `https://www.thaliahallchicago.com/shows` | **Upstream API, not DOM** (2026-06-07, 64 shows) — Squarespace shell injects events via `/s/event-feed-widget.js` | Single Ticketmaster call, `size=200`, one page | Thalia Hall (1807 S Allport St) + the small **Tack Room** (1227 W 18th St) come back as distinct TM venues → both Chicago, IL | **16-on-Center family** (Thalia Hall, Empty Bottle, Promontory, Space, Salt Shed, Three Top, Cahn) all share that widget → the **Ticketmaster Discovery API**. Read the `getEvents()` switch in the widget JS for each venue's venueIds + (public) apikey — some also concat a `promoterId` feed + an Eventbrite `apiEvents` feed, deduped by name. Thalia call: `app.ticketmaster.com/discovery/v2/events.json?size=200&apikey=Mj9g4ZY7tXTmixNb7zMOAP85WPGAfFL8&venueId=rZ7HnEZ17aJq7&venueId=rZ7HnEZ17aJq0&venueId=KovZpZAktlaA&source=ticketweb`. Fields: `_embedded.attractions` ([0]=headliner, pre-split & cased — **but sometimes misses small openers that are only in `name`, and normalizes stylizations** e.g. DeVotchKa→"Devotchka", LITE→"Lite"; parse `name` too & let attraction casing win on merge except known stylizations), `dates.start.localDate/localTime`, `priceRanges[0].min` (emit as a **number** — backend `price` is `*float64`), `url` (ticket link), `classifications` (genre). Filter `dates.status.code==='cancelled'`; exclude comedy/podcast/variety/camp/`TWO DAY PASS`. apikeys are public widget keys → may rotate (re-fetch the JS on 403). |
| **Club Congress** + Plaza Stage (Tucson, AZ) | `https://hotelcongress.com/calendar/` | **Server-rendered** (WordPress; DICE.fm ticketing) — plain `curl` returns all events in the HTML, no JS/API needed (2026-06-07, 33 shows) | None — one page lists all upcoming (~143 cards, ~5 months) | Club Congress (indoor) + **Plaza Stage** (outdoor) → one **Club Congress**, Tucson AZ (311 E Congress St). **The Century Room** (jazz) + **Tiger's Taproom** (open mics) are distinct rooms in the same building → exclude | Each `div.single-event` has schema.org microdata: `id="event-NNNNN"` (dedup key), the **room slug in its CSS class** (`club-congress`/`plaza-stage`/`the-century-room`/`tigers-taproom` → filter rooms by this), `.venue [itemprop=name]` + address, `[itemprop=startDate] content="YYYY-MM-DD HH:MM:SS"`, `<h4 itemprop=name>` title, DICE ticket `<a href>`. **Heavy non-concert programming** — exclude FIFA/World-Cup watch parties, trivia/Jeopardy, paint nights, poetry/open mics, one-woman/theater shows, and **DJ/dance residencies** (Tempo, PURR!, Alchemy, the "No Skip Summer" series, Y2K Perreo, Retromania) + cancelled. Title cleaning: strip flags (`*RESCHEDULED*`), tour suffixes after ` – `/`: `, and series prefixes ("Sunday Sunset Mariachi with X", pipe-split "Congress After Dark with X \| Y \| Z"); some band names sit mid-title (TsuShiMaMiRe) → override. **Run the artist-skip QA scan** — the 0.6 fuzzy matcher conflated "Hippie Death Cult" with the existing "Iguana Death Cult"; pre-create the distinct act via `POST /admin/artists` so its 1.0 match wins. |
| **Schubas + Lincoln Hall** (Audiotree / "LH-ST", Chicago, IL) | `https://lh-st.com/` (homepage lists ALL upcoming) | **Server-rendered (WordPress)** — plain `curl -s -A "Mozilla/5.0"` (no browser MCP needed) | None — one page server-renders every upcoming card (~173 over ~7 months) | `Schubas` + `Schubas (Upstairs)` → **Schubas Tavern**, Chicago, IL (upstairs room merged, not a separate venue); `Lincoln Hall` → **Lincoln Hall**, Chicago, IL; `Sleeping Village` → **Sleeping Village**, Chicago, IL (co-presented one-offs at a non-LH-ST room — set no `website`). | **Cleanest source yet.** Card `div.tessera-show-card` carries `data-venue`, `data-month`, `data-tags`; **the link slug `/shows/MM-DD-YYYY-…/` has the full date incl. year — no year inference**. Presenter is in its own `.event-header` (e.g. "Audiotree Presents") — **separate from artists, so NO presenter-strip on names needed**. Artists in `a > h4.card-title` (first = headliner, rest = supports). Filter non-music two ways: (1) `data-tags` ∈ {`Comedy`,`Wrestling`,`Podcast`}; (2) **untagged event-names-as-artist** the tags miss — `mortified`, `chirp vinyl`/`listening bar`, `friendlyjordies`, `nori is present`, `schubas garden party`, `we rock`/`end-of-camp` (kids camp recital). `windycitycomedy`/`WildChild`/`The Bends` tags are NOT categories — keep (Wild Child, THE BENDS are real bands). Clean suffixes: `(Solo)`, `(of <band>)`, `for Kids`, `<x> Present(s): <title>` → keep the act. **Do NOT auto-split "and"/"&"** (AJ Lee & Blue Summit, Teen Jesus and the Jean Teasers, Dana and Alden are single acts). No price/age/ticket_url in the list (TICKETS button is a dice.fm JS popup). First run 2026-06-07: 173 cards → 148 shows, 25 excluded (17 tag + 8 name). |
| **Sleeping Village** (Chicago, IL) | `https://sleeping-village.com/events/` (WordPress, "built with Plot") | **Upstream Plot CMS API, NOT the DICE widget.** Page embeds a DICE *ticketing* widget, but the events load from a same-origin Plot REST route. Render in browser MCP once to capture it (the DICE widget JS only supplies ticket links). | `GET https://sleeping-village.com/api/plot/v1/listings?currentpage=1&notLoaded=false&listingsPerPage=48&_locale=user` — single call, `maxPages:1` (47 listings / ~5 months); returns a JSON **object keyed `"0".."N"`** (not an array). No auth beyond same-origin. | Single venue → **Chicago, IL** (3734 W. Belmont Ave., 60618). **Set `website`+`instagram` (`@sl33pingvillag3`)** — SV already existed (ID 112) from the LH-ST co-presented ingest *without* a website, so this run **enriches** it (1 venue updated). | Each listing: `title` (billing string), `dateTime` (`<span>Thu, Jun 11 9pm - 1am</span>` — **no year, infer by month-decrease**), `lineup.standard[]` (`{title,time}`), `venue`, `fromPrice` (`"Tickets from $15.00"` → number), `ticket.link` (dice.fm → `ticket_url`), `permalink`. **`lineup.standard` is clean but INCOMPLETE — it drops `w/` openers (sometimes empty entirely) and over-adds (David Bazan *and* Pedro the Lion).** So **parse `title` as the authoritative full lineup**; use `lineup.standard` only to resolve `&`/`and` splits (API lists "The Well"+"Moon Destroys" & "upsammy"+"Valentina Magaletti" separate → split; "Pearl & The Oysters"/"Runner and Bobby" one entry → keep whole). Title cleaning: strip presenter prefixes (`<x> Presents:/Present…`, "Metro/LH-ST/Forever Deaf/Kickstand/Amplified Chicago/Sleeping Village and C3 Presents:"), strip `Official Lollapalooza Aftershow` suffix + `: Nth Anniversary Show` theme + per-act `(solo)`/`(DJ Set)` + "special guests"; split headliner/supports on ` w/ `, each side on `+`,`/`,`,` (NOT &/and — guard via API). **Heavy non-music — exclude** dance parties / DJ residencies (Lover's Lounge, Omar's World, WIGS, The Groove, Umbra Ground, DJ Max + Fingy, Industry Night, Tokyo Disco), book launches/clubs, burlesque (Club Chiffon), expos, talent shows (Dykes Got Talent), readings/markets, plant workshops; **kept** named-artist DJ-set aftershows (Vandelux, Scary Lady Sarah on a band bill). **Normalize curly apostrophes (U+2019)→`'` before the exclusion regex** (else "Omar's"/"Lover's" slip through), and **un-anchor** night-name regex (presenter prefixes precede them, e.g. "Trak Qwn Presents: WIGS"). **Artist-skip QA scan: pre-created "Bellows" via `POST /admin/artists`** (ID 1504) to beat a 71% fuzzy false-match to existing "Belles"; "Mamas Gun"→existing "Mama's Gun" (90%, same UK band) is correct. First run 2026-06-08 (→ stage): 47 listings → 30 shows (29 new + 1 idempotent Seedhe Maut already moved Schubas→SV), 17 excluded. |
| **Metro Baltimore** (Baltimore, MD) | `https://metrobmore.com/events/` (WordPress, "built with Plot") | **Same Plot CMS platform as Sleeping Village** — events come from the same-origin Plot API, not the DICE widget. | `GET https://metrobmore.com/api/plot/v1/listings?currentpage=1&notLoaded=false&listingsPerPage=48&_locale=user` — **plain `curl` works, no nonce/browser needed**; single call, `maxPages:1` (45 listings / ~4 months). | Mostly **Metro Baltimore** (1700 North Charles St, 21201; IG `@metro_baltimore`); **honor the per-listing `e.venue`** — one offsite Metro-promoted show came back as **Holy Frijoles** (also Baltimore, MD; no website). | Same Plot fields + title-authoritative-lineup rule as the Sleeping Village row, plus **Metro deltas**: (1) **headliners are ALL-CAPS in `title`** (supports are proper-case) → title-case the head only (preserve `S.G.`/`AC/DC`-style period/slash tokens, lowercase mid-name `and`/`of`/`the`/`&`). (2) Strip album/tour/anniversary theming from the head: quoted `'…'` segments, `Record Release Show`, `– … Tour`, `N Year Anniversary Show`, `(Early/Late Show)`. (3) **Supports use `" and "` as a list separator** (`"Cataclysmic, Aisle 19 and Muscipula"`) → split supports on `,`/` and `/`/`/`+`, **but KEEP `&`** (band names: `Jeffrey Lewis & The Voltage`, `Conor & the Wild Hunt`, `Ebban & Ephraim Dorsey`). (4) Supports come from `title` (complete); fall back to `lineup.standard` only when there's no ` w/ ` (`PLOSIVS`→Hammered Hulls, `SINCERE ENGINEER`→Stay Inside/Smug). (5) **Per-id overrides** for event-name titles whose acts aren't in the title — `Labor Daze: The Second Dose`→5 bands, `Queer Rodeo 2`→Sparkle Carcass/Goatroper (Goatroper from the description). (6) Normalize stylization `BIG ‡ BRAVE`→`BIG|BRAVE`. **Exclude** Pride/rave/DJ dance parties (Sweet Spot Pride, B-Valley, Metroschock) + student showcase (School of Rock Allstars). **Flag for manual entry:** multi-band DIY reunions whose lineup is only in a **truncated, delimiter-less description** (Charm City Art Space Reunion — ~10 bands, unparseable). **Artist-skip QA: pre-created Demiser / Flesher / Flowerbomb / The Silver** to beat 70–93% fuzzy false-matches to Debaser / Fleshwater / Flowerbabe / The Silver Palms. First run 2026-06-08 (→ stage): 45 listings → 40 shows (39 Metro + 1 Holy Frijoles), 5 excluded; venues 113/114. |
| **Zebulon** (Los Angeles, CA) | `https://zebulon.la/` (homepage embeds a **Dice.fm** widget) | **Skip the JS widget — hit Dice's partners API directly (no browser MCP).** `grep` the raw HTML for `DiceEventListWidget.create({…})`; its init JSON exposes `partnerId`, `apiKey`, `venues[]`, `promoters[]`. | `GET https://partners-endpoint.dice.fm/api/v2/events?page[size]=24&page[number]=N&types=linkout,event&filter[venues][]=…&filter[promoters][]=…` with header `x-api-key:<apiKey>`; loop `page[number]` until `data:[]` is empty (2026-06: 110 events / 5 pages, Jun–Nov). **This pattern works for ANY Dice-powered venue site** — just re-read the widget config for the current key/filters. | Single venue → **Los Angeles, CA** (2478 Fletcher Dr, 90039). Widget config also lists Monty Bar / The Moroccan Lounge but they had 0 current events; each event's true venue is in `.venue` / `.location` — honor it if those rooms reappear. | **Rich JSON, but `artists[]` is UNRELIABLE — it repeatedly drops the headliner (TsuShiMaMiRe, Ak'chamel, Party Dozen, Rearranged Face…) and carries artifacts (`BIG\|BRAVE`, unicode hyphen in `ESCAPE‐ISM`, a stray "sahn").** Treat `name` (the billing string) as the lineup authority and override `artists[]` per-show where they disagree (≈24 of 77 needed it). `date` is **UTC** → convert to venue-local date via `location.place` (`2026-06-09T03:00Z` → `2026-06-08`; backend re-stores local-20:00→UTC, so it round-trips). `url` = the dice ticket link (`ticket_url`). All events are `type_tags:music:gig`, so **filter non-music by `name`**: dance parties / DJ nights / film screenings / sports watch parties / 2-day-pass bundles (≈32 of 110). **33 events have empty `artists[]`** — mostly the non-music ones, but a few are real bands (Mama's Gun) → parse `name`. First run 2026-06-08 (→ stage): 110 events → 77 shows, 33 excluded; **pre-created Al Lover / Dave Harrington / Nick Waterhouse via `POST /admin/artists`** to beat 0.73–0.75 fuzzy false-matches to Ex Lover / Goose Harrington / Suki Waterhouse. |

## Label roster-page ingest

A record label's "Artists" page (e.g. `sacredbonesrecords.com/pages/artists`) is the label analog of a venue's events page: one label plus its roster of artists. Keep it current by re-running every so often — re-runs are idempotent (existing label/artists skip; the label↔artist links are `INSERT … ON CONFLICT DO NOTHING`).

### Inline roster shape (preferred)

Express the whole page as **one** self-contained batch item — a label carrying an `artists` array:

```json
[
  {
    "entity_type": "label",
    "name": "Sacred Bones Records",
    "country": "US",
    "website": "https://sacredbonesrecords.com",
    "artists": [
      {"name": "Anika"},
      {"name": "Amen Dunes"},
      {"name": "Zola Jesus"}
    ]
  }
]
```

The CLI **expands** this into the label item plus one `artist` item per roster entry with `label` injected, then processes labels-before-artists so each artist resolves the just-created label and links to it. Roster entries may be bare name strings (`"Anika"`) or full artist objects (`{name, city, tags, …}`); an explicit per-artist `label` overrides the injected one. The flat form (a `label` item + separate `{artist, label}` items) still works and is equivalent.

**Dry-run reports the link plan** ("Label links: N planned across M label(s)") and **the confirm run reports outcomes** ("Label links: N linked" / "K label-not-found" / "J link-failed") — a failed link is surfaced, no longer a silent warn.

### Workflow

1. **Render the page — try `curl` first, browser MCP only if the names aren't in the HTML.** Render varies by label even on the same platform: **Feel It Records (Shopify) server-renders the whole roster** — plain `curl -s <url> -A "Mozilla/5.0"` returns all 86 names — while **Sacred Bones (also Shopify) injects the grid via JS** and needs the browser. So `curl` the page and `grep` for the artist names/links first; only if they're absent fall back to the browser MCP / `agent-browser` (`wait --load networkidle`) and extract from the live DOM. Either way, **inspect the actual markup each run** (the names may live in card text, an `alt` attribute, a link slug, or all three) — don't hardcode a selector blindly.
2. **Honor the page's own sections — don't guess music vs non-music.** Many labels list visual artists / authors / photographers alongside musicians (Sacred Bones has an `Artists` + `Alumni` music roster and a `Books` section of 12 visual artists). Filter by the page's section headings, not by guessing per name. Exclude non-music sections; keep the music sections.
3. **Build the batch programmatically** (never hand-transcribe a 130-name roster). Emit one label item with the inline `artists` array. Cross-check display names against their slugs — source display text occasionally has typos the slug disproves (`childrens-hospital` slug vs. a displayed "Chidren's Hospital"); trust the slug. Preserve exact spelling/unicode otherwise (`Vår`, `SQÜRL`, `Föllakzoid`).
   - **Capture per-artist Bandcamp/social when the card exposes it — high value, it renders a playable embed.** Inspect what each card *links to*: Feel It's cards link to each band's Bandcamp (the album link's subdomain is the artist's Bandcamp root → set `bandcamp: "https://<slug>.bandcamp.com"`), whereas Sacred Bones' cards link to *internal* `/collections/<slug>` label pages (nothing external to capture — names only). Set the **bare `bandcamp`/`spotify`/`instagram`** fields (these match the create payload), not `bandcamp_url`. Re-ingest now enriches **existing** artists' links too (PSY-1171, PR #1202), so this fills the link whether the artist is new or already exists.
4. **Keep collaboration names un-split.** "Boris & Uniform", "Marissa Nadler & Stephen Brodsky" are distinct release-credit entities — do NOT auto-split on `&`/`and`/`,` (would shatter real names).
5. **Dry-run + the artist-skip QA scan (MANDATORY for label rosters — collision rate is high)** — `bun run src/entry.ts --env <env> batch /tmp/ph-ingest.json`. Map every SKIP back to the proposed name (the skip header shows the *existing* matched entity): the 0.6 fuzzy threshold conflates distinct artists, and short/common punk band names collide heavily — **Feel It hit 4 false matches in 7 skips (Fan Club→"Yot Club", It Thing→"Nothing", Spllit→"Split", Vacation→"Medication")**, on top of Sacred Bones' (**Institute→"Prostitute", Lathe of Heaven→"Left of Heaven", Cheena→"Cheem"**, plus a collab→member, **Emma Ruth Rundle & Thou→"Emma Ruth Rundle"**). A wrong skip also links the *wrong* existing artist to the label. **Fix:** pre-create each distinct artist via `POST /admin/artists` (exact find-or-create) so its 1.0 match wins, then re-run. Re-ingest now enriches an *existing* artist's links (PSY-1171, PR #1202), so the batch fills the `bandcamp`/social on that same run — a name-only pre-create no longer strands the band (you can still pass the fields on create, or `PATCH /admin/artists/{id} {"bandcamp":…}` directly, if you prefer). Do NOT pre-create via `ph submit artist` — that fuzzy-matches too.
6. **Confirm + verify** — `batch --confirm`, then verify against the API, not the CLI's self-report: `GET /labels/search?q=<name>` → id, then `GET /labels/{id}/artists` and check the **`count`** field equals the roster size (the roster response is `{artists, count}` — there is no `total` field, and it takes no `limit` param). The label's website is stored under `social.website` in the detail response (top-level `website` in *search* results is a separate projection and may read null). **To verify per-artist enrichment (Bandcamp/social), hit `GET /artists/{id}` — NOT `GET /labels/{id}/artists`:** the roster *list projection* omits the `social`/`bandcamp` block, so it reads a false 0/N even when every artist has a link (Feel It read 0/86 on the list endpoint, 86/86 on the per-artist detail endpoint).

7. **Stamp the source registry** (Catalog Refresh, PSY-1146) — so stale-first refresh knows this roster was just pulled. After a successful confirm, register the source (once) and stamp the refresh, using the label/venue id from step 6:
   ```bash
   bun run src/entry.ts --env <env> sources register label <label_id> "<roster_url>"
   bun run src/entry.ts --env <env> sources refresh label <label_id>
   ```
   To decide **what to refresh next**, list the stalest sources first: `ph sources stale --limit 20` (never-refreshed sort first). Venues use `sources register venue <venue_id> "<calendar_url>"` the same way.

8. **Fill names-only entries (optional follow)** — roster entries whose cards exposed no external link (e.g. Sacred Bones' internal `/collections/` links) land without a Bandcamp/social. Enrich them via **[Per-entity link enrichment (follow)](#per-entity-link-enrichment-follow)** below.

> **Re-ingest now enriches an existing artist's (and venue's) social links — fixed in PSY-1171 (PR #1202).** The old limitation (two field-name mismatches in `cli/src/lib/duplicates.ts`: `ARTIST_FIELDS` read `bandcamp_url`/`spotify_url`/`instagram_url`, and `searchArtists()` read a non-existent top-level `bandcamp_url` instead of the nested `social.bandcamp`) is resolved: `ARTIST_FIELDS` now uses the canonical bare names and `searchArtists`/`searchVenues` flatten the link fields from the response's nested `social`. So an existing artist re-ingested with a `bandcamp`/`spotify`/`instagram`/etc. now gets it filled — no manual `PATCH` required. **Still deferred (PSY-1179):** label socials beyond `website`/`bandcamp`, and venue `address`/`zipcode`/`capacity`, are not yet enriched on re-ingest (the label list response needs widening, venue `address`/`zipcode` need verified-gating since the API redacts them for unverified venues, and `capacity` has no backend column yet).

### Label registry

| Label | Roster URL | Render | Sections (music kept / excluded) | Notes |
| --- | --- | --- | --- | --- |
| **Sacred Bones Records** | `https://www.sacredbonesrecords.com/pages/artists` | JS (Shopify) — browser MCP; artists are `/collections/<slug>` links under `#MainContent` | **Artists (80) + Alumni (50) = 130 kept**; **Books (12) excluded** (visual artists/authors: Peter Beste, Jesse Draxler, …) | First run 2026-06-20 → **stage**: label id 1 + 130 linked. Pre-created Institute / Lathe of Heaven / Cheena / Emma Ruth Rundle & Thou to beat 0.6 fuzzy false-matches. 2 source typos fixed via slug (Children's Hospital, Daily Void). `release_count` 0 (roster page has no release data). |
| **Feel It Records** | `https://www.feelitrecordshop.com/pages/artists` | **Server-rendered (Shopify)** — plain `curl -A "Mozilla/5.0"` returns the whole roster (no browser MCP). Roster is a single `<div class="artist_wrapper">` of `.artist_container` cards; per card the name is the text after `<br>` (also in `<a alt>`/`<img alt>` — cross-check, they agreed 86/86) and the `href` is a Bandcamp **album** link whose **subdomain is the artist's Bandcamp root** (`https://<slug>.bandcamp.com`). | **86 kept (single section, all music)**; no Alumni/Books/visual-artist section to exclude. | First run 2026-06-21 → **stage**: label id 2 (Cincinnati, OH — verify location from `/pages/hours-and-location`, the label relocated from Richmond, VA) + 86 linked, **each artist carrying its Bandcamp root** so the playable Bandcamp section renders. Pre-created Fan Club / It Thing / Spllit / Vacation (with bandcamp) to beat 0.6 fuzzy false-matches to Yot Club / Nothing / Split / Medication, and PATCHed bandcamp onto real dups Artificial Go (481) / Man-Eaters (1105) / Sweeping Promises (351) — at the time, the batch couldn't enrich existing artists' social links (since fixed: PSY-1171 / PR #1202). Keep `and`/`/` names whole (Fashion Pimps and the Glamazons, Green/Blue); "The Cowboy" (Cleveland, thecowboycle) ≠ "The Cowboys" (thecowboysnow) — distinct, the Bandcamp subdomain disambiguates. **Verify Bandcamp via `GET /artists/{id}` detail, NOT `GET /labels/{id}/artists`** — the roster *list projection* omits `social`/`bandcamp` (reads 0/86 falsely); detail confirmed 86/86. |
| **12XU** | `https://12xurecs.bandcamp.com/` (the label's **Bandcamp hub** — its root IS the "Artists \| 12XU" grid; the label's own site `12xu.net` is a WordPress blog, not a clean roster) | **Server-rendered (Bandcamp)** — plain `curl -A "Mozilla/5.0"`. **NEW source type: a Bandcamp *label* hub.** The `data-blob` only carries `{label_name, artist_grid:bool}` (no list) → parse the DOM grid: each `<li class='artists-grid-item'>` has `<a href='https://<sub>.bandcamp.com?…'>` (→ artist Bandcamp root, strip the `?` query), `<div class="artists-grid-name">` (name), `<div class="artists-grid-location secondaryText">` (City, State/Country). | **59 kept (single grid, all music)**; no non-music section. | First run 2026-06-21 → **stage**: label id 3 (Austin, TX — *inferred*: owner Gerard Cosloy's BC location + roster plurality; 12xu.net doesn't state it) + 59 linked, each with its `<x>12xu.bandcamp.com` root. **Captured per-artist city/state from the grid location** (US → 2-letter state via a full-name map; international → city only, no state, no locale tag; bare-country → neither). **Multi-label win:** Uniform (1717, already on Sacred Bones) also linked to 12XU — the cross-label graph enrichment. Pre-created chimers / Love Child / Rocket 808 (with bandcamp + location) to beat 0.6 fuzzy false-matches to Chambers / Wild Child / Rocket; PATCHed bandcamp onto existing exact-dups Uniform (1717) / The Sleeves (1492) — at the time, the batch couldn't enrich existing artists (since fixed: PSY-1171 / PR #1202). Keep `&`/`/` names whole (Ed Kuepper & Jim White, Blank Hellscape / Wolf Eyes, USA/Mexico, John Schooley & Walter Daniels). **No tags this run** — tag endpoints are rate-limited and PSY-1173 (the phk_ bypass) wasn't confirmed deployed to stage yet. |

## Label release-pass (Bandcamp discography → releases)

After a label roster is ingested (artists + Bandcamp links), the **release pass** pulls each artist's Bandcamp discography into `release` entities — title, year, type, a playable Bandcamp `external_link`, and genre/locale tags. Proven on **12XU (133 releases, 2026-06-21)** and **Feel It (407, 2026-06-22)** → stage. The artists must already exist (releases resolve by name), so run this *after* the roster ingest.

**Gate — tags hit rate-limited endpoints.** Tag create/apply is limited (20/hour create, 30/min vote). Bulk tagging needs **PSY-1173** (the `phk_` admin-token rate-limit bypass) live on the **target env**. Confirm it's deployed there first (35 rapid tag-adds with no 429); without it, the run applies releases fine but every tag 429s. Newly-created artists' release data (title/year/type/links) is unaffected — only tags need the bypass.

### Per-artist extraction (the parser that matters)

1. **Discography = the `#music-grid` ONLY, not raw hrefs.** Fetch `<artist-bandcamp>/music` (follow redirects). Parse the `<ol id="music-grid">` `<li>` items' `/album/` + `/track/` hrefs — those are the real releases. **Do NOT regex `/album|/track` over the whole page** — a single-release `*12xu`-style page *redirects* `/music` to the album, whose **tracklist** then explodes into N bogus "singles" (Mope Grooves → 27 fake tracks). When there's **no** `#music-grid`, `/music` redirected to one album → take that single URL (`response.geturl()` / `og:url`).
2. **Release fields from JSON-LD.** Each release page's `<script type="application/ld+json">` `MusicAlbum`/`MusicRecording`: `name`→title, `datePublished`→year (regex a 4-digit year), `numTracks`, `keywords`→tags.
3. **`release_type` heuristic** (track-count, documented & accepted as a heuristic): a `/track/` URL or ≤2 tracks → `single`, 3–6 → `ep`, 7+ → `lp`.
4. **Artist name must match the stage entity exactly** — releases link by name. Roster display names that got deduped/normalized on ingest will MISS (Feel It "Man Eaters" vs stage "Man-Eaters"). Dry-run, grep `Unresolved artists:`, patch those names to the exact stage form, re-dry-run to 0 unresolved.

### Multi-label is NOT extractable from Bandcamp

Release pages expose only the Bandcamp **account name** (e.g. "Mope Grooves"), never the record label — so per-release label detection doesn't work; don't try. Cross-label graph enrichment happens at the **artist level via roster ingests**: ingesting a second label's roster auto-cross-links a shared artist (Uniform → Sacred Bones + 12XU). That's the mechanism.

### Tags: allowlist-clean + emergent-tag promotion loop

Map each keyword: normalize variants first, then `genre` (allowlist) / `locale` (city/country map), else **drop but COUNT**. The drop-counter is the feature — it surfaces high-value keywords to promote so the allowlist grows over time:

1. **Cache the raw keywords** per release (`/tmp/releases-raw.json`) so re-cleaning needs no re-fetch.
2. **Report** the top non-allowlist keywords by frequency.
3. **Promote** the high-value ones (ask the user): clear genres → the genre set; cities → the locale map. Drop noise (lyric words, band in-jokes, regions/states/countries already covered by a city).
4. **Rebuild from cache** with the expanded allowlist (instant). Feel It: 405→**407/407 tagged**, most releases 4–6 tags, after promoting 8 genres + 14 cities.

**The durable allowlist** (extend it as keywords emerge — this is the persisted source of truth):

```python
# normalize variants -> canonical BEFORE matching (hyphen/space/apostrophe drift)
NORM = {"post punk":"post-punk","powerpop":"power pop","power-pop":"power pop",
 "rock n roll":"rock'n'roll","rock and roll":"rock'n'roll","rock n' roll":"rock'n'roll",
 "lofi":"lo-fi","altpop":"alt-pop","hip hop":"hip-hop","synthpop":"synth pop",
 "art-punk":"art punk","proto punk":"proto-punk","garage-rock":"garage rock"}
GENRES = {  # genre tags (default category)
 "punk","post-punk","hardcore punk","garage punk","hardcore","garage","garage rock",
 "coldwave","new wave","no wave","synthwave","synthpop","synth pop","synth-punk","egg punk",
 "art punk","power pop","proto-punk","oi","street punk","d-beat","crust","post-hardcore",
 "goth","gothic","darkwave","minimal synth","ebm","industrial","noise rock","noise","drone",
 "psychedelic","psych","psych rock","psychedelic rock","electronic","experimental","ambient",
 "folk","gospel","funk","disco","avant-garde","hip-hop","jazz","metal","alternative","rock",
 "rock & roll","rock'n'roll","pop","surf","dub","krautrock","shoegaze","dream pop","country",
 "soul","blues","indie","indie rock","lo-fi","dance","techno","house","improv","free jazz",
 # promoted 2026-06-22 (Feel It report): broad-but-genre here given the punk/garage rosters
 "synth","glam","britpop","punk rock","art rock","skate punk","alt-pop","alt-psych"}
LOCALES = {  # keyword -> locale-tag (category=locale)
 "australia":"Australian","netherlands":"Dutch","finland":"Finnish","uk":"British",
 "england":"British","canada":"Canadian","germany":"German","france":"French","italy":"Italian",
 "spain":"Spanish","japan":"Japanese","ireland":"Irish",
 # cities promoted 2026-06-22 (origin/scene): high-value for browsing
 "cincinnati":"Cincinnati","richmond":"Richmond","detroit":"Detroit","cleveland":"Cleveland",
 "melbourne":"Melbourne","minneapolis":"Minneapolis","seattle":"Seattle","las vegas":"Las Vegas",
 "bloomington":"Bloomington","chicago":"Chicago","kansas city":"Kansas City","memphis":"Memphis",
 "kyiv":"Kyiv","baltimore":"Baltimore"}
# Next-tier candidates seen but NOT yet promoted (low-freq tail): kiwi pop, grungepop, cello-rock,
# noise-pop, indie pop, kbd; cities London/Brooklyn/Olympia/Charlottesville/Leipzig. Promote on demand.
```

### Workflow

1. **Re-derive the roster's Bandcamp roots** (the `/labels/{id}/artists` list projection omits `bandcamp`) — re-parse the label's roster/hub page (or read each artist's detail).
2. **Sample-validate the parser on 3–5 artists FIRST** (mix a single-release `*label` page + an own-domain page) before the full run — this caught the track-explosion bug. Confirm no 0-release artists, no absurd counts.
3. **Extract all** → cache raw + write the batch + print the dropped-keyword report.
4. **Promote tags** (the loop above), rebuild from cache.
5. **Dry-run on the env** — fix `Unresolved artists:` name mismatches → 0 unresolved.
6. **Confirm** (PSY-1173 must be live for tags). Big runs background; verify counts = 0 rate-limit / 0 tag-failures.
7. **Verify** via `GET /artists/{id}/releases` + `GET /entities/release/{id}/tags` (the `/releases` list endpoint is first-page-only; the release-detail `tags` field is a filtered projection — use the entity-tags endpoint).

## Per-entity link enrichment (follow)

Roster / calendar ingests often land entities **names-only** — Sacred Bones' cards link to internal pages (no Bandcamp), and a venue calendar rarely shows the venue's own socials. This pass fills the missing external links on **already-ingested artists and venues** by web-researching each name, verifying the match, and PATCHing the confirmed links. It's the agent's job — there is **no backend "name → link" discovery** (the enrichment service does only dedup + MusicBrainz MBID + SeatGeek); write-back uses the existing admin PATCH endpoints. Like every ingest step: **dry-run → confirm**, on-demand, **never guess**.

### Invoking (what you type)

- **Tail of an ingest:** after a roster/calendar confirm — "…then enrich the names-only entries' links."
- **Standalone:** `/ingest <env> — enrich the N artists missing links on label <id>` (or `venue <id>`, or "…on the streaming worklist").

### Work-source (which entities — capped at N/run)

Pick **N ≈ 10–15 per run** (like `ph sources stale --limit N`) — web research is the cost, so bound it.
- **Just-ingested:** the entities you created this run that had no link — already in hand, no lookup needed.
- **A label/venue source:** `GET /labels/{id}/artists` → for each, `GET /artists/{id}` **detail** and keep those whose `social.*` are empty (the roster *list* omits `social`, so you MUST read detail — the same verify-via-detail rule as PSY-1171). The venue itself: `GET /venues/{id}`.
- **Show artists:** `GET /admin/streaming-worklist` is a ready-made queue — artists with upcoming shows + a non-terminal `streaming_discovery_status`.

### Find the links (agent web-research, per entity)

Capture **full on-platform URLs** (same host rules as Step 1 "Social links"; the backend rejects bare handles):
- **artist:** Bandcamp first (highest value — it renders a playable embed), then Spotify (`open.spotify.com/artist/…`), Instagram, official website.
- **venue:** official website + Instagram (search `"<name>" <city> venue`).

### Verify before applying (skip ambiguous — a wrong link is worse than a missing one)

Apply a link only when the **name matches AND a second signal corroborates**:
- **artist:** genre / hometown / a release or label that fits what we already know — the roster's label is a strong prior (a band on Sacred Bones' Bandcamp belongs to that scene). Same-name collisions are rife — the dedup section's "Fan Club → Yot Club" lesson applies to web search too.
- **venue:** city **plus** a second independent signal — the venue's own site stating city+state, capacity/booking details, or a cross-link from the calendar source. Note `address`/`zipcode` are redacted for **unverified** venues (see the PSY-1179 note above), so they're usually unavailable here — **city alone does NOT clear the bar** for a common/ambiguous name ("The Echo", "Lincoln Hall", "The Independent"); SKIP it.

**Open the candidate page and confirm the corroborating signal *on that page* before applying — a search-result snippet is not corroboration** (snippets are exactly where same-name acts/venues look identical). Can't corroborate → **SKIP**, leave it for manual review. When the entity came from the streaming worklist (worklist-sourced artists **only** — roster/venue-sourced entities have no worklist row, so don't POST a status for them), record the outcome either way: `POST /admin/artists/{id}/streaming-discovery-status` → `linked` or `no_links_found`.

### Dry-run → confirm → PATCH

1. **Preview** per entity: the links found + the corroborating evidence, marking new vs already-set. Pause for the user's OK (exactly like a batch dry-run).
2. **On confirm**, PATCH only the confirmed links:
   - **artist:** `PATCH /admin/artists/{id}` (admin; all 8 social fields in one body, as profile URLs — same as the roster batch, which is what makes the Bandcamp section render).
   - **venue:** `PUT /venues/{id}` (admin-gated; partial body of `website`/`instagram`/… — same endpoint the batch uses).
3. **Verify** via `GET /artists/{id}` / `GET /venues/{id}` **detail** (the list/roster projections omit `social`).

> **Why a targeted PATCH, not a re-ingest batch?** The batch enriches links on re-ingest now (PSY-1171), but only for fields the *source page* supplied. This follow exists for names-only sources — the links come from web research, so a per-confirmed-entity PATCH is cleaner than hand-populating a batch.

## Label discography-page ingest (catalog list → label + artists + releases)

Some labels — especially **defunct** ones — publish a flat **discography page** (a catalogue list) rather than a Bandcamp roster. Parse it into a label + its artists + its releases directly (no Bandcamp). Proven on **Creation Records** (`creation-records.com/discography/`, 104 artists + 325 single releases, 2026-06-22).

### Parser

- **Format is usually `CAT – Artist – Title`**, one per line, `<br>`-separated inside a `<p>` (Creation: `CRE001 – The Legend! – '73 in '83`). Split on `<br>`, then on ` – ` (en-dash `&#8211;`) → `cat / artist / title` (rejoin parts[2:] so titles keep their own dashes).
- **Parse the BODY copy, NOT the `<head>` JSON-LD/meta copy.** WordPress/SEO plugins embed a *truncated, run-together* (no `<br>`) copy of the list in a `<head>` `og:description`/schema block — matching that first gives you 1 giant "release." Anchor on the body (`h[h.rfind('</head>'):]`).
- **Exclude placeholder catalogue numbers** — Creation had 8 `CRE### – Not used` rows (allocated-but-unused). Drop any artist/title == "Not used" (and a trailing "THE END" terminator).

### Fields + decisions

- **`release_type`**: infer from the page's framing. Creation's `CRE` page is its **singles** catalogue (albums use `CRELP`, a different page) → default `single`. Then **re-type EP-titled releases** (`\bEP\b` in the title → `ep`; caught 18: Glider EP, Ride EP, Tremolo (EP)…) via `PUT /releases/{id} {"release_type":"ep"}` (partial update — pointer fields, not PATCH).
- **Catalogue number IS stored** (PSY-1183) — set `labels: ["<label name>"]` + `catalog_number: "<CAT>"` (e.g. `CRE001`) on each `release` item. It's persisted on the release↔label association (`release_labels.catalog_number`), so it only applies when the release names a **single** label (a multi-label release with one flat `catalog_number` is ambiguous → the CLI drops it with a warning). Store the **raw** `CAT` string, no prefix/number parsing. The `CAT` still doubles as the dedup/order key.
  - **Write-once.** The link POST is `ON CONFLICT DO NOTHING` on the `(release_id, label_id)` PK (`backend/internal/services/catalog/label.go` `AddReleaseToLabel`) — the number lands only when the association is **first** created. Re-linking an **already-linked** release leaves its existing `catalog_number` untouched (there is no CLI/API path to *update* one). So this **backfills** cleanly for releases not yet linked to the label, but cannot *correct* a number already set — to change one you'd touch the association directly.
- **No year stored** — the page rarely lists years.
- **Label metadata** from known history, flagged as such (Creation = London, UK — owner/era well-documented, not on the page). **Don't set `website` to a fan-archive URL** (creation-records.com is a fan site, not the defunct label's official site).
- **Collaborations kept whole** (`Nikki Sudden and Rowland Howard`, `Primal Scream, Irvine Welsh & On-U-Sound System`). **Same person, different credit** (Creation's `Ed Ball` / `Edward Ball`) kept as the page credits them — flag for a manual merge.

### Workflow + gotchas

1. Parse the body list → unique artists + releases; build a label item with the inline `artists` roster + one `release` item per row (`artists:[{name}]`, `release_type`, `labels:["<label name>"]`, `catalog_number:"<CAT>"`).
2. **Artist-skip QA scan is CRITICAL here** — a famous-label roster collides hard at the 0.6 fuzzy threshold. Creation hit **5 false matches** (BMX Bandits→"RX Bandits", Momus→"Momma", Phil Wilson→"Ric Wilson", Silverfish→"Silverada", William→"WILLIS"); pre-create each distinct act via `POST /admin/artists`. Exact-name matches to bare existing stubs (Sugar, Swervedriver, 18 Wheeler) are kept + linked — flag generic ones.
3. Dry-run: the **"N unresolved releases" is the usual artifact** (new artists aren't created until `--confirm`); on confirm they resolve. Confirm → verify roster `count` + a few `GET /artists/{id}/releases`.
4. **Don't register a defunct label as a refresh source** — its discography is frozen; nothing to re-scrape.

> **⚠️ Release re-runs are NOT idempotent until PSY-1184 is DEPLOYED to the target env.** The CLI's release dedup (`searchReleases`) was **first-page-only** — re-running any release batch against a large dataset (Creation/Feel It/12XU) classifies existing releases as CREATE and **duplicates** them. PSY-1184 (PR #1210) fixed this but is **merge ≠ deploy** — confirm #1210 is live on the target env before any re-run (incl. a `catalog_number` backfill). Until then, never re-run a release batch to "enrich"; apply tags directly to existing release ids (`POST /entities/release/{id}/tags`), matching by artist + normalized title.

### Label registry (discography pages)

| Label | Discography URL | Render | Notes |
| --- | --- | --- | --- |
| **Creation Records** | `https://creation-records.com/discography/` (fan archive) | Server-rendered (WordPress) — plain `curl`; the list is a `<br>`-separated `<p>` (parse the BODY, not the truncated `<head>` meta copy) | First run 2026-06-22 → **stage**: label id 4 (London, UK — from known history) + **104 artists + 325 singles**. The `CRE` catalogue = singles (albums are `CRELP`, not on this page). 8 "Not used" placeholders excluded; 18 EP-titled re-typed `single`→`ep`. Pre-created BMX Bandits / Momus / Phil Wilson / Silverfish / William to beat 0.6 fuzzy false-matches. Ed Ball/Edward Ball kept as 2 (same person, source credits both). No year. Catalogue numbers NOT yet backfilled — the first run predated PSY-1183 (`catalog_number` wiring) + emitted no per-release `labels`, so no `release_labels` rows exist for these releases. Backfill needs PSY-1184 (release dedup) deployed to stage so re-ingest skips-then-links; re-run with `labels`+`catalog_number` once #1210 is live. The number lands because that first run left the releases unlinked (write-once `ON CONFLICT DO NOTHING` — see the catalogue-number note above). Not registered for refresh (defunct = frozen). |

## Stale-first global refresh (Catalog Refresh)

Once sources are registered in the source-config registry (PSY-1149), refresh the **stalest first** instead of by hand. The registry tracks one source per catalog entity (a venue's calendar page or a label's roster page) plus `last_refreshed_at`; the loop below is the operator/agent workflow that ties the per-source ingest workflows together.

### Invoking (what you type)

This is **manual + on-demand** — run it as time allows (daily, every other day, whenever); nothing is scheduled. One line:

> `/ingest <env> — refresh the N stalest registered sources` (e.g. `/ingest stage — refresh the 5 stalest sources`).

The agent then runs the loop below — `ph sources stale --limit N` → for each row, the matching ingest workflow (venue events-page / label roster-page) using the row's `SOURCE URL` → `ph sources refresh <type> <id>` to stamp. You pick `N` and the cadence; pause for your OK at each source's dry-run before `--confirm`, exactly like a normal ingest.

### The loop

```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli
# 1. What's stalest? (never-refreshed sort first; excludes circuit-broken sources)
bun run src/entry.ts --env <env> sources stale --limit 20 --max-failures 5
```

For each row returned (it shows `TYPE ID LAST-REFRESHED FAILS SOURCE-URL`):

2. **Run the matching ingest workflow** already documented above, using the row's `SOURCE URL`:
   - `TYPE=venue` → the **Venue events-page periodic ingest** workflow (registry table for render/pagination hints).
   - `TYPE=label` → the **Label roster-page ingest** workflow.
   Always dry-run + the QA scans, get OK, then `--confirm` (re-runs are idempotent — unchanged entities skip cleanly thanks to the PSY-1157 dedup fix).
3. **Stamp the refresh** so the source drops to the bottom of the stale list:
   ```bash
   bun run src/entry.ts --env <env> sources refresh <venue|label> <id>
   # on a failed/abandoned run instead, record it so the circuit breaker can engage:
   bun run src/entry.ts --env <env> sources failure <venue|label> <id>   # (admin API; see note)
   ```
   Repeat until the stalest `last_refreshed_at` is recent enough for your cadence.

### Seeding the registry

The loop only sees **registered** sources, so seed them once (then refreshes keep them current):

```bash
# Resolve the entity id, then register its source URL.
bun run src/entry.ts --env <env> search venue "Empty Bottle"     # -> id
bun run src/entry.ts --env <env> sources register venue <id> "https://www.emptybottle.com/"
bun run src/entry.ts --env <env> sources register label <id> "https://www.sacredbonesrecords.com/pages/artists"
```

Seed venues from the **Venue registry** table above (each row's events URL) and labels from the **Label registry** table. Registering is idempotent (upsert on `entity_type`+`entity_id`) and does NOT reset `last_refreshed_at`, so re-running `register` to update a URL is safe.

**Multi-room venue orgs:** one calendar URL often covers several venue entities (First Avenue's MN rooms; Schubas + Lincoln Hall both under `lh-st.com`). Register **one source row per distinct venue that the ingest creates shows for**, all pointing at the shared calendar URL, and stamp each after a refresh. (Stage seed, 2026-06-21: registered First Avenue/94, Empty Bottle/14, Thalia Hall/107, Club Congress/109, Schubas/110, Lincoln Hall/111, Sleeping Village/112, Metro Baltimore/113, Zebulon/43 + Sacred Bones label/1.)

> `sources failure` is exposed by the admin API (`POST /admin/sources/failure`) but not yet a `ph` subcommand — call it via `curl` if needed, or just rely on `register`/`refresh` for the manual loop.

## Individual Commands Reference

```bash
# Search before creating
bun run src/entry.ts search artist "name"
bun run src/entry.ts search venue "name"
bun run src/entry.ts search release "title"
bun run src/entry.ts search label "name"

# Submit single entities
bun run src/entry.ts submit artist '[{"name": "...", "tags": ["punk"]}]'
bun run src/entry.ts submit venue '[{"name": "...", "city": "...", "state": "..."}]'
bun run src/entry.ts submit show '[{"event_date": "2026-04-15", ...}]'
bun run src/entry.ts submit release '[{"title": "...", "artists": [...]}]'
bun run src/entry.ts submit label '[{"name": "..."}]'
bun run src/entry.ts submit festival '[{"name": "...", "series_slug": "...", ...}]'

# All commands support --confirm (default is dry-run)
# All commands accept JSON as argument or piped via stdin

# Source-config registry (stale-first refresh; entity-type is venue|label)
bun run src/entry.ts sources stale --limit 20            # stalest sources first
bun run src/entry.ts sources register label 1 "https://..."  # register/update a source
bun run src/entry.ts sources refresh label 1             # stamp a successful refresh
```
