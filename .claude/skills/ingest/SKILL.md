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

#### @handle extraction (Instagram / social)

Instagram posts contain @handles for artists and venues in captions, tags, and image text. Extract these and map them to Instagram URLs:

- `@la_witch` → `"instagram": "https://instagram.com/la_witch"`
- `@sidthecatauditorium` → `"instagram": "https://instagram.com/sidthecatauditorium"`

Set the `instagram` field on artist and venue batch items when a handle is identified. Only include handles that clearly correspond to an artist or venue entity being created. Example:

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

**artist**: `name` (required), `city`, `state`, `instagram`, `bandcamp`, `spotify`, `website`, `tags`, `label` (name of a label to link this artist to — resolved by exact name after create; links via `POST /admin/labels/{id}/artists`)
**venue**: `name` (required), `city` (required), `state` (required), `address`, `instagram`, `website`, `tags`
**show**: `event_date` (required, YYYY-MM-DD), `city` (required), `state` (required), `title`, `price`, `ticket_url` (URL for ticket purchase -- extract from flyers when visible), `artists` (required, array of `{name, is_headliner?}`), `venues` (required, array of `{name, city, state}`)
**release**: `title` (required), `release_type` (lp/ep/single/compilation/live/remix/demo), `release_year`, `artists` (required), `external_links` ([{platform, url}]), `tags`
**label**: `name` (required), `city`, `state`, `country`, `website`, `bandcamp`, `tags`, `artists` (optional inline roster — see "Label roster-page ingest" below). Use `bandcamp` (the canonical field); the legacy `bandcamp_url` alias is accepted but normalized to `bandcamp` before submit.
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

- **Add a new venue:** `/ingest <env> — add a new venue from its events page: <URL>. Run the venue events-page workflow: extract all upcoming months, music concerts only, correct city/state, dry-run + both QA scans, pause for my OK, then write and add a registry row.`
- **Refresh an existing venue:** `/ingest <env> — refresh <venue name>'s listings using its registry row below. Re-scrape all upcoming months, dry-run (idempotent → only new shows), both QA scans, my OK, then write.`

Either way: always dry-run + the two QA scans (step 6) and get explicit confirmation before `--confirm`.

### Workflow

1. **First, look for an upstream data API — it usually beats scraping the DOM.** Many venue sites inject events client-side from a JSON API via a widget script. `curl` the page; if the shows aren't in the server HTML, `grep` it for `<script src=…>`, fetch that JS, and `grep` *it* for `fetch(` / `ajax` / API hostnames. **If it calls a structured API (Ticketmaster Discovery, SeeTickets, DICE, a WordPress REST route…), hit that JSON directly** — it already has clean artist entities, dates, prices, and ticket URLs, so you skip DOM parsing, pagination, and year-inference entirely (see the Thalia Hall / 16-on-Center registry row). Then **cross-check**: render the page in the browser MCP and confirm the visible card count matches the API result 1:1 (catches a second feed or a filter the API view misses).
2. **Otherwise, render + scrape.** Server-rendered page → `curl -s <url> -A "Mozilla/5.0"`. JS-rendered/paginated → browser MCP (`chrome-devtools`): `new_page(url)` then `evaluate_script`. (First Avenue is JS — current month is server-rendered, later months load via JS.)
3. **Discover structure once** (scrape path only). One `evaluate_script` to find the show-card selector + sub-fields (headliner / supports / venue / date) and the pagination mechanism (URL param vs. a "next" control). Look at the rendered DOM, don't assume.
4. **Extract all pages.** If pagination is a URL param, loop same-origin `fetch()` inside one `evaluate_script` (no CORS), `DOMParser` each response, accumulate. Else click the next control and re-extract per page. Stop after 2 consecutive empty pages. **Dates often lack a year** (Empty Bottle) — infer it: start at the current year and bump it whenever a card's month number drops below the previous card's (Dec→Jan rollover). Save raw output via `evaluate_script` `filePath` to a **workspace-internal scratch path you will delete afterward** (`/tmp` is rejected by the MCP; repo root works if you `rm` it, or use a gitignored dir). **Scrub all scratch files before finishing** so nothing lands in a commit.
5. **Transform programmatically** (never hand-transcribe hundreds of rows) — start from the **skeleton + shared rulesets below**. Apply the room→city/state map, the shared music-only **exclusion** + **headliner-cleaning** rulesets, parse headliner + supports, emit `/tmp/ph-ingest.json`. **Keep co-billed headliners as a single entity** ("X and Y") rather than auto-splitting `and`/`&` (would break real names like "Amyl and The Sniffers"); list them for a manual split pass afterward.
6. **Dry-run + two QA scans.** `ph batch --env <env> /tmp/ph-ingest.json`. (a) **Artist-skip scan** — check the skip list for fuzzy false-positives (0.6 batch threshold — Casket Cassette / Automatic; pre-create the distinct artist via `POST /admin/artists` so its 1.0 exact match wins). (b) **Headliner sanity scan** — grep kept headliners for leaked presenter/billing tokens: `/presents|present:|featuring| with |aftershow| pass$|hosted by|celebrates| w\/?$/i`. Any hit = the cleaning missed a presenter/theme line (this caught ~20 garbled Empty Bottle entries) → fix the transform and re-run.
7. **Confirm + ingest.** A 0-show or garbage result means the site changed → re-inspect (steps 1–3). Confirm counts are non-zero and plausible, then `--confirm`. Clean up scratch files.

