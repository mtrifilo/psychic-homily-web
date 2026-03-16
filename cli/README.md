# PH CLI — Knowledge Graph Ingest Tool

CLI for rapidly adding entities (artists, venues, shows, releases, labels, festivals) to the Psychic Homily knowledge graph. Designed for use from Claude Code sessions — Claude extracts structured data from screenshots and text, the CLI validates, detects duplicates, and submits to the API.

## Quick Start

```bash
# 1. Install dependencies
bun install

# 2. Generate an API token (local dev — requires backend + PostgreSQL running)
cd ../backend && go run ./cmd/gen-api-token --make-admin

# 3. Configure the CLI
bun run src/entry.ts init --url http://localhost:8080 --token phk_xxx --name local

# 4. Verify
bun run src/entry.ts status
```

## Usage

### Search for existing entities

```bash
bun run src/entry.ts search artist "Nina Hagen"
bun run src/entry.ts search venue "Crescent"
bun run src/entry.ts search release "Satori"
bun run src/entry.ts search label "Numero"
bun run src/entry.ts search festival "M3F"
```

### Submit entities (single or batch)

Every submit command defaults to **dry-run** — it shows what would happen without making changes. Add `--confirm` to execute.

```bash
# Dry-run (preview only)
bun run src/entry.ts submit artist '[{"name": "Nina Hagen", "city": "Berlin", "tags": ["punk", {"name": "German", "category": "locale"}]}]'

# Execute
bun run src/entry.ts submit artist --confirm '[{"name": "Nina Hagen", "city": "Berlin"}]'

# Pipe from stdin
echo '[{"name": "Artist"}]' | bun run src/entry.ts submit artist --confirm
```

### Batch import (mixed entity types)

```bash
# Dry-run
bun run src/entry.ts batch /tmp/data.json

# Execute
bun run src/entry.ts batch --confirm /tmp/data.json
```

Batch files are JSON arrays with an `entity_type` field on each item. The CLI processes them in dependency order: labels → artists → releases → venues → festivals → shows.

### Environment targeting

```bash
# Default environment is set during init
bun run src/entry.ts --env local submit artist '[...]'
bun run src/entry.ts --env production submit artist '[...]'

# Change default
bun run src/entry.ts config set default_environment local
```

## Entity Schemas

### Artist
```json
{"name": "Required", "city": "Optional", "state": "Optional", "instagram": "@handle", "bandcamp": "url", "spotify": "url", "website": "url", "tags": ["genre", {"name": "Locale", "category": "locale"}]}
```

### Venue
```json
{"name": "Required", "city": "Required", "state": "Required", "address": "Optional", "website": "url"}
```

### Show
```json
{"event_date": "2026-04-15", "city": "Required", "state": "Required", "title": "Optional", "price": 25.00, "artists": [{"name": "Artist", "is_headliner": true}], "venues": [{"name": "Venue", "city": "City", "state": "ST"}]}
```
- `event_date` accepts `YYYY-MM-DD` (auto-normalized to `YYYY-MM-DDT20:00:00Z`)
- Artists and venues are resolved by name search — existing entities use their ID, new ones are created automatically

### Release
```json
{"title": "Required", "release_type": "lp", "release_year": 2025, "artists": [{"name": "Artist"}], "external_links": [{"platform": "bandcamp", "url": "https://..."}], "tags": ["genre"]}
```
- `release_type`: `lp`, `ep`, `single`, `compilation`, `live`, `remix`, `demo`

### Label
```json
{"name": "Required", "city": "Optional", "state": "Optional", "country": "Optional", "website": "url"}
```

### Festival
```json
{"name": "Required", "series_slug": "required-slug", "edition_year": 2026, "start_date": "2026-06-01", "end_date": "2026-06-03", "city": "Optional", "state": "Optional", "artists": [{"name": "Artist", "billing_tier": "headliner"}]}
```
- `billing_tier`: `headliner`, `sub_headliner`, `mid_card`, `undercard`, `local`, `dj`, `host`

## Tags

Tags can be included on any entity (artist, release, label, festival, venue). They're specified as an array of strings or objects:

```json
"tags": ["punk", "noise rock", {"name": "Japanese", "category": "locale"}]
```

