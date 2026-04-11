# LLM Context — Psychic Homily

> Read this first every session. Routes you to the right doc for your task.

## What Is This?

Psychic Homily is **the spiritual successor to What.cd and Oink** — the same obsessive community curation, knowledge graph, and music discovery features, rebuilt for the legal streaming era. Instead of hosting files, we link to Bandcamp, Spotify, Discogs, YouTube, and other legal sources. Our unique advantage: **live shows as the discovery gateway**. Shows lead to artists, artists lead to releases and labels, labels lead to more artists and more shows. The knowledge graph grows outward from the live experience — something What.cd could never offer.

See `docs/vision.md` for the full north star, What.cd feature mapping, and entity model.

## Current Checkpoint (April 2026)

**Where we are:** All phases through Phase 3 **COMPLETE**. Radio entities (Phase 2d) **COMPLETE** with 3 provider bug fixes shipped. **Collections UX overhaul COMPLETE** — feature went from non-functional shell to 75% complete in one session (5 PRs). **Comments system Wave 1-5 COMPLETE** — full discussion infrastructure with voting, subscriptions, moderation, trust tiers, and show field notes. Community curation is the moat.


| Area          | Status                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Core entities | Artists, Venues, Shows, Releases, Labels, Festivals — all with full CRUD, slugs, search, admin UI. Collections, Requests, Revisions, Tags, Artist relationships: all full stack DONE. Tag admin, Bill position, Artist merge/split, Data quality dashboard, Scene pages: all DONE. **Data provenance** (PSY-34) on all 6 core entity tables. |
| Collections   | **Full-stack DONE** (PSY-314–318): add/remove items from collection + entity detail pages, per-item notes, reorder (up/down), browse page with tabs/search/filters, entity backlinks ("In Collections" on all 6 entity types), user profile Collections tab, share button, timestamps, entity type breakdown. Backend was already complete; 5 PRs wired up the frontend. |
| Comments      | **Waves 1-5 DONE** (PSY-285–295): polymorphic comments on all entity types, bounded nesting (3 levels), markdown rendering (goldmark + bluemonday), Wilson score voting, subscriptions with auto-subscribe, trust-tier publishing (new_user→pending_review), rate limiting (per-entity + global), admin moderation (hide/restore/approve/reject/pending queue), auto-hide on 3+ reports, entity reports for comments. **Show field notes** (PSY-294/295): structured attendee reflections with verified attendee badges, star ratings (sound/crowd), spoiler handling, position-based sort. |
| User features | Accounts, saved shows, ICS calendar feed, favorite venues/cities, show reminders. **Contributor profiles** (PSY-63/64): public profiles, 3-level privacy, tier model, **profile editing** (PSY-261, username/name/bio). **Going/interested** (PSY-55): full stack DONE. **Follow system** (PSY-56): full stack DONE. **Entity edit drawer** (PSY-127): community edit suggestions with pending queue. **Entity reports** (PSY-131): flag issues on artists/venues/festivals/comments. **Attribution** (PSY-136): "Last edited by" on detail pages. |
| Radio         | **Phase 2d COMPLETE** + provider bug fixes (PSY-276–278): 3 providers (KEXP, WFMU, NTS) all fixed and working. **Historical import infrastructure** (PSY-272/273): show-level import with async job system, progress tracking, cancellation. Provider `FetchPlaylist` bugs fixed (NTS tracklist endpoint, KEXP time-range filtering, WFMU archive page parsing). |
| Admin         | Show/venue approval workflows, batch approve/reject (PSY-81), audit log, discovery imports, release/label/festival CRUD, collection management. Data quality dashboard (PSY-45), tag admin (PSY-46), artist merge/split (PSY-47): all DONE. **Platform analytics** (PSY-48): backend DONE. **Comment moderation** (PSY-292/293): pending comment queue, trust-tier visibility, auto-hide, admin hide/restore/approve/reject. Phase 1.7 dashboard UX polish: all DONE. |
| Auth          | Email/password, magic link, OAuth (Google/GitHub), passkeys (WebAuthn)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Discovery     | **AI-first pipeline operational.** Full end-to-end: venue config → tiered fetch (static/dynamic/screenshot via chromedp) → change detection → AI extraction (Claude Haiku) → non-music filtering → import with per-venue auto-approve control. Admin trigger endpoints live. **Automated scheduler** (PSY-31): background service with worker pool, circuit breaker, anomaly detection, Discord notifications. **Phase 1.6a COMPLETE.** **Phase 1.6b:** PSY-30 (AI billing) DONE, PSY-31 (scheduler) DONE. **PSY-58 (iCal/RSS feeds) DONE.** Remaining: PSY-33 (consolidate discovery UI — may be covered by PSY-36), PSY-35 (post-import enrichment). |
| Frontend      | Sidebar nav, **Cmd+K command palette with entity search** (PSY-257), redesigned show/artist/venue cards, density toggle, EntityDetailLayout template. **Feature modules** (`features/`): co-located components/hooks/types for all domains including **comments** (`features/comments/`). Browse/detail pages for collections (with tabs/search/filters), requests, tags, scenes. Revision history + attribution on entity detail pages. Library page. Charts page. **Comments on all 7 entity types** with voting, threading, replies. **Show field notes** with star ratings, verified badges, spoiler handling. **"Add to Collection" picker** on all entity pages. **"In Collections" backlinks** on all entity pages. |
| Data seeding  | MusicBrainz CLI, Bandcamp enrichment CLI, Festival data entry CLI (all human-run with --dry-run)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Testing       | 69 E2E, 68.5%+ backend coverage, ~2500+ frontend unit tests (190+ test files). **Dogfood QA**: all user journeys tested. **Collections UX audit** (April 2026): full dogfood with prior-art benchmarking against Letterboxd/What.cd/Discogs. |
| Observability | PostHog analytics, Sentry error tracking                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| iOS           | Code complete (39 files), not shipped — needs Apple Developer enrollment                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |


