# LLM Context — Psychic Homily

> Read this first every session. Routes you to the right doc for your task.

## What Is This?

Psychic Homily is **the spiritual successor to What.cd and Oink** — the same obsessive community curation, knowledge graph, and music discovery features, rebuilt for the legal streaming era. Instead of hosting files, we link to Bandcamp, Spotify, Discogs, YouTube, and other legal sources. Our unique advantage: **live shows as the discovery gateway**. Shows lead to artists, artists lead to releases and labels, labels lead to more artists and more shows. The knowledge graph grows outward from the live experience — something What.cd could never offer.

See `docs/vision.md` for the full north star, What.cd feature mapping, and entity model.

## Current Checkpoint (March 2026)

**Where we are:** Phase 1.5 — knowledge graph vertical slice actively under construction. Frontend redesign complete, Releases and Labels entities landed.

| Area | Status |
|------|--------|
| Core entities | Artists, Venues, Shows (many-to-many, slugs, search, filters), **Releases** (with artist roles + external links, full CRUD API), **Labels** (with artist/release junctions, catalog numbers). Building next: Festivals, enriched artist pages. Planned: Tags, Scenes, Collections, Requests, Radio Stations, Radio Shows, Musicians, Promoters. |
| User features | Accounts, saved shows, ICS calendar feed, favorite venues/cities, show reminders (email, 24h before). Planned: generic `user_bookmarks` table replacing per-entity tables. |
| Admin | Show/venue approval workflows, pending edits, audit log, discovery imports, release CRUD (admin-only API) |
| Auth | Email/password, magic link, OAuth (Google/GitHub), passkeys (WebAuthn) |
| Discovery | 5 venue scrapers (Phoenix metro), admin import UI |
| Frontend | Sidebar nav, Cmd+K command palette, redesigned show/artist/venue cards, density toggle, TagPill + RelationshipBadge placeholder components |
| Testing | 69 E2E, 68.5%+ backend coverage, 897+ frontend unit tests |
| Observability | PostHog analytics, Sentry error tracking |
| iOS | Code complete (39 files), not shipped — needs Apple Developer enrollment |

**In progress:** Label service/handler/routes (PSY-10), Release frontend pages + entity detail template (PSY-8 + PSY-18).

**Recently shipped:** Release model + migration (PSY-5), Release service/handler/routes with 57 tests (PSY-6), Label model + migration (PSY-9), visual polish sweep with density toggle (PSY-21), artist card redesign (PSY-20), sidebar nav (PSY-16), Cmd+K command palette (PSY-17), show card redesign (PSY-19).

**Next up:** Label frontend pages (PSY-12), enriched artist pages with discography + label affiliations (PSY-13), release admin UI (PSY-7), label admin UI (PSY-11), MusicBrainz data seeding (PSY-14), Bandcamp enrichment (PSY-15), generic `user_bookmarks` table, Festival entity.

## Task Routing

| If your task involves... | Read these docs |
|--------------------------|-----------------|
| Product vision, new entities, strategic direction | `docs/vision.md` |
| What to build next, quarterly priorities | `docs/strategy/ROADMAP.md` |
| Frontend UI redesign (sidebar, layout, cards, templates) | `docs/strategy/ui-redesign.md` |
| Web frontend or Go backend features | `docs/strategy/web.md` |
| iOS app | `docs/strategy/ios.md` + `docs/learnings/ios.md` |
| Discovery scrapers or data pipeline | `docs/strategy/discovery.md` + `docs/learnings/discovery.md` |
| Backend architecture, conventions, test patterns | `CLAUDE.md` (Architecture section) |
| Gazelle/What.cd implementation patterns (voting, tags, notifications, privacy) | `docs/learnings/gazelle-patterns.md` |
| Agent workflow, Linear issues, PR process | `docs/agent-workflow.md` |

## Guardrails

- **Entities use slugs** — all public-facing entities have SEO-friendly slug URLs
- **Approval workflow** — user-submitted content goes through admin review
- **Fire-and-forget** — Discord notifications and audit logs never fail parent operations
- **JSONB columns** — use `*json.RawMessage` (not `datatypes.JSON`)
- **Huma quirk** — all request body fields required by default, even pointers; mark optional explicitly
- **Migration numbering** — latest is 000036 (create_labels); next is 000037