Manual fix-ups via API: rename an artist `PATCH /admin/artists/{id} {"name":...}`; re-link a show's artists `PUT /shows/{id} {"artists":[{"id","is_headliner"}]}`; create one `POST /admin/artists {"name":...}` (exact find-or-create); delete an orphan (0-show) artist `DELETE /artists/{id}`. (Admin token required.)

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

1. **Render the page — the roster is usually client-side.** Label sites (Shopify, Squarespace) inject the artist grid via JS; a plain `curl` returns a shell without names. Use the browser MCP / `agent-browser`, `wait --load networkidle`, then extract artist names + their per-artist links from the live DOM. Don't hardcode a selector blindly — inspect the DOM each run.
2. **Honor the page's own sections — don't guess music vs non-music.** Many labels list visual artists / authors / photographers alongside musicians (Sacred Bones has an `Artists` + `Alumni` music roster and a `Books` section of 12 visual artists). Filter by the page's section headings, not by guessing per name. Exclude non-music sections; keep the music sections.
3. **Build the batch programmatically** (never hand-transcribe a 130-name roster). Emit one label item with the inline `artists` array. Cross-check display names against their slugs — source display text occasionally has typos the slug disproves (`childrens-hospital` slug vs. a displayed "Chidren's Hospital"); trust the slug. Preserve exact spelling/unicode otherwise (`Vår`, `SQÜRL`, `Föllakzoid`).
4. **Keep collaboration names un-split.** "Boris & Uniform", "Marissa Nadler & Stephen Brodsky" are distinct release-credit entities — do NOT auto-split on `&`/`and`/`,` (would shatter real names).
5. **Dry-run + the artist-skip QA scan** — `bun run src/entry.ts --env <env> batch /tmp/ph-ingest.json`. Map every SKIP back to the proposed name (the skip header shows the *existing* matched entity): the 0.6 fuzzy threshold conflates distinct artists (caught on Sacred Bones: **Institute→"Prostitute", Lathe of Heaven→"Left of Heaven", Cheena→"Cheem"**, plus a collab→member, **Emma Ruth Rundle & Thou→"Emma Ruth Rundle"**). A wrong skip also links the *wrong* existing artist to the label. **Fix:** pre-create each distinct artist via `POST /admin/labels`-adjacent `POST /admin/artists {"name":…}` (exact find-or-create) so its 1.0 match wins, then re-run. Do NOT pre-create via `ph submit artist` — that fuzzy-matches too.
6. **Confirm + verify** — `batch --confirm`, then verify against the API, not the CLI's self-report: `GET /labels/search?q=<name>` → id, then `GET /labels/{id}/artists` and check the **`count`** field equals the roster size (the roster response is `{artists, count}` — there is no `total` field, and it takes no `limit` param). The label's website is stored under `social.website` in the detail response (top-level `website` in *search* results is a separate projection and may read null).

7. **Stamp the source registry** (Catalog Refresh, PSY-1146) — so stale-first refresh knows this roster was just pulled. After a successful confirm, register the source (once) and stamp the refresh, using the label/venue id from step 6:
   ```bash
   bun run src/entry.ts --env <env> sources register label <label_id> "<roster_url>"
   bun run src/entry.ts --env <env> sources refresh label <label_id>
   ```
   To decide **what to refresh next**, list the stalest sources first: `ph sources stale --limit 20` (never-refreshed sort first). Venues use `sources register venue <venue_id> "<calendar_url>"` the same way.

### Label registry

| Label | Roster URL | Render | Sections (music kept / excluded) | Notes |
| --- | --- | --- | --- | --- |
| **Sacred Bones Records** | `https://www.sacredbonesrecords.com/pages/artists` | JS (Shopify) — browser MCP; artists are `/collections/<slug>` links under `#MainContent` | **Artists (80) + Alumni (50) = 130 kept**; **Books (12) excluded** (visual artists/authors: Peter Beste, Jesse Draxler, …) | First run 2026-06-20 → **stage**: label id 1 + 130 linked. Pre-created Institute / Lathe of Heaven / Cheena / Emma Ruth Rundle & Thou to beat 0.6 fuzzy false-matches. 2 source typos fixed via slug (Children's Hospital, Daily Void). `release_count` 0 (roster page has no release data). |

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