**All phases through Phase 3 COMPLETE.** Phase 1.5 (entities), 1.6 (pipeline), 1.7 (admin UX), 2a-2d (community, tags, engagement, radio), 3 (contribution flows) — all shipped.

**Recently shipped (April 2026):**
- **Radio historical import**: Show-level discovery (PSY-272), async import jobs with progress tracking (PSY-273), provider audit doc (PSY-274)
- **Radio provider bug fixes**: NTS tracklist endpoint (PSY-276), KEXP time-range filtering (PSY-277), WFMU archive page parsing (PSY-278)
- **Collections UX overhaul**: Add/remove items (PSY-314), reorder + notes (PSY-315), browse discovery (PSY-316), entity backlinks + profiles (PSY-317), share + timestamps (PSY-318)
- **Comments Wave 1-5**: Schema + CRUD (PSY-285), handlers (PSY-286), voting + Wilson score (PSY-287), subscriptions (PSY-288), frontend module + entity integration (PSY-290/291), moderation backend (PSY-292), moderation UI (PSY-293), field notes backend (PSY-294), field notes frontend (PSY-295)
- **Housekeeping**: Radio import dedup refactor, data cleanup runbook, curated list prior-art research doc

**Next up:**
- **Comments Wave 6:** PSY-296 (per-author reply permissions), PSY-297 (admin edit history viewer)
- **Comment notifications:** PSY-289 (notifications + mention parsing)
- **Collections polish:** PSY-319 (terminology, cover art, tags)
- **Radio:** PSY-279 (provider `until` parameter), execute data cleanup runbook
- **Strategic:** UX gap analysis priorities — edition-level release provenance, ratings, "your impact" metrics, ranked lists, knowledge graph export, structured setlists

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
| Comments, discussion, field notes, voting, subscriptions, moderation           | `docs/strategy/comments-and-field-notes.md`                  |
| Collections UX, curated lists, prior-art patterns                              | `docs/learnings/curated-list-prior-art.md`                   |
| Radio stations, radio shows, playlist parsing, "as heard on", co-occurrence    | `docs/strategy/radio-entities.md`                            |
| Radio data cleanup, re-import after provider fixes                             | `docs/learnings/radio-data-cleanup-runbook.md`               |
| Radio provider backfill capabilities, API limits, historical data              | `docs/learnings/radio-provider-backfill-audit.md`            |
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
- **Migration numbering** — latest is 000066 (add_comment_moderation_fields); next is 000067

