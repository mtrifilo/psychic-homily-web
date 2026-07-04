# Screenshot / post batch ingest

Extract structured entity data from screenshots and social posts, build `/tmp/ph-ingest.json`, dry-run, confirm.

> Extraction rules are reified at `cli/eval/extraction-prompt.md` and `cli/eval/batch-schema.json` — keep this prose and that artifact in sync.

## Step 1: Extract from screenshot/post

Analyze ALL available sources:

- **Image/flyer**: visible text, artist names, dates, venues, prices
- **Caption/text**: show data, dates, venues, @handles, ticket links
- **Both together**: cross-reference — captions often have details not on the flyer

**WFMU playlists** — artists, tracks, albums (→ releases), labels, years  
**Show flyers** — artists (headliner/opener), venue, date, city/state, price  
**Tour announcements** — ALL shows listed; one show entry per date  
**Festival lineups** — festival name, dates, artists with billing tiers, venue(s)

### Multi-show extraction

Each date becomes its own show entry. Example tour post:

```json
[
  {"entity_type": "artist", "name": "La Witch", "city": "Los Angeles", "state": "CA", "instagram": "https://instagram.com/la_witch"},
  {"entity_type": "venue", "name": "Valley Bar", "city": "Phoenix", "state": "AZ"},
  {"entity_type": "venue", "name": "191 Toole", "city": "Tucson", "state": "AZ"},
  {"entity_type": "show", "event_date": "2026-04-15", "city": "Phoenix", "state": "AZ", "artists": [{"name": "La Witch", "is_headliner": true}], "venues": [{"name": "Valley Bar", "city": "Phoenix", "state": "AZ"}]},
  {"entity_type": "show", "event_date": "2026-04-16", "city": "Tucson", "state": "AZ", "artists": [{"name": "La Witch", "is_headliner": true}], "venues": [{"name": "191 Toole", "city": "Tucson", "state": "AZ"}]}
]
```

### Social links

Map @handles to **full on-platform URLs** (backend rejects bare handles):

- `@la_witch` → `"instagram": "https://instagram.com/la_witch"`
- Twitter/X → `"twitter": "https://twitter.com/handle"` (`x.com` also valid)
- Facebook / YouTube / Spotify / SoundCloud / Bandcamp → matching field by host; other links → `website`

**Post-author handle ≠ artist name is common** — when ambiguous, ask the user; don't mint a second entity or discard silently.

## Step 2: Build batch JSON

Write `/tmp/ph-ingest.json`:

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

### Entity schemas

**artist**: `name` (required), `city`, `state`, `country`, social fields, `website`, `description`, `tags`, `label` (name → linked after create)  
**venue**: `name`, `city`, `state` (required), `address`, `zipcode`, `country`, social fields, `website`, `description`, `tags`  
**show**: `event_date`, `city`, `state`, `title`, `price`, `ticket_url`, `artists`, `venues`  
**release**: `title`, `release_type`, `release_year`, `artists`, `external_links`, `tags`  
**label**: `name`, location fields, social fields, `founded_year`, `description`, `tags`, `artists` (inline roster — see [label-roster.md](label-roster.md))  
**festival**: `name`, `series_slug`, `edition_year`, `start_date`, `end_date`, `city`, `state`, `artists`, `tags`

### Tag format

```json
"tags": ["punk", "noise rock", {"name": "Japanese", "category": "locale"}]
```

Categories: `genre`, `locale`, `other`.

### Processing order

CLI processes: labels → artists → releases → venues → festivals → shows.

### Extraction guidelines

- Exact spelling from source; don't normalize artist names
- Release types: `lp`, `ep`, `single`, `compilation`, `live`
- Skip non-music: DJ interludes, commercials, trivia nights
- Festival billing tiers: headliner, sub_headliner, mid_card, undercard, local, dj, host

## Steps 3–5: Dry-run, confirm, fix-ups

```bash
cd /Users/mtrifilo/dev/psychic-homily-web/cli && bun run src/entry.ts --env <env> batch /tmp/ph-ingest.json
# After user OK:
bun run src/entry.ts --env <env> batch --confirm /tmp/ph-ingest.json
```

Present dry-run: counts per type, fuzzy tag matches, unresolved artists, validation errors.

Fix-ups: `submit artist --confirm`, then retry failed releases/shows.

See [troubleshooting.md](troubleshooting.md) for show dedup, timezone, and verify endpoints.
