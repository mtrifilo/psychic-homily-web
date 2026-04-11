# LLM Context — Psychic Homily

> Read this first every session. Routes you to the right doc for your task.

## What Is This?

Psychic Homily is **the spiritual successor to What.cd and Oink** — the same obsessive community curation, knowledge graph, and music discovery features, rebuilt for the legal streaming era. Instead of hosting files, we link to Bandcamp, Spotify, Discogs, YouTube, and other legal sources. Our unique advantage: **live shows as the discovery gateway**. Shows lead to artists, artists lead to releases and labels, labels lead to more artists and more shows. The knowledge graph grows outward from the live experience — something What.cd could never offer.

See `docs/vision.md` for the full north star, What.cd feature mapping, and entity model.

## Current Checkpoint (March 2026)

**Where we are:** Phase 2 **COMPLETE**. Phase 3 community contribution flows **in progress** — entity edit drawer (PSY-127), attribution (PSY-136), entity reports (PSY-131) all shipped. **Dogfood QA round (March 30-31):** 8 bugs found and fixed (PSY-254–261). Profile username editing, Cmd+K entity search, tag detail entity listing, timestamp fixes all merged. Code reorg fully complete. Community curation is the moat, not the data pipeline.


| Area          | Status                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Core entities | Artists, Venues, Shows, Releases, Labels, Festivals — all with full CRUD, slugs, search, admin UI. Collections, Requests, Revisions, Tags, Artist relationships: all full stack DONE. Tag admin, Bill position, Artist merge/split, Data quality dashboard, Scene pages: all DONE. **Data provenance** (PSY-34) on all 6 core entity tables. |
| User features | Accounts, saved shows, ICS calendar feed, favorite venues/cities, show reminders. **Contributor profiles** (PSY-63/64): public profiles, 3-level privacy, tier model, **profile editing** (PSY-261, username/name/bio). **Going/interested** (PSY-55): full stack DONE. **Follow system** (PSY-56): full stack DONE. **Entity edit drawer** (PSY-127): community edit suggestions with pending queue. **Entity reports** (PSY-131): flag issues on artists/venues/festivals. **Attribution** (PSY-136): "Last edited by" on detail pages. |
| Admin         | Show/venue approval workflows, batch approve/reject (PSY-81), audit log, discovery imports, release/label/festival CRUD, collection management. Data quality dashboard (PSY-45), tag admin (PSY-46), artist merge/split (PSY-47): all DONE. **Platform analytics** (PSY-48): backend DONE (growth, engagement, community health, data quality time-series), frontend planned. Phase 1.7 (opportunistic): dashboard UX polish (PSY-37–44). Phase 3: unified moderation queue, trust tiers, contributor leaderboard. |
| Auth          | Email/password, magic link, OAuth (Google/GitHub), passkeys (WebAuthn)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Discovery     | **AI-first pipeline operational.** Full end-to-end: venue config → tiered fetch (static/dynamic/screenshot via chromedp) → change detection → AI extraction (Claude Haiku) → non-music filtering → import with per-venue auto-approve control. Admin trigger endpoints live. **Automated scheduler** (PSY-31): background service with worker pool, circuit breaker, anomaly detection, Discord notifications. **Phase 1.6a COMPLETE.** **Phase 1.6b:** PSY-30 (AI billing) DONE, PSY-31 (scheduler) DONE. **PSY-58 (iCal/RSS feeds) DONE.** Remaining: PSY-33 (consolidate discovery UI — may be covered by PSY-36), PSY-35 (post-import enrichment). |
| Frontend      | Sidebar nav, **Cmd+K command palette with entity search** (PSY-257), redesigned show/artist/venue cards, density toggle, EntityDetailLayout template. **Feature modules** (`features/`): co-located components/hooks/types for all domains. Browse/detail pages for collections, requests, **tags with entity listing** (PSY-260), scenes. Revision history + **attribution lines** on entity detail pages. Library page (shows/artists/venues/releases/labels/festivals tabs). Charts page (trending shows, popular artists, active venues, hot releases). **Shared `formatRelativeTime` utility** with UTC-safe parsing. |
| Data seeding  | MusicBrainz CLI, Bandcamp enrichment CLI, Festival data entry CLI (all human-run with --dry-run)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Testing       | 69 E2E, 68.5%+ backend coverage, 2378 frontend unit tests (180 test files). **Dogfood QA** (March 30-31): all user journeys (V1-V5, U1-U14, A1-A3) tested via agent-browser. 8 bugs found and fixed. |
| Observability | PostHog analytics, Sentry error tracking                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| iOS           | Code complete (39 files), not shipped — needs Apple Developer enrollment                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |


**Phase 1.5 complete.** All 24 issues shipped.
**Phase 1.6 COMPLETE.** Pipeline foundation + AI billing + scheduler + feeds all shipped.
**Phase 1.7 COMPLETE.** Admin dashboard UX (PSY-37–44) all shipped.
**Phase 2 COMPLETE.** All 18 tickets across 2a (community foundations), 2b (tags, relationships, scenes), 2c (engagement, follow, analytics, charts, venue profiles) shipped.
**Code reorg COMPLETE.** Backend services reorg + frontend feature modules all shipped.
**Phase 3 IN PROGRESS.** Community contribution flows: entity edit drawer (PSY-127), attribution (PSY-136), entity reports (PSY-130/131), profile editing (PSY-261), pending edits (PSY-125). Dogfood QA complete — 8 bugs found and fixed (PSY-254–261).

**Recently shipped (March 2026):**
- Phase 3: Entity edit drawer (PSY-127), attribution lines (PSY-136), entity reports (PSY-130/131), profile username editing (PSY-261)
- Dogfood fixes: UTC timestamp fix (PSY-255), scene chart month fix (PSY-258), report dialog UX (PSY-256), Cmd+K entity search (PSY-257), tag detail entity listing (PSY-260), request author display (PSY-259), attribution username (PSY-254)

**Next up:**
- **Phase 3 (remaining):** PSY-125 (generic pending edits approval queue), PSY-126 (trust-tiered editing), PSY-128 (contribution prompts), unified moderation queue, contributor leaderboard.
- **Phase 2d (parallel):** Radio entities — PSY-160 data model shipped, KEXP MVP next.
- All Phase 2 + Phase 3 design docs complete. See `docs/strategy/ROADMAP.md` for full details.

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
| Dogfooding, QA testing, user journey catalog                                   | `docs/user-journeys.md`                                     |


## Guardrails

- **Entities use slugs** — all public-facing entities have SEO-friendly slug URLs
- **Approval workflow** — user-submitted content goes through admin review
- **Fire-and-forget** — Discord notifications and audit logs never fail parent operations
- **JSONB columns** — use `*json.RawMessage` (not `datatypes.JSON`)
- **Huma quirks** — all request body fields required by default, even pointers; mark optional explicitly. Query/path/header params must NOT use pointer types (`*uint`, `*string`) — Huma panics; use value types with zero-value checks instead.
- **Migration numbering** — latest is 000066 (add_comment_moderation_fields, PSY-292); next is 000067

