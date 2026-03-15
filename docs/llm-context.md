# LLM Context — Psychic Homily

> Read this first every session. Routes you to the right doc for your task.

## What Is This?

Psychic Homily is **the spiritual successor to What.cd and Oink** — the same obsessive community curation, knowledge graph, and music discovery features, rebuilt for the legal streaming era. Instead of hosting files, we link to Bandcamp, Spotify, Discogs, YouTube, and other legal sources. Our unique advantage: **live shows as the discovery gateway**. Shows lead to artists, artists lead to releases and labels, labels lead to more artists and more shows. The knowledge graph grows outward from the live experience — something What.cd could never offer.

See `docs/vision.md` for the full north star, What.cd feature mapping, and entity model.

## Current Checkpoint (March 2026)

**Where we are:** Phase 2c engagement/social **well advanced** — going/interested full stack (PSY-55) and follow backend (PSY-56) shipped; PSY-56 frontend and PSY-48 (analytics backend) in progress. Phase 2b knowledge graph connective tissue **COMPLETE**. Phase 2a community foundations **CODING COMPLETE** — all 12 issues shipped (PSY-63–74). Code reorg fully complete. Community curation is the moat, not the data pipeline.


| Area          | Status                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Core entities | Artists, Venues, Shows, Releases, Labels, Festivals — all with full CRUD, slugs, search, admin UI. Collections, Requests, Revisions, Tags, Artist relationships: all full stack DONE. Tag admin, Bill position, Artist merge/split, Data quality dashboard, Scene pages: all DONE. **Data provenance** (PSY-34) on all 6 core entity tables. |
| User features | Accounts, saved shows, ICS calendar feed, favorite venues/cities, show reminders. **Contributor profiles** (PSY-63/64): public profiles, 3-level privacy, tier model. **Going/interested** (PSY-55): full stack DONE — attendance buttons on show cards + My Shows page. **Follow system** (PSY-56): backend DONE (follow/unfollow, batch counts, followers list, my-following), frontend in progress. |
| Admin         | Show/venue approval workflows, batch approve/reject (PSY-81), audit log, discovery imports, release/label/festival CRUD, collection management. Data quality dashboard (PSY-45), tag admin (PSY-46), artist merge/split (PSY-47): all DONE. **Platform analytics** (PSY-48): in progress. Phase 1.7 (opportunistic): dashboard UX polish (PSY-37–44). Phase 3: unified moderation queue, trust tiers, contributor leaderboard. |
| Auth          | Email/password, magic link, OAuth (Google/GitHub), passkeys (WebAuthn)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Discovery     | **AI-first pipeline operational.** Full end-to-end: venue config → tiered fetch (static/dynamic/screenshot via chromedp) → change detection → AI extraction (Claude Haiku) → non-music filtering → import with per-venue auto-approve control. Admin trigger endpoints live. **Automated scheduler** (PSY-31): background service with worker pool, circuit breaker, anomaly detection, Discord notifications. **Phase 1.6a COMPLETE.** **Phase 1.6b:** PSY-30 (AI billing) DONE, PSY-31 (scheduler) DONE. **PSY-58 (iCal/RSS feeds) DONE.** Remaining: PSY-33 (consolidate discovery UI — may be covered by PSY-36), PSY-35 (post-import enrichment). |
| Frontend      | Sidebar nav, Cmd+K command palette, redesigned show/artist/venue cards, density toggle, EntityDetailLayout template. **Feature modules** (`features/`): co-located components/hooks/types for all domains — releases, labels, festivals, blog, auth, collections, requests, shows, artists, venues, scenes, tags. Browse/detail pages for collections, requests, tags, scenes. Revision history on entity detail pages. **My Shows** page with going/interested tabs. **AttendanceButton** on show cards + detail with batch fetching + optimistic updates. |
| Data seeding  | MusicBrainz CLI, Bandcamp enrichment CLI, Festival data entry CLI (all human-run with --dry-run)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Testing       | 69 E2E, 68.5%+ backend coverage, 1400+ frontend unit tests                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Observability | PostHog analytics, Sentry error tracking                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| iOS           | Code complete (39 files), not shipped — needs Apple Developer enrollment                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |


