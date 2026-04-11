# PH CLI — Knowledge Graph Ingest Tool

> **STATUS: SHIPPED.** All tickets complete. Lives in a separate repo.
>
> Design doc for the `ph` CLI. Retained as reference.

## Problem

Adding data to the knowledge graph currently requires either the admin web UI (slow for bulk work) or automated pipeline extraction (only works for venue calendars). There's no fast path for:

- Processing tour announcement screenshots into shows
- Adding artists with social links from Instagram/Bandcamp profiles
- Importing WFMU playlist data (artists, releases, labels) from screenshots
- Bulk-entering festival lineups
- Adding venues from show flyers

The admin UI is optimized for review/approval, not rapid data entry. We need a CLI tool that lets an admin (via Claude Code) go from "screenshot of a tour poster" to "12 shows created with linked artists and venues" in under a minute.

## Architecture

### Claude Code as the AI Layer

The `ph` CLI intentionally has **no Claude API integration**. Claude Code itself is the extraction engine:

1. User pastes a screenshot or text into Claude Code
2. Claude Code (multimodal) extracts structured entity data
3. Claude Code calls `ph submit` with JSON
4. CLI validates, searches for duplicates, shows preview
5. User confirms in Claude Code conversation
6. Claude Code re-runs with `--confirm` flag
7. CLI submits to API, reports results

This keeps the CLI simple — it's a validated API client with duplicate detection, not an AI tool.

### Two-Phase Execution

Since Claude Code can't interact with running processes (no stdin), every command supports:

- **`--dry-run`** (default) — validate, search duplicates, show preview, exit
- **`--confirm`** — actually submit to the API

Claude Code runs dry-run first, shows the user the preview, then runs with `--confirm` on approval.

### Location & Runtime

- **Directory**: `/cli` in the monorepo (top-level, like `/discovery`)
- **Runtime**: Bun
- **CLI framework**: Commander.js (same as decant)
- **Config**: `~/.psychic-homily/config.json`

### Authentication

Uses existing `phk_`-prefixed API tokens. The backend's `HumaJWTMiddleware` already validates these — the token is passed as `Authorization: Bearer phk_xxx`. Tokens are created via the admin UI (`POST /admin/tokens`) or a future `ph init` flow.

### Environment Targeting

```json
// ~/.psychic-homily/config.json
{
  "environments": {
    "production": {
      "url": "https://api.psychichomily.com",
      "token": "phk_..."
    },
    "local": {
      "url": "http://localhost:8080",
      "token": "phk_..."
    }
  },
  "default_environment": "production"
}
```

Override per-command: `ph submit artist --env local ...`

## Commands

### `ph init`

Interactive setup: prompts for API URL, token, tests connection.

### `ph config`

View/edit configuration. `ph config show`, `ph config set default_environment local`.

### `ph submit <entity-type> [json]`

Submit a single entity or array of entities. Accepts JSON via argument or stdin.

**Supported entity types:** `show`, `artist`, `venue`, `release`, `label`, `festival`

**Workflow:**
1. Parse and validate JSON against entity schema
2. For each entity, search for existing duplicates by name
3. If duplicate found:
   - Show side-by-side diff (existing vs proposed)
   - Highlight fields that would be updated (new info the existing entity is missing)
   - Classify as UPDATE (merge new data into existing) or SKIP (already complete)
4. If no duplicate: classify as CREATE
5. Print summary table: N creates, N updates, N skips
6. With `--confirm`: execute all creates/updates, report results

**Example:**
```bash
# Dry run (default) — show what would happen
ph submit artist '[{"name": "Nina Hagen", "city": "Berlin", "bandcamp": "https://ninahagen.bandcamp.com"}]'

# Actually submit
ph submit artist --confirm '[{"name": "Nina Hagen", "city": "Berlin", "bandcamp": "https://ninahagen.bandcamp.com"}]'

# From stdin (for large payloads)
echo '[...]' | ph submit show --confirm

# Target local environment
ph submit show --env local --confirm '[...]'
```

### `ph search <entity-type> <query>`

Search for existing entities. Used by Claude Code to check for duplicates before building submit payloads.

```bash
ph search artist "Nina Hagen"
ph search venue "Crescent Ballroom"
ph search release "Satori"
ph search label "Numero"
```

### `ph batch <file.json>`

Submit a mixed-entity JSON file. The file contains an array of objects with an `entity_type` field:

```json
[
  {"entity_type": "artist", "name": "Nina Hagen", "city": "Berlin"},
  {"entity_type": "label", "name": "Legacy/Columbia"},
  {"entity_type": "release", "title": "Nunsexmonkrock", "artists": [{"name": "Nina Hagen Band"}], "release_year": 1982}
]
```

