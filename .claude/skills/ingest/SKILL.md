---
name: ingest
description: Extract entities from screenshots (show flyers, WFMU playlists, tour announcements, festival lineups) and import them into the Psychic Homily knowledge graph via the ph CLI.
argument-hint: "[description of what to ingest, or paste a screenshot]"
---

# Ingest: Screenshot to Knowledge Graph

Extract structured entity data from screenshots and import into Psychic Homily using the `ph` CLI.

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

### Step 1: Extract Data from Screenshot

When the user provides a screenshot (show flyer, WFMU playlist, tour poster, festival lineup, etc.), extract ALL entities visible:

**For WFMU playlists** — extract: artists, tracks, albums (→ releases), labels, years
**For show flyers** — extract: artists (with headliner/opener), venue, date, city/state, price
**For tour announcements** — extract: artist, multiple dates/venues/cities
**For festival lineups** — extract: festival name, dates, artists with billing tiers, venue(s)

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

**artist**: `name` (required), `city`, `state`, `instagram`, `bandcamp`, `spotify`, `website`, `tags`
**venue**: `name` (required), `city` (required), `state` (required), `address`, `website`, `tags`
**show**: `event_date` (required, YYYY-MM-DD), `city` (required), `state` (required), `title`, `price`, `artists` (required, array of `{name, is_headliner?}`), `venues` (required, array of `{name, city, state}`)
**release**: `title` (required), `release_type` (lp/ep/single/compilation/live/remix/demo), `release_year`, `artists` (required), `external_links` ([{platform, url}]), `tags`
**label**: `name` (required), `city`, `state`, `country`, `website`, `bandcamp`, `tags`
**festival**: `name` (required), `series_slug` (required), `edition_year` (required), `start_date` (required), `end_date` (required), `city`, `state`, `artists` ([{name, billing_tier}]), `tags`

#### Tag format

Tags can be strings (defaults to genre) or objects with category:
```json
"tags": ["punk", "noise rock", {"name": "Japanese", "category": "locale"}]
```

Categories: `genre`, `locale`, `mood`, `era`, `style`, `instrument`, `other`

#### Processing order

The batch command processes in dependency order: labels → artists → releases → venues → festivals → shows. Put entities in any order — the CLI handles sequencing.

#### Guidelines for data extraction

- **Artist names**: Use the exact spelling from the source. Don't correct or normalize.
- **Release types**: `lp` for full albums, `ep` for EPs, `compilation` for comps/anthologies, `live` for live recordings, `single` for singles.
- **Release years**: Use the original release year when available. For reissues, use the reissue year.
- **Tags**: Add genre and locale tags where you can confidently identify them. Common genres: punk, post-punk, noise rock, psychedelic, electronic, industrial, experimental, ambient, folk, gospel, funk, disco, synth pop, avant-garde, hip-hop, jazz, metal. Locale tags use `category: "locale"`: Japanese, German, Spanish, Russian, Thai, Brazilian, etc.
- **Billing tiers** (festivals): headliner, sub_headliner, mid_card, undercard, local, dj, host.
- **Skip non-music entries**: DJ interludes, radio commercials, compilation album titles without a distinct artist, trivia nights.

### Step 3: Dry Run

Run the batch in dry-run mode and show the user the preview:
```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts batch /tmp/ph-ingest.json
```

Present the output to the user and ask for confirmation. Highlight:
- How many entities of each type will be created/updated/skipped
- Any fuzzy tag matches (where existing similar tags were found)
- Any unresolved artists (for releases/shows)
- Any validation errors

### Step 4: Confirm

After user approval:
```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts batch --confirm /tmp/ph-ingest.json
```

Report the results: how many created, updated, skipped, errored.

### Step 5: Fix-ups (if needed)

If any entities failed (e.g., unresolved artists for releases), fix them individually:
```bash
# Create the missing artist
bun run src/entry.ts submit artist --confirm '[{"name": "Missing Artist"}]'

# Retry the release
bun run src/entry.ts submit release --confirm '[{"title": "Album", "artists": [{"name": "Missing Artist"}]}]'
```

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
```
