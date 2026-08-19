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
| Venue (archive) | `chronology-hall-exemplar-phoenix-az`             | `/venues/chronology-hall-exemplar-phoenix-az`           |

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
- **Venue (archive)** — see below. The one exemplar whose point is its
  history rather than its field coverage.

## The archive exemplar (PSY-1843)

The PSY-665 exemplars above cover *rich fields*. They do not cover *depth
of history*: the richest of them has 3 past shows, so the venue page's
past-shows archive — year strip with per-year counts, month-range page
labels, 50-per-page pagination — renders in its degenerate one-page form
and cannot be visually reviewed on a fresh stack. Before this existed, a
throwaway venue had to be hand-written into a stack with raw SQL to review
that UI (during PR #1904).

`chronology-hall-exemplar-phoenix-az` closes that gap. It is implemented in
`backend/cmd/seed/exemplars_archive.go` and seeded by the same
`seedRichExemplars` entry point.

> **A fresh dispatch stack does NOT have this venue.** `stack-up.sh` seeds
> via `frontend/e2e/setup-db.sh`, which never runs `cmd/seed`. Apply it
> first, or every URL below 404s:
>
> ```bash
> source dispatch-stack/.env
> cd backend && DATABASE_URL="$STACK_POSTGRES_URL" ENVIRONMENT=development go run ./cmd/seed
> ```

> **Not created on stage or production.** `cmd/seed` is not dev-only —
> `backend/scripts/deploy-stage.sh` runs it on every stage deploy, and
> `psy-deploy-prod --with-db-restore` copies stage's catalog into prod. A
> verified venue with hundreds of approved shows would outrank real venues
> in the graph, scene rollups, and sitemap, so `archiveExemplarEnabled`
> skips this fixture unless all three of these agree it is local:
>
> | Signal | Skips when | Why it alone is not enough |
> | --- | --- | --- |
> | `ENVIRONMENT` | `stage` / `production` | On stage it arrives only via the gitignored `.env.stage`; regenerate that from `.env.example` (no `ENVIRONMENT` key) and the signal vanishes |
> | `NODE_ENV` | `stage` / `production` | Set directly by the deploy command, so it survives a dotenv mishap, but it is not what the rest of the config reads |
> | `DATABASE_URL` | host is not `localhost`/`127.0.0.1` | Ground truth about which database is about to be written; catches an ad-hoc run against a deployed DSN with no env name set |
>
> The env-name checks fail open (unset or unfamiliar values still seed) so
> local workflows are unaffected; the `DATABASE_URL` check is what makes a
> deployed DSN fail closed.

| Property | Value |
| --- | --- |
| Past shows | 360 across 3 calendar years (2023: 62, 2024: 108, 2025: 190) |
| Pages at 50/page | 8 all-years; 2 / 3 / 4 per year — every year paginates |
| Upcoming shows | 5, re-dated on every seed so they never drift into the past |
| Bills | 1-3 acts typically, ~1 in 11 is 6-8 acts, from 14 fictional `-exemplar` artists |
| Timezone | `America/Phoenix`, venue-local evening start times |

What it exercises that nothing else does: the year strip's per-year counts,
the month-range page labels (page 1 is `Oct–Dec 2025`, page 2 `Aug–Oct
2025`, page 8 `Jan–Mar 2023`; the distribution also puts a page across a
year boundary, so the crossing-years label form renders too), the pager's
ellipsis branch (only reachable above 7 pages), empty months the labels
must skip, `SOLD OUT` and `CANCELLED` badges, all three price states
(`Free` / `$12.50` / absent), and bill wrapping via deliberately uneven
artist-name lengths (`Vane (Exemplar)` through `Ada Vaughn-Reyes and the
Long Goodbye (Exemplar)`).

Useful URLs for a screenshot pass — note the year is a **path segment**,
not a `?year=` param:

```
/venues/chronology-hall-exemplar-phoenix-az            # page 1 + Upcoming
/venues/chronology-hall-exemplar-phoenix-az?page=4     # mid-archive, ellipsis pager
/venues/chronology-hall-exemplar-phoenix-az/shows/2025 # single-year archive
/venues/chronology-hall-exemplar-phoenix-az/shows/2025?page=2
```

Before editing it, read the header comment in `exemplars_archive.go`.
Everything there follows from one fact — the idempotency key is each show's
generated slug, which embeds its date and headliner — and the ways to break
it by accident are not obvious. The counts above are load-bearing (they are
what makes each UI branch render) and are pinned by
`TestArchiveFixtureMeetsItsReviewThresholds` and
`TestArchiveDocumentedCountsAreAccurate`; the page labels below are derived
from them by hand, so re-verify those if you change `archiveYears`.

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
