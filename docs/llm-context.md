# LLM Context — Psychic Homily

> Read this first every session. Routes you to the right doc for your task.

## What Is This?

Psychic Homily is **the spiritual successor to What.cd and Oink** — the same obsessive community curation, knowledge graph, and music discovery features, rebuilt for the legal streaming era. Instead of hosting files, we link to Bandcamp, Spotify, Discogs, YouTube, and other legal sources. Our unique advantage: **live shows as the discovery gateway**. Shows lead to artists, artists lead to releases and labels, labels lead to more artists and more shows. The knowledge graph grows outward from the live experience — something What.cd could never offer.

See `docs/vision.md` for the full north star, What.cd feature mapping, and entity model.

## Current Checkpoint (March 2026)

**Where we are:** Phase 1.5 complete. **Major roadmap restructuring (March 2026):** Community foundations pulled forward from Phase 3 to Phase 2a, running in parallel with pipeline work. Insight from What.cd user analysis: community curation is the moat, not the data pipeline. Contributor identity, collections, and requests now ship in Phase 2a (March–May 2026), not Q4 2026. Pipeline foundation (Phase 1.6a) ships minimum viable (PSY-32, PSY-34, PSY-36); remaining pipeline work becomes a background track. Admin dashboard polish demoted to opportunistic.


| Area          | Status                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Core entities | Artists, Venues, Shows (many-to-many, slugs, search, filters), **Releases** (with artist roles + external links, full CRUD API + admin UI + frontend pages), **Labels** (with artist/release junctions, catalog numbers, full CRUD API + admin UI + frontend pages), **Festivals** (with billing tiers, multi-venue, full CRUD API + admin UI + frontend pages + data entry CLI). Enriched artist pages with discography + label affiliations + festival appearances. Planned: Tags, Scenes, Collections, Requests, Radio Stations, Radio Shows, Musicians, Promoters.                                                    |
| User features | Accounts, saved shows, ICS calendar feed, favorite venues/cities, show reminders (email, 24h before). Generic `user_bookmarks` table live (replaces per-entity tables). **Contributor profiles** (PSY-63): public profiles with contribution stats/history, 3-level granular privacy (visible/count\_only/hidden per field), user tier model, custom profile sections (up to 3, markdown).                                                                                                                                                                                                                                    |
| Admin         | Show/venue approval workflows, pending edits, audit log, discovery imports, release/label/festival CRUD (admin-only API). Phase 2b: "Needs Work" data quality dashboard, tag admin, artist merge/split (PSY-45–47). Phase 2c: platform analytics (PSY-48). Phase 1.7 (opportunistic): dashboard UX polish (PSY-37–44). Phase 3: unified moderation queue, trust tiers, contributor leaderboard.                                                                                                                                                                                                                           |
| Auth          | Email/password, magic link, OAuth (Google/GitHub), passkeys (WebAuthn)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Discovery     | **AI-first, multi-source pipeline.** PSY-29 audit: 5/7 Playwright scrapers broken. New strategy: iCal/RSS feeds (cheapest) → HTTP JSON-LD → AI web extraction (universal, ~$0.01-0.03/page) → ticketing APIs (enrichment/validation). **Tiered rendering:** static HTTP → chromedp dynamic → chromedp screenshot (auto-detected per venue). Most venue sites are JS-rendered or anti-scrape protected — chromedp (pure Go headless Chrome) is the default path. Change detection skips unchanged pages. APIs for cross-referencing, not primary. Legacy Playwright scrapers deprecated. Phase 1.6: PSY-29 through PSY-36. |
| Frontend      | Sidebar nav, Cmd+K command palette, redesigned show/artist/venue cards, density toggle, EntityDetailLayout template, festival listing + detail pages with tiered lineup                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Data seeding  | MusicBrainz CLI, Bandcamp enrichment CLI, Festival data entry CLI (all human-run with --dry-run)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Testing       | 69 E2E, 68.5%+ backend coverage, 897+ frontend unit tests                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Observability | PostHog analytics, Sentry error tracking                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| iOS           | Code complete (39 files), not shipped — needs Apple Developer enrollment                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |


**Phase 1.5 complete.** All 24 issues (PSY-5 through PSY-28) shipped.

**Recently shipped:** Contributor profile backend with 58 tests (PSY-63), festival frontend pages (PSY-27), festival admin UI (PSY-26), festival service/handlers with 82 tests (PSY-25), festival model (PSY-24), TIMESTAMPTZ standardization (PSY-23), generic bookmarks (PSY-22), enriched artist pages (PSY-13), MusicBrainz + Bandcamp seeding CLIs (PSY-14, PSY-15), festival data entry CLI (PSY-28).

**Next up (restructured):**
- **Phase 1.6a + 2a (parallel, NOW):** Pipeline foundation (PSY-32 AI extraction, PSY-34 provenance, PSY-36 venue config) running parallel with community foundations (~~PSY-63~~ DONE, PSY-64 contributor profile frontend, PSY-65–68 collections, PSY-70–72 requests, PSY-73/74 revision history).
- **Phase 2b (May–July):** Tags (PSY-49/50/51/46), artist relationships (PSY-52/53), scenes (PSY-59/60), "Needs Work" dashboard (PSY-45), artist merge/split (PSY-47), bill position (PSY-54).
- **Phase 2c (July–Aug):** Going/interested (PSY-55), follow (PSY-56), top charts (PSY-57), notification filters, venue profiles (PSY-61/62), platform analytics (PSY-48).
- **Phase 1.6b (background):** Pipeline maturation (PSY-30/31/33/35). **Phase 1.7 (opportunistic):** Admin polish (PSY-37–44).
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
- **Huma quirk** — all request body fields required by default, even pointers; mark optional explicitly
- **Migration numbering** — latest is 000044 (auto_approve_default_false, PSY-80); next is 000045

