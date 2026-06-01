# Dev seed: rich exemplars (PSY-665)

The local dev seed (`backend/cmd/seed`) historically created
minimum-viable entities — only the fields a feature demo needed. Most
optional fields (description, social links, external_links,
cover_art_url, tags) were NULL or empty, so rich-data render paths
(About sections, Listen/Buy grids, social rows, tag clouds, multi-day
festival lineups) were untestable locally.

PSY-665 adds **one rich exemplar per entity type** with *every* optional
field populated, plus the empty-state canaries that must stay testable.
The exemplar code lives in `backend/cmd/seed/exemplars.go`
(`seedRichExemplars`), invoked from `main()` after the test users exist
(tag and collection FKs are `NOT NULL` references to `users`).

## How to apply

```bash
cd backend
NODE_ENV=development go run ./cmd/seed   # reads .env.development for DATABASE_URL
```

The exemplar seed is **additive** (every exemplar uses a new, fixed
`*-exemplar` slug, so existing dev/E2E entities are untouched) and
**idempotent** (each create is guarded by a slug existence check —
re-running neither duplicates rows nor breaks referential integrity).

To apply against a dispatch stack's isolated Postgres (which is seeded
by `frontend/e2e/setup-db.sh`, not `cmd/seed`), point `DATABASE_URL` at
the stack DB and set `ENVIRONMENT` so the remote-host guard passes:

```bash
DATABASE_URL="$STACK_POSTGRES_URL" ENVIRONMENT=development go run ./cmd/seed
```

## Exemplar slugs (for screenshot / repro work)

Each rich exemplar has every optional field per its PSY-665 acceptance
criterion populated. Image fields point at local committed placeholders
under `frontend/public/seed-placeholders/` (rendered via plain `<img>`,
so no `next/image` remote-host allowlist applies); entity **names** stay
realistic per the ticket.

| Entity     | Rich exemplar slug                                     | URL path                                                |
| ---------- | ------------------------------------------------------ | ------------------------------------------------------- |
| Artist     | `marissa-nadler-exemplar`                              | `/artists/marissa-nadler-exemplar`                      |
| Venue      | `the-rhythm-room-exemplar-phoenix-az`                  | `/venues/the-rhythm-room-exemplar-phoenix-az`           |
| Release    | `the-path-of-the-clouds-exemplar`                      | `/releases/the-path-of-the-clouds-exemplar`             |
| Label      | `sacred-bones-records-exemplar`                        | `/labels/sacred-bones-records-exemplar`                 |
| Festival   | `marfa-myths-exemplar-2026`                            | `/festivals/marfa-myths-exemplar-2026`                  |
| Show       | `the-path-tour-exemplar-at-the-rhythm-room-exemplar`   | `/shows/the-path-tour-exemplar-at-the-rhythm-room-exemplar` |
| Collection | `psychic-homily-staff-picks-exemplar`                  | `/collections/psychic-homily-staff-picks-exemplar`      |

### What each exemplar exercises

- **Artist** — bio, image, all 8 social links, 6 tags across genre /
  locale / other, 2 aliases, a label link, a release credit, 3 upcoming
  + 3 past tracked shows, 4 similar-artist edges, 1 festival appearance
  (headlines day 1 of the festival exemplar).
- **Venue** — image, all 8 social links, 6 tags, 3 upcoming + 3 past
  shows (via the artist's tracked shows).
- **Release** — 200+ char description with a paragraph break, cover art,
  5 external links (bandcamp / spotify / apple_music / youtube_music /
  discogs), a label link with catalog number `SBR-EXEMPLAR-001`, 3
  credited artists with distinct roles (main / featured / producer), 6
  tags.
- **Label** — description, image, all 8 social links, `founded_year`
  2007, Brooklyn / NY / USA, 6 tags, 3 associated artists, a release in
  its catalog.
- **Festival** — description, flyer, all 8 social links (jsonb),
  website, ticket URL, 2 venues with `is_primary` flags, a 3-day lineup
  (6 artists/day, 18 slots) covering every billing tier (headliner,
  sub_headliner, mid_card, undercard, local, dj), 6 tags.
- **Show** — description, flyer image, `age_requirement` 21+, ticket
  URL, 6 tags, a 5-act bill with full `set_type` variety (headliner /
  support / opener / dj / host).
- **Collection** — description, cover image, 6 tags, 6 items spanning
  every entity type (artist / release / festival / show / venue / label)
  with per-item notes, ranked display mode.

## Empty-state canaries — DO NOT backfill

These preserve the truthy-but-empty / empty-list render paths so the
hide-when-empty UI stays testable. They are intentional and must NOT be
given social links, venues, links, or tags.

| Canary                                  | Slug                          | Shape preserved                          |
| --------------------------------------- | ----------------------------- | ---------------------------------------- |
| Festival with `social = {}` (truthy)    | `desert-daze-exemplar-2026`   | PSY-657 truthy-empty-object + no venues  |

The minimal dev/E2E seed already provides the other canaries
(`external_links: []` and `tags: []` on most existing releases/venues),
so the rich exemplars don't disturb them.