- String tags default to `category: "genre"`
- Object tags specify a category: `genre`, `locale`, `mood`, `era`, `style`, `instrument`, `other`
- The CLI auto-detects existing tags (case-insensitive, with alias resolution)
- Fuzzy duplicates are flagged in dry-run (e.g., "post punk" matches "post-punk")
- New tags are created automatically on `--confirm`
- Tags are applied even when an entity is skipped (already exists)

## Duplicate Detection

The CLI searches for existing entities before creating. For each entity it classifies the action:

- **CREATE** — no match found, will create new entity
- **UPDATE** — match found, proposed data has new fields to add (never overwrites existing values)
- **SKIP** — match found, no new information to add

## Agent Usage Guide

### Using the `/ingest` skill

The `/ingest` skill automates the full screenshot-to-knowledge-graph flow. When a user pastes a screenshot (WFMU playlist, show flyer, tour poster, festival lineup), invoke `/ingest` to guide the extraction.

### Common mistakes to avoid

1. **Don't forget `--confirm`** — without it, nothing is submitted. Always dry-run first, then confirm.

2. **Don't skip the dry-run** — show the user the preview before confirming. They need to verify entity names, tag assignments, and duplicate detection results.

3. **Don't guess artist/venue IDs** — let the CLI resolve them by name. Use `name` fields, not `id` fields, unless you've verified the ID via `ph search`.

4. **Don't normalize artist names** — use the exact spelling from the source material. The duplicate detection handles case-insensitive matching.

5. **Event dates are just YYYY-MM-DD** — the CLI normalizes to ISO 8601 automatically. Don't add timezone info.

6. **Use batch for multi-entity imports** — don't run 20 separate `ph submit` commands. Write a batch JSON file and use `ph batch`. The dependency ordering (labels → artists → releases → venues → festivals → shows) is handled automatically.

7. **Tags are optional but valuable** — add genre and locale tags when you can confidently identify them from the source material. Don't guess genres you're unsure about.

8. **Check for existing entities first** — before a large import, use `ph search` to check if key entities already exist. This helps you anticipate which items will be creates vs updates vs skips.

9. **The CLI needs the backend running** — for local dev, the Go backend must be running on port 8080. Use `ph status` to verify connectivity before starting an import.

10. **Config defaults to production** — make sure `default_environment` is set correctly. Use `ph config show` to check, and `ph config set default_environment local` to change.

### Batch JSON tips for agents

- Put all entities in a single flat array with `entity_type` on each item
- Labels and artists before releases (releases reference artists by name)
- Venues before shows (shows reference venues by name)
- The CLI deduplicates — if the same artist appears in multiple releases, list them once in the artists section
- For WFMU playlists: skip DJ interludes, compilation album entries without distinct artists, and radio commercials
- For release types: use `compilation` for various-artist compilations, `lp` for standard albums, `live` for live recordings/bootlegs

## Development

```bash
bun install          # Install dependencies
bun test             # Run tests (232 tests)
bunx tsc --noEmit    # Type check
bun run src/entry.ts --help  # CLI help
```

## Architecture

```
cli/
├── src/
│   ├── entry.ts              # Bun shebang entry point
│   ├── cli.ts                # Commander.js command wiring
│   ├── commands/
│   │   ├── init.ts           # ph init (configure environment)
│   │   ├── config.ts         # ph config show/set
│   │   ├── search.ts         # ph search <type> <query>
│   │   ├── status.ts         # ph status (connection check)
│   │   ├── batch.ts          # ph batch <file.json>
│   │   ├── submit-artist.ts  # ph submit artist
│   │   ├── submit-venue.ts   # ph submit venue
│   │   ├── submit-show.ts    # ph submit show
│   │   ├── submit-release.ts # ph submit release
│   │   ├── submit-label.ts   # ph submit label
│   │   └── submit-festival.ts # ph submit festival
│   └── lib/
│       ├── api.ts            # API client (fetch + auth)
│       ├── config.ts         # Config file management
│       ├── types.ts          # Shared TypeScript types
│       ├── display.ts        # Terminal output (tables, diffs, colors)
│       ├── ansi.ts           # ANSI color helpers (NO_COLOR aware)
│       ├── duplicates.ts     # Duplicate detection engine
│       ├── schemas.ts        # Entity validation
│       └── tags.ts           # Tag resolution (search, create, apply)
└── test/                     # 232 tests across 15 files
```
