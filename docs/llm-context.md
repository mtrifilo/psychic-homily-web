# LLM Context — Psychic Homily

> Read this first every session. Routes you to the right doc for your task.

## What Is This?

Psychic Homily is **the spiritual successor to What.cd and Oink** — the same obsessive community curation, knowledge graph, and music discovery features, rebuilt for the legal streaming era. Instead of hosting files, we link to Bandcamp, Spotify, Discogs, YouTube, and other legal sources. Our unique advantage: **live shows as the discovery gateway**. Shows lead to artists, artists lead to releases and labels, labels lead to more artists and more shows. The knowledge graph grows outward from the live experience — something What.cd could never offer.

See `docs/vision.md` for the full north star, What.cd feature mapping, and entity model.

## Current Checkpoint (March 2026)

**Where we are:** Phase 2a community foundations **CODING COMPLETE** — all 12 issues shipped (PSY-63–74). Phase 2b knowledge graph connective tissue **STARTED** — tag model (PSY-49) and artist relationship model (PSY-52) shipped, unlocking service layers next. Phase 1.6b pipeline maturation progressing — AI billing (PSY-30) and automated scheduler (PSY-31) shipped. Code reorg fully complete. Community curation is the moat, not the data pipeline — insight from What.cd user analysis drove Phase 2a forward from Q4 to March 2026.


| Area          | Status                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Core entities | Artists, Venues, Shows (many-to-many, slugs, search, filters), **Releases** (with artist roles + external links, full CRUD API + admin UI + frontend pages), **Labels** (with artist/release junctions, catalog numbers, full CRUD API + admin UI + frontend pages), **Festivals** (with billing tiers, multi-venue, full CRUD API + admin UI + frontend pages + data entry CLI). Enriched artist pages with discography + label affiliations + festival appearances. **Collections** (PSY-65–68): full stack DONE. **Requests** (PSY-70–72): model, service, handlers, frontend ALL DONE — `/requests` browse + detail pages with voting, create, fulfill, close. **Revisions** (PSY-73–74): model, service, handlers, frontend ALL DONE — revision history on artist/venue detail pages with field-level diffs and admin rollback. **Tags** (PSY-49): model + migration DONE (4 tables). **Artist relationships** (PSY-52): model + migration DONE (2 tables). **Data provenance** on all 6 core entity tables (PSY-34). Planned: Tag service/UI, artist relationship service/visualization, Scenes, Radio Stations, Radio Shows, Musicians, Promoters. |
| User features | Accounts, saved shows, ICS calendar feed, favorite venues/cities, show reminders (email, 24h before). Generic `user_bookmarks` table live (replaces per-entity tables). **Contributor profiles** (PSY-63/64): public profiles at `/users/:username` with contribution stats/history, 3-level granular privacy (visible/count\_only/hidden per field), user tier model, custom profile sections (up to 3, markdown). Frontend with identity hub, privacy controls UI, sections editor.                                                                                                                                           |
| Admin         | Show/venue approval workflows, **batch approve/reject with rejection categories** (PSY-81), pending edits, audit log, discovery imports, release/label/festival CRUD (admin-only API), **collection management** (PSY-67). Phase 2b: "Needs Work" data quality dashboard, tag admin, artist merge/split (PSY-45–47). Phase 2c: platform analytics (PSY-48). Phase 1.7 (opportunistic): dashboard UX polish (PSY-37–44). Phase 3: unified moderation queue, trust tiers, contributor leaderboard.                                                                                                                               |
| Auth          | Email/password, magic link, OAuth (Google/GitHub), passkeys (WebAuthn)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Discovery     | **AI-first pipeline operational.** Full end-to-end: venue config → tiered fetch (static/dynamic/screenshot via chromedp) → change detection → AI extraction (Claude Haiku) → non-music filtering → import with per-venue auto-approve control. Admin trigger endpoints live. **Automated scheduler** (PSY-31): background service with worker pool, circuit breaker, anomaly detection, Discord notifications. **Phase 1.6a COMPLETE.** **Phase 1.6b:** PSY-30 (AI billing) DONE, PSY-31 (scheduler) DONE. Remaining: PSY-33 (consolidate discovery UI — may be covered by PSY-36), PSY-35 (post-import enrichment), PSY-58 (iCal/RSS auto-detection). |
| Frontend      | Sidebar nav, Cmd+K command palette, redesigned show/artist/venue cards, density toggle, EntityDetailLayout template, festival listing + detail pages with tiered lineup. **Feature modules** (`features/`): co-located components/hooks/types for all features — releases, labels, festivals, blog, auth, collections, requests, shows, artists, venues. Collection browse/detail at `/collections`. Request browse/detail at `/requests` with voting + filters. Revision history on entity detail pages. |
| Data seeding  | MusicBrainz CLI, Bandcamp enrichment CLI, Festival data entry CLI (all human-run with --dry-run)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Testing       | 69 E2E, 68.5%+ backend coverage, 1400+ frontend unit tests                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Observability | PostHog analytics, Sentry error tracking                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| iOS           | Code complete (39 files), not shipped — needs Apple Developer enrollment                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |


**Phase 1.5 complete.** All 24 issues (PSY-5 through PSY-28) shipped.
**Phase 1.6a COMPLETE.** All pipeline foundation issues shipped (PSY-29, PSY-75–83, PSY-34, PSY-36).
**Phase 2a CODING COMPLETE.** All 12 community foundation issues shipped (PSY-63–74).
**Code reorg COMPLETE.** Backend services reorg (PSY-84–92) and frontend feature modules (PSY-93–103) all shipped.
**Phase 2b STARTED.** Tag model (PSY-49) and artist relationship model (PSY-52) shipped.

**Recently shipped (March 2026):**
- Pipeline: PSY-75–83 (full pipeline stack), PSY-34 (data provenance), PSY-36 (venue config admin UI), PSY-30 (AI billing), PSY-31 (automated scheduler with worker pool + anomaly detection)
- Community: PSY-63/64 (contributor profiles), PSY-65–68 (collections full stack), PSY-70–72 (request system full stack — model, service with Wilson score voting, handlers, frontend with browse/detail/voting/fulfillment), PSY-73–74 (revision history full stack — model, service, handlers, frontend with field-level diffs on artist/venue pages)
- Code reorg: PSY-84–92 (backend services), PSY-93–103 (frontend feature modules)
- Phase 2b foundations: PSY-49 (tag model — 4 tables: tags, entity_tags, tag_votes, tag_aliases), PSY-52 (artist relationship model — 2 tables with canonical ordering + Wilson score)

**Next up:**
- **Phase 2a exit criteria (remaining non-code work):** Percentile rankings on profiles, "Added by [username]" attribution, "Your Impact" metrics, contribution heatmap, seed collections + auto-generated requests (data entry).
- **Phase 2b (in progress):** ~~PSY-49~~ ~~PSY-52~~ DONE (models). Next: PSY-50 (tag service), PSY-53 (artist relationship service), PSY-47 (artist merge/split), PSY-54 (bill position), PSY-59 (scene backend). Then: PSY-51 (tag frontend), PSY-46 (tag admin), PSY-60 (scene frontend), PSY-45 ("Needs Work" dashboard).
- **Phase 1.6b (background):** ~~PSY-30~~ ~~PSY-31~~ DONE. Remaining: PSY-33, PSY-35, PSY-58.
- **Phase 2c (July–Aug):** Going/interested (PSY-55), follow (PSY-56), top charts (PSY-57), notification filters, venue profiles (PSY-61/62), platform analytics (PSY-48).
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
- **Migration numbering** — latest is 000052 (create_artist_relationships, PSY-52); next is 000053