Processes in dependency order: labels → artists → releases → venues → festivals → shows.

### `ph status`

Show recent submissions from the current session.

## Entity Schemas (CLI Input)

These map to existing API request bodies but are simplified for CLI use.

### Artist
```typescript
{
  name: string            // required
  city?: string
  state?: string
  instagram?: string
  facebook?: string
  twitter?: string
  youtube?: string
  spotify?: string
  soundcloud?: string
  bandcamp?: string
  website?: string
}
```

### Venue
```typescript
{
  name: string            // required
  city: string            // required
  state: string           // required
  address?: string
  zipcode?: string
  instagram?: string
  website?: string
  // ... other socials
}
```

### Show
```typescript
{
  event_date: string      // required, ISO date or "YYYY-MM-DD"
  city: string            // required
  state: string           // required
  title?: string
  price?: number
  age_requirement?: string
  description?: string
  artists: Array<{
    name: string          // or id for existing
    id?: number
    is_headliner?: boolean
  }>
  venues: Array<{
    name: string          // or id for existing
    id?: number
    city?: string
    state?: string
  }>
}
```

### Release
```typescript
{
  title: string           // required
  release_type?: string   // lp, ep, single, compilation, live, remix, demo
  release_year?: number
  release_date?: string   // YYYY-MM-DD
  description?: string
  cover_art_url?: string
  artists: Array<{
    artist_id?: number    // if known
    name?: string         // for lookup/create
    role?: string         // main, featured, producer, remixer, composer, dj
  }>
  labels?: Array<{
    label_id?: number
    name?: string
  }>
  external_links?: Array<{
    platform: string      // bandcamp, spotify, discogs, etc.
    url: string
  }>
}
```

### Label
```typescript
{
  name: string            // required
  city?: string
  state?: string
  country?: string
  founded_year?: number
  status?: string         // active, inactive, defunct
  description?: string
  website?: string
  bandcamp?: string
  // ... other socials
}
```

### Festival
```typescript
{
  name: string            // required
  series_slug: string     // required
  edition_year: number    // required
  start_date: string      // required, YYYY-MM-DD
  end_date: string        // required, YYYY-MM-DD
  description?: string
  city?: string
  state?: string
  country?: string
  website?: string
  ticket_url?: string
  status?: string         // announced, confirmed, cancelled, completed
  artists?: Array<{
    name?: string
    artist_id?: number
    billing_tier?: string // headliner, sub_headliner, mid_card, undercard, local, dj, host
  }>
}
```

## Duplicate Detection

The CLI's most important feature. Before creating any entity, it searches the existing database:

### Search Strategy per Entity Type

| Entity | Search Method | Match Criteria |
|--------|--------------|----------------|
| Artist | `GET /artists/search?q={name}` | Case-insensitive name match, alias match |
| Venue | `GET /venues/search?q={name}` | Case-insensitive name + city match |
| Release | `GET /releases/search?q={title}` | Title + artist match (new endpoint needed) |
| Label | `GET /labels/search?q={name}` | Case-insensitive name match (new endpoint needed) |
| Festival | `GET /festivals/search?q={name}` | Name + year match (new endpoint needed) |
| Show | `GET /shows?city={city}&date={date}` | Date + venue + artist overlap |

### Update Detection

When a duplicate is found, the CLI compares fields:

```
Artist "Nina Hagen" found (ID: 42)
  city:      (empty) → "Berlin"        ← NEW INFO
  bandcamp:  (empty) → "ninahagen.bandcamp.com"  ← NEW INFO
  instagram: "@ninahagen"              ← ALREADY SET

  Action: UPDATE (2 new fields)
```

Fields with existing values are never overwritten — only empty fields are filled in. This makes the operation safe and idempotent.

## Backend Endpoints Used

All endpoints below are shipped and working.

