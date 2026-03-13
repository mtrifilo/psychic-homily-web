# LLM Context — Psychic Homily

> Read this first every session. Routes you to the right doc for your task.

## What Is This?

Psychic Homily is **the spiritual successor to What.cd and Oink** — the same obsessive community curation, knowledge graph, and music discovery features, rebuilt for the legal streaming era. Instead of hosting files, we link to Bandcamp, Spotify, Discogs, YouTube, and other legal sources. Our unique advantage: **live shows as the discovery gateway**. Shows lead to artists, artists lead to releases and labels, labels lead to more artists and more shows. The knowledge graph grows outward from the live experience — something What.cd could never offer.

See `docs/vision.md` for the full north star, What.cd feature mapping, and entity model.

## Current Checkpoint (March 2026)

**Where we are:** Phase 1.6a pipeline foundation **COMPLETE**. Phase 2a community foundations well advanced — contributor profiles, collections (model through frontend), request model, and revision history all shipped. Phase 1.6b pipeline maturation started (AI billing done, scheduler in progress). Code reorg fully complete. **Major roadmap restructuring (March 2026):** Community foundations pulled forward from Phase 3 to Phase 2a, running in parallel with pipeline work. Insight from What.cd user analysis: community curation is the moat, not the data pipeline. Contributor identity, collections, and requests now ship in Phase 2a (March–May 2026), not Q4 2026. Pipeline foundation (Phase 1.6a) ships minimum viable; remaining pipeline work becomes a background track. Admin dashboard polish demoted to opportunistic.


| Area          | Status                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Core entities | Artists, Venues, Shows (many-to-many, slugs, search, filters), **Releases** (with artist roles + external links, full CRUD API + admin UI + frontend pages), **Labels** (with artist/release junctions, catalog numbers, full CRUD API + admin UI + frontend pages), **Festivals** (with billing tiers, multi-venue, full CRUD API + admin UI + frontend pages + data entry CLI). Enriched artist pages with discography + label affiliations + festival appearances. **Collections** (PSY-65–68): model, service, admin UI, and public browse/detail pages DONE. **Requests** (PSY-70): model and migrations DONE, service/handlers in progress (PSY-71). **Revisions** (PSY-73): model and service DONE, handlers/frontend in progress (PSY-74). **Data provenance** on all 6 core entity tables (PSY-34). Planned: Tags, Scenes, Radio Stations, Radio Shows, Musicians, Promoters. |
| User features | Accounts, saved shows, ICS calendar feed, favorite venues/cities, show reminders (email, 24h before). Generic `user_bookmarks` table live (replaces per-entity tables). **Contributor profiles** (PSY-63/64): public profiles at `/users/:username` with contribution stats/history, 3-level granular privacy (visible/count\_only/hidden per field), user tier model, custom profile sections (up to 3, markdown). Frontend with identity hub, privacy controls UI, sections editor.                                                                                                                                           |
| Admin         | Show/venue approval workflows, **batch approve/reject with rejection categories** (PSY-81), pending edits, audit log, discovery imports, release/label/festival CRUD (admin-only API). Phase 2b: "Needs Work" data quality dashboard, tag admin, artist merge/split (PSY-45–47). Phase 2c: platform analytics (PSY-48). Phase 1.7 (opportunistic): dashboard UX polish (PSY-37–44). Phase 3: unified moderation queue, trust tiers, contributor leaderboard.                                                                                                                                                               |
| Auth          | Email/password, magic link, OAuth (Google/GitHub), passkeys (WebAuthn)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Discovery     | **AI-first pipeline operational.** Full end-to-end: venue config → tiered fetch (static/dynamic/screenshot via chromedp) → change detection → AI extraction (Claude Haiku) → non-music filtering → import with per-venue auto-approve control. Admin trigger endpoints live. **Phase 1.6a COMPLETE:** venue source config + extraction runs (PSY-75), calendar extraction prompt (PSY-76), HTTP fetcher with ETag/hash change detection (PSY-77), chromedp rendering (PSY-78), pipeline orchestrator (PSY-79), auto-approve wiring + non-music filtering (PSY-80), batch review UI (PSY-81), rejection feedback loop (PSY-82), data provenance (PSY-34), venue config admin UI with full CRUD (PSY-36). **Phase 1.6b in progress:** AI billing enrichment DONE (PSY-30), automated extraction scheduler in progress (PSY-31). Remaining: PSY-33 (consolidate discovery UI), PSY-35 (post-import enrichment), PSY-58 (iCal/RSS auto-detection). |
| Frontend      | Sidebar nav, Cmd+K command palette, redesigned show/artist/venue cards, density toggle, EntityDetailLayout template, festival listing + detail pages with tiered lineup. **Feature modules** (`features/`): co-located components/hooks/types for all features — releases, labels, festivals, blog, auth, collections, shows, artists, venues. **Code reorg COMPLETE** (PSY-93–103). Collection browse/detail pages at `/collections`. |
| Data seeding  | MusicBrainz CLI, Bandcamp enrichment CLI, Festival data entry CLI (all human-run with --dry-run)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Testing       | 69 E2E, 68.5%+ backend coverage, 1400+ frontend unit tests                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Observability | PostHog analytics, Sentry error tracking                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| iOS           | Code complete (39 files), not shipped — needs Apple Developer enrollment                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |


**Phase 1.5 complete.** All 24 issues (PSY-5 through PSY-28) shipped.
**Phase 1.6a COMPLETE.** All pipeline foundation issues shipped (PSY-29, PSY-75–83, PSY-34, PSY-36).
**Code reorg COMPLETE.** Backend services reorg (PSY-84–92) and frontend feature modules (PSY-93–103) all shipped.

**Recently shipped (March 2026):**
- Pipeline foundation: PSY-75–83 (full pipeline stack), PSY-34 (data provenance on all core tables), PSY-36 (venue config admin UI with full CRUD, run history, reset render method)
- AI billing enrichment: PSY-30 (headliner/support detection in extraction prompts)
- Community foundations: PSY-63/64 (contributor profiles), PSY-65/66 (collection model + service), PSY-67/68 (collection admin UI + browse/detail pages), PSY-70 (request model), PSY-73 (revision history model + service)
- Code reorg: PSY-84–92 (backend services into domain sub-packages), PSY-93–103 (frontend feature modules for all domains)

**Next up:**
- **Phase 2a (in progress):** ~~PSY-63~~ ~~PSY-64~~ ~~PSY-65~~ ~~PSY-66~~ ~~PSY-67~~ ~~PSY-68~~ ~~PSY-70~~ ~~PSY-73~~ DONE. In progress: PSY-71 (request service/handlers), PSY-74 (revision history handlers/frontend). Remaining: PSY-72 (request frontend).
- **Phase 1.6b (in progress):** ~~PSY-30~~ DONE. In progress: PSY-31 (automated extraction scheduler). Remaining: PSY-33 (consolidate discovery UI — may be largely covered by PSY-36), PSY-35 (post-import enrichment), PSY-58 (iCal/RSS auto-detection).
- **Phase 2b (May–July):** Tags (PSY-49/50/51/46), artist relationships (PSY-52/53), scenes (PSY-59/60), "Needs Work" dashboard (PSY-45), artist merge/split (PSY-47), bill position (PSY-54).
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
- **Migration numbering** — latest is 000050 (create_revisions, PSY-73); next is 000051