**Phase 1.5 complete.** All 24 issues (PSY-5 through PSY-28) shipped.
**Phase 1.6a COMPLETE.** All pipeline foundation issues shipped (PSY-29, PSY-75–83, PSY-34, PSY-36).
**Phase 1.6b nearly complete.** PSY-30, PSY-31, PSY-58 DONE. Remaining: PSY-33, PSY-35.
**Phase 2a CODING COMPLETE.** All 12 community foundation issues shipped (PSY-63–74).
**Code reorg COMPLETE.** Backend services reorg (PSY-84–92) and frontend feature modules (PSY-93–103) all shipped.
**Phase 2b COMPLETE.** Tags (PSY-49–51), artist relationships (PSY-52–53), tag admin (PSY-46), bill position (PSY-54), artist merge/split (PSY-47), scene pages (PSY-59/60), data quality dashboard (PSY-45) — all shipped.
**Phase 2c WELL ADVANCED.** PSY-55 full stack (going/interested) and PSY-56 backend (follow) shipped. PSY-56 frontend and PSY-48 backend in progress.

**Recently shipped (March 2026):**
- Phase 2c: PSY-55 full stack (going/interested — AttendanceButton on show cards + detail, My Shows page, batch fetching, optimistic updates), PSY-56 backend (follow system — 93 tests, batch counts, followers list, my-following endpoint)
- Phase 2b: PSY-46 (tag admin), PSY-54 (bill position), PSY-47 (artist merge/split), PSY-59/60 (scene pages), PSY-45 (data quality dashboard)

**Next up:**
- **Phase 2c (in progress):** PSY-56 frontend (follow buttons on entity pages + Following page), PSY-48 (platform analytics backend+frontend). Then: PSY-57 (top charts), venue profiles (PSY-61/62).
- **Phase 1.6b (background):** PSY-33, PSY-35.
- **Phase 1.7 (opportunistic):** Admin polish (PSY-37–44).
- **Phase 3 (Q4):** Open edit flows, trust tiers, unified moderation queue, show field notes, data export.
- All 5 Phase 2 design docs complete. See `docs/strategy/ROADMAP.md` for full details.

## Task Routing


| If your task involves...                                                       | Read these docs                                              |
| ------------------------------------------------------------------------------ | ------------------------------------------------------------ |
| Product vision, new entities, strategic direction                              | `docs/vision.md`                                             |
| What to build next, quarterly priorities                                       | `docs/strategy/ROADMAP.md`                                   |
| Frontend UI redesign (sidebar, layout, cards, templates)                       | `docs/strategy/ui-redesign.md`                               |
| Frontend form patterns, component conventions, query patterns                  | `docs/learnings/frontend-patterns.md`                        |
| Web frontend or Go backend features                                            | `docs/strategy/web.md`                                       |
| iOS app                                                                        | `docs/strategy/ios.md` + `docs/learnings/ios.md`             |
| Discovery scrapers or data pipeline                                            | `docs/strategy/discovery.md` + `docs/learnings/discovery.md` |
| Backend architecture, conventions, test patterns                               | `CLAUDE.md` (Architecture section)                           |
| Scene pages, venue intelligence, city landing pages                            | `docs/strategy/scene-pages.md`                               |
| Similar artist visualization, relationship graph, artist voting                | `docs/strategy/similar-artists.md`                           |
| Notification filters, matching engine, multi-channel notifications             | `docs/strategy/notification-filters.md`                      |
| Radio stations, radio shows, playlist parsing, "as heard on", co-occurrence    | `docs/strategy/radio-entities.md`                            |
| Festival intelligence, lineup overlap, breakout tracking, circuit analysis     | `docs/strategy/festival-intelligence.md`                     |
| Gazelle/What.cd implementation patterns (voting, tags, notifications, privacy) | `docs/learnings/gazelle-patterns.md`                         |
| Gazelle user profiles (identity hub, paranoia, ranks, customization, donors)   | `docs/learnings/gazelle-user-profiles.md`                    |
| What.cd user psychology, contributor motivation, product-market fit lessons     | `docs/learnings/whatcd-user-insights.md`                     |
| Agent workflow, Linear issues, PR process                                      | `docs/agent-workflow.md`                                     |


## Guardrails

- **Entities use slugs** — all public-facing entities have SEO-friendly slug URLs
- **Approval workflow** — user-submitted content goes through admin review
- **Fire-and-forget** — Discord notifications and audit logs never fail parent operations
- **JSONB columns** — use `*json.RawMessage` (not `datatypes.JSON`)
- **Huma quirks** — all request body fields required by default, even pointers; mark optional explicitly. Query/path/header params must NOT use pointer types (`*uint`, `*string`) — Huma panics; use value types with zero-value checks instead.
- **Migration numbering** — latest is 000053 (create_artist_aliases, PSY-47); next is 000054