| Endpoint | CLI Command | Added by |
|----------|-------------|----------|
| `POST /admin/artists` | `ph submit artist` (create) | PSY-138 |
| `POST /admin/venues` | `ph submit venue` (create) | PSY-139 |
| `GET /releases/search?q=` | Duplicate detection | PSY-140 |
| `GET /labels/search?q=` | Duplicate detection | PSY-140 |
| `GET /festivals/search?q=` | Duplicate detection | PSY-140 |
| `GET /artists/search?q=` | Duplicate detection | (pre-existing) |
| `GET /venues/search?q=` | Duplicate detection | (pre-existing) |
| `POST /shows` | `ph submit show` | (pre-existing) |
| `POST /releases` | `ph submit release` | (pre-existing) |
| `POST /labels` | `ph submit label` | (pre-existing) |
| `POST /festivals` | `ph submit festival` | (pre-existing) |
| `PATCH /admin/artists/{id}` | Update artist with new info | (pre-existing) |
| `PUT /venues/{id}` | Update venue with new info | (pre-existing) |
| `PUT /releases/{id}` | Update release with new info | (pre-existing) |
| `PUT /labels/{id}` | Update label with new info | (pre-existing) |
| `PUT /festivals/{id}` | Update festival with new info | (pre-existing) |
| `POST /tags` | Tag creation (auto on confirm) | (pre-existing) |
| `GET /tags/search?q=` | Tag duplicate detection | (pre-existing) |
| `POST /entities/{type}/{id}/tags` | Tag application | (pre-existing) |

## Tag Integration

All entity types accept an optional `tags` array. Tags can be strings (defaults to `genre` category) or objects with an explicit category:

```json
"tags": ["punk", "noise rock", {"name": "Japanese", "category": "locale"}]
```

Categories: `genre`, `locale`, `mood`, `era`, `style`, `instrument`, `other`.

The TagResolver handles:
- **Session caching** — same tag searched once across a batch
- **Fuzzy duplicate detection** — "post punk" flagged as similar to "post-punk" in dry-run
- **Alias resolution** — server-side, transparent to the CLI
- **Auto-creation** — new tags created on `--confirm`, 409 on duplicate treated as success
- **Idempotent application** — already-tagged entities silently skipped

Tags are applied even on SKIP actions (entity already exists but may not have these tags).

## `/ingest` Skill

The `/ingest` Claude Code skill automates the full screenshot-to-knowledge-graph workflow. Usage:

```
/ingest [--env production|local] [description or paste screenshot]
```

Workflow: extract entities from screenshot → build batch JSON → dry-run preview → user confirms → submit with tags.

See `.claude/skills/ingest/SKILL.md` for full documentation.

## Dev Utilities

### `gen-api-token`

Generates `phk_` API tokens directly against the database, bypassing the API auth bootstrap loop:

```bash
cd backend && go run ./cmd/gen-api-token --make-admin
```

Flags: `--user-id`, `--email`, `--days`, `--description`, `--make-admin`.

## WFMU / Radio Playlist Workflow

WFMU playlists (and other radio station playlists) are a rich source of artist, release, and label data. A single playlist screenshot like the one from `wfmu.org/playlists/shows/162125` yields rows with: **Artist, Track, Album, Label, Year, Comments**.

### Claude Code Workflow

1. User pastes WFMU playlist screenshot into Claude Code
2. Claude Code extracts structured data from the table:
   ```json
   [
     {"artist": "Nina Hagen", "track": "Born in Xixax", "album": "Nunsexmonkrock/Nina Hagen Band", "label": "Legacy/Columbia", "year": 1982},
     {"artist": "Flower Travelin' Band", "track": "Satori Part 1", "album": "Satori", "label": "Phoenix", "year": 1971},
     ...
   ]
   ```
3. Claude Code transforms this into `ph batch` format:
   - Each unique artist → artist entity (search first, create if new)
   - Each unique label → label entity (search first, create if new)
   - Each album → release entity (with artist + label linkage)
4. Claude Code calls `ph batch --dry-run` to preview
5. User reviews, confirms
6. Claude Code calls `ph batch --confirm`

### Data Mapping

| WFMU Column | PH Entity | Field |
|-------------|-----------|-------|
| Artist | Artist | `name` |
| Track | (stored for future radio_plays) | — |
| Album | Release | `title` |
| Label | Label | `name` |
| Year | Release | `release_year` |
| Comments | (informational only) | — |

Track-level data (individual songs played) will be stored when radio entity tables are built (see `docs/strategy/radio-entities.md`). For now, the CLI focuses on creating the artist/release/label entities that WFMU playlists surface.

## Project Structure

See `cli/README.md` for the full architecture diagram, entity schemas, and agent usage guide.

## Status

**All 12 tickets shipped (PSY-137 through PSY-148).** Post-project enhancements: tag integration, `/ingest` skill, `gen-api-token` utility, CLI README.

## Non-Goals

- **Claude API integration** — Claude Code handles all AI extraction
- **Radio entity backend** — separate project (`docs/strategy/radio-entities.md`). Track-level data deferred.
- **Interactive TUI** — Claude Code is the UI. CLI is a validated pipe.
- **Web scraping** — Claude Code reads screenshots. No fetching/parsing.
- **Non-admin access** — this is an admin power tool, requires `phk_` token from admin user
