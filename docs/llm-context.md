# LLM Context — Psychic Homily

> Read this first every session. Routes you to the right doc for your task.

## What Is This?

Psychic Homily is **the spiritual successor to What.cd and Oink** — the same obsessive community curation, knowledge graph, and music discovery features, rebuilt for the legal streaming era. Instead of hosting files, we link to Bandcamp, Spotify, Discogs, YouTube, and other legal sources. Our unique advantage: **live shows as the discovery gateway**. Shows lead to artists, artists lead to releases and labels, labels lead to more artists and more shows. The knowledge graph grows outward from the live experience — something What.cd could never offer.

See `docs/vision.md` for the full north star, What.cd feature mapping, and entity model.

## Current Checkpoint (March 2026)

**Where we are:** Phoenix show tracker, v1 feature-complete, pre-launch hardening. Phase 1 (utility features).

| Area | Status |
|------|--------|
| Core entities | Artists, Venues, Shows (many-to-many relationships, slugs, search, filters). Building next: Festivals (distinct entity with series_slug, billing tiers, multi-venue support), Releases, Labels. Planned: Tags, Scenes, Collections, Requests, Radio Stations, Radio Shows, Musicians, Promoters. |
| User features | Accounts, saved shows, ICS calendar feed, favorite venues/cities, show reminders (email, 24h before). Planned: generic `user_bookmarks` table replacing per-entity tables. |
| Admin | Show/venue approval workflows, pending edits, audit log, discovery imports |
| Auth | Email/password, magic link, OAuth (Google/GitHub), passkeys (WebAuthn) |
| Discovery | 5 venue scrapers (Phoenix metro), admin import UI |
| Testing | 69 E2E, 68.5% backend coverage, 897 frontend unit tests |
| Observability | PostHog analytics, Sentry error tracking |
| iOS | Code complete (39 files), not shipped — needs Apple Developer enrollment |

**In progress:** Frontend redesign polish + show reminders PR, then knowledge graph vertical slice.

**Recently shipped:** Sidebar nav (PSY-16), Cmd+K command palette (PSY-17), show card redesign (PSY-19), show reminder emails with unsubscribe flow, `testutil.RunAllMigrations()` helper (eliminates manual migration lists in tests).

**Next up:** Merge show reminders PR (#15) and command palette fix PR (#16). Then: entity detail template (PSY-18), artist card redesign (PSY-20), visual polish (PSY-21), data layer foundation (generic `user_bookmarks` table), Festival entity, Releases + Labels entities, enriched artist pages, data seeding.

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
- **Migration numbering** — latest is 000034; next is 000035
