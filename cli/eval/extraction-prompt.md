# Ingest Extraction Prompt (versioned source of truth)

This is the reified extraction step of the `/ingest` skill (Step 1 "Extract Data"
and Step 2 "Build Batch JSON"). The `/ingest` skill and the eval harness both
reference THIS file so there is one source of truth for how a flyer / playlist /
lineup image becomes batch JSON. When you change extraction rules, change them
here, then re-run the evals (`cli/eval/README.md`).

The output contract (field names, required fields, enums) is defined by
`cli/eval/batch-schema.json`, which mirrors what `ph batch` consumes
(`cli/src/commands/batch.ts`, `cli/src/lib/schemas.ts`).

---

## Prompt

You extract structured entity data from a music event image (show flyer, festival
lineup poster, tour announcement, or radio playlist screenshot) for the Psychic
Homily knowledge graph.

Read EVERYTHING visible in the image: artist names, the festival or event name,
dates, venue(s), city/state, prices, ticket links, and any @handles. Account for
visual hierarchy — on a festival poster, larger / higher-placed names are higher
billing tiers.

Produce a single JSON array. Each element is one entity with an `entity_type`
field. Output ONLY the JSON array — no prose, no markdown fences, no commentary.

### Entity types and required fields

- **artist**: `name` (required). Optional: `city`, `state`, `instagram`,
  `bandcamp`, `spotify`, `website`, `tags`.
- **venue**: `name`, `city`, `state` (all required). Optional: `address`,
  `instagram`, `website`, `tags`.
- **show**: `event_date` (`YYYY-MM-DD`), `city`, `state`, `artists`
  (array of `{name, is_headliner?}`, ≥1), `venues`
  (array of `{name, city, state}`, ≥1) — all required. Optional: `title`,
  `price`, `ticket_url`.
- **release**: `title`, `artists` (≥1) required. Optional: `release_type`
  (`lp`/`ep`/`single`/`compilation`/`live`/`remix`/`demo`), `release_year`,
  `external_links`, `tags`.
- **label**: `name` (required). Optional: `city`, `state`, `country`, `website`,
  `bandcamp`, `tags`.
- **festival**: `name`, `series_slug`, `edition_year`, `start_date`, `end_date`
  (all required). Optional: `location_name`, `city`, `state`, `country`,
  `website`, `status`, `venues` (array of `{name, is_primary?}`), `artists`
  (array of `{name, billing_tier?}`), `tags`.

### Extraction rules (apply exactly)

1. **Use the exact spelling from the image.** Do not correct, normalize, expand,
   or "fix" artist names. `3OH!3` stays `3OH!3`; `División Minúscula` keeps its
   accents; `Kiwi Jr.` keeps the period.
2. **One artist entity per distinct artist**, listed once even if it also appears
   in a festival lineup or multiple shows.
3. **Festival lineups are link-only.** A festival's inline `artists` array only
   *links* artists that exist; it never creates them. So EVERY lineup artist MUST
   ALSO appear as its own top-level `{"entity_type": "artist", "name": ...}` item,
   in addition to appearing inside the festival's `artists` array.
4. **Billing tiers** (festival lineup `billing_tier`) reflect the poster's visual
   hierarchy. Map tiers in descending prominence:
   `headliner` → `sub_headliner` → `mid_card` → `undercard`. Use `local`, `dj`,
   or `host` only when the source clearly indicates them. The biggest/top names
   are `headliner`; progressively smaller rows step down a tier.
5. **Dates**: festival `start_date`/`end_date` and show `event_date` are
   `YYYY-MM-DD`. Infer the year from the source. A date range like
   "September 18-19-20" → `start_date` first day, `end_date` last day.
6. **series_slug** is a stable kebab-case slug for the festival series WITHOUT the
   year (e.g. "Riot Fest 2026" → `riot-fest`). `edition_year` carries the year.
7. **Venue**: when a single primary venue/park is named, emit it as a `venue`
   entity (with `city`, `state`) AND reference it from the festival's `venues`
   array as `{"name": ..., "is_primary": true}`.
8. **Multi-show posts / tours**: emit one `show` per date, each with its own
   venue, city, state, and the full artist lineup for that date.
9. **@handles** (Instagram / social): map `@handle` → `https://instagram.com/handle`
   on the matching artist or venue. Include only when the handle clearly maps to
   an entity; skip when ambiguous.
10. **Tags**: add `genre` / `locale` tags only when confidently identifiable from
    the source. String tags default to genre; locale/other use
    `{"name": ..., "category": ...}`. Do not guess.
11. **Skip non-music entries**: DJ interludes, radio commercials, trivia nights,
    "tickets on sale", sponsor logos, and other non-entity text.

Return the JSON array now.
