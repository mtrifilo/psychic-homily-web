# Roadmap

> Quarterly priorities across all tracks. For the product vision and entity model, see `docs/vision.md`. For track-specific details, see individual track files.

## Guiding Principles

Live shows are the gateway into the knowledge graph. Every phase builds outward from the live experience: shows lead to artist discovery, which leads to releases and labels, which leads to more artists and more shows. The knowledge graph grows from the live foundation — never away from it.

**Community curation is the moat, not the data pipeline.** The pipeline produces raw material. Community tools produce the knowledge graph. What.cd's real value was built by thousands of obsessive volunteers working within quality constraints — not by automated ingestion. Build the tools that attract contributors before optimizing the pipeline that feeds them.

**Contributor identity is foundational infrastructure.** Attribution ("Added by [username]"), contribution profiles, and visible impact are not late-stage polish — they are the scaffolding that makes people invest. Every feature that allows user contribution ships with attribution from day one. A What.cd user contributed 4 of 5 albums for a Japanese band, importing CDs at personal expense. When the site died, he said "it's like part of me died." That level of ownership is what we're building toward. See `docs/learnings/whatcd-user-insights.md`.

**Ship what creates contributors before what creates passive users.** Tags, collections, and requests create active contributors. Going/Interested creates passive engagement. Prioritize the former.

**Quality standards attract, not repel.** What.cd's strictness was a magnet for the exact people who built the database. Open registration means quality assurance comes from reputation systems and review processes, not entrance barriers.

**Pipeline and community are parallel tracks.** They're independent work streams that reinforce each other. The pipeline feeds data in; the community deepens it. Neither should block the other.

## Current Priority Order

| Priority | Focus | What | Status |
|----------|-------|------|--------|
| 1a | Discovery | Phase 1.6a: AI pipeline foundation (PSY-29, PSY-75–81, PSY-83 DONE; PSY-82, PSY-34, PSY-36 remaining) | Nearly done |
| 1b | Community | Phase 2a: Community foundations — contributor identity, collections, requests, revision history (PSY-63 through PSY-74) | In progress (PSY-63 done) |
| 2 | Web | Phase 2b: Knowledge graph connective tissue — tags, relationships, scenes (PSY-45–54, PSY-59/60) | Planned |
| 3 | Web | Phase 2c: Engagement & social — going/interested, follow, charts, notifications (PSY-55–57, PSY-61/62) | Planned |
| — | Discovery | Phase 1.6b: Pipeline maturation (PSY-30, PSY-31, PSY-33, PSY-35) | Background track |
| — | Admin | Phase 1.7: Admin dashboard polish (PSY-37–44) | Opportunistic |
| 4 | Community | Phase 3: Community at scale — moderation, trust tiers, open edit flows | Planning |
| 5 | iOS | Polish, test, ship to App Store | Blocked (Apple Developer enrollment) |

---

## COMPLETED: Foundation + Knowledge Graph Vertical Slice (Phase 1 / 1.5)

**Theme:** Close out remaining utility work, then prove the What.cd vision with a vertical slice of the knowledge graph.

### Wrap Up (Phase 1 utility)
- [x] Artists list page with search + multi-city filters
- [x] Venues page search + multi-city filters
- [x] Calendar sync (ICS feed for saved shows)
- [x] Show reminders (email, 24h before, with one-click unsubscribe)
- [ ] Email preferences UI (in progress)

### Frontend Redesign (Phase 1)

Structural UI redesign to evolve from "show tracker" to "music knowledge graph." Full spec in `docs/strategy/ui-redesign.md`.

1. ~~**Sidebar navigation + wider layout**~~ — DONE (PSY-16, PR #11)
2. ~~**Cmd+K command palette**~~ — DONE (PSY-17, PR #13)
3. ~~**Entity detail page template**~~ — DONE (PSY-18)
4. ~~**Show card redesign**~~ — DONE (PSY-19, PR #14)
5. ~~**Artist card redesign**~~ — DONE (PSY-20)
6. ~~**Visual polish**~~ — DONE (PSY-21, PR #18)

### Data Layer Foundation + Knowledge Graph Vertical Slice (Phase 1.5)

1. ~~**Data layer foundation**~~ — DONE (PSY-22 generic bookmarks, PSY-23 TIMESTAMPTZ)
2. ~~**Festival entity**~~ — DONE (PSY-24 model, PSY-25 service/handlers 82 tests, PSY-26 admin UI, PSY-27 frontend pages, PSY-28 data entry CLI)
3. ~~**Releases entity**~~ — DONE (PSY-5 model, PSY-6 service 57 tests, PSY-7 admin UI, PSY-8 frontend pages)
4. ~~**Labels entity**~~ — DONE (PSY-9 model, PSY-10 service, PSY-11 admin UI, PSY-12 frontend pages)
5. ~~**Artist pages enriched**~~ — DONE (PSY-13)
6. ~~**Data seeding**~~ — DONE (PSY-14 MusicBrainz CLI, PSY-15 Bandcamp CLI, PSY-28 festival entry CLI)

**Phase 1.5 exit criteria met.** All 24 issues (PSY-5 through PSY-28) shipped.

### iOS (blocked)
- [ ] Apple Developer enrollment ($99/year)
- [ ] Re-enable capabilities, polish, TestFlight → App Store

---

## NOW: Pipeline Foundation + Community Foundations (Phase 1.6a + Phase 2a)

These two tracks run **in parallel**. Phase 1.6a is backend-heavy pipeline infrastructure. Phase 2a is the community features that make PH more than a listing site. They are independent work streams.

### Phase 1.6a: Discovery Pipeline Foundation (March–April 2026)

**Theme:** Get the AI extraction pipeline working well enough that data flows reliably, then shift primary engineering focus to community tools. The remaining pipeline issues become a background track.

**Linear project:** "Phase 1.6: Discovery Pipeline & Data Quality" (PSY-29 through PSY-36)

**Data source tiers (resilience + scalability):**
1. iCal/RSS feeds — free, structured, standard, most reliable when available
2. HTTP JSON-LD — free, structured, no browser needed (schema.org markup)
3. AI web extraction — universal, ~$0.01-0.03/page, works on any website
4. Ticketing platform APIs — enrichment + cross-referencing (SeatGeek, Bandsintown, Ticketmaster)
5. Festival lineups + community submissions — human-powered gap-filling

**What shipped (pipeline sprint, March 2026):**

1. ~~**Provider reliability audit** (PSY-29)~~ — DONE. 2/7 passing. Conclusion: pivot to AI-first.
2. ~~**Venue source config + extraction runs** (PSY-75)~~ — DONE. `venue_source_configs` and `venue_extraction_runs` tables with full service layer.
3. ~~**Calendar extraction prompt** (PSY-76)~~ — DONE. Claude Haiku prompt for multi-event calendar pages with `IsMusicEvent` classification.
4. ~~**HTTP fetcher with change detection** (PSY-77)~~ — DONE. ETag/hash-based change detection, skips unchanged pages.
5. ~~**chromedp rendering** (PSY-78)~~ — DONE. Tiered: static HTTP → chromedp dynamic → chromedp screenshot. Auto-detected per venue.
6. ~~**Pipeline orchestrator** (PSY-79)~~ — DONE. `PipelineService` orchestrates end-to-end: config → fetch → detect changes → extract → import. Admin trigger via `POST /admin/pipeline/extract`.
7. ~~**Auto-approve wiring + non-music filtering** (PSY-80)~~ — DONE. Per-venue `auto_approve` flag (default false). AI-classified `IsMusicEvent` filtering before import. `ImportEvents` threaded with `initialStatus`.
8. ~~**Batch review admin UI** (PSY-81)~~ — DONE. Batch approve/reject endpoints. `rejection_category` column. Frontend: batch selection, filter bar, keyboard shortcuts, quick "Not Music" reject.
9. ~~**Huma query param fix** (PSY-83)~~ — DONE. Fixed pointer type panic in query params.

**What remains:**
- **PSY-82** (Rejection feedback loop) — venue-level rejection stats, extraction notes, auto-generated hints
- **PSY-34** (Data provenance tracking) — `data_source`, `source_confidence`, `last_verified_at` on core entity tables
- **PSY-36** (Venue source config admin UI) — admin interface for managing venue configs

**What defers to Phase 1.6b (background track):**
- PSY-30 (AI billing enrichment) — useful but not blocking
- PSY-31 (Automated scheduling) — manual triggering suffices initially
- PSY-33 (Consolidate discovery UI) — admin convenience
- PSY-35 (Post-import enrichment pipeline) — manual seeding CLIs work today
- iCal/RSS feed detection — opportunistic, per-venue

**Exit criteria:** AI extraction working on 5+ venues. Data provenance on core tables. Venue source config persisted. Change detection reducing unnecessary extractions.

---

### Phase 2a: Community Foundations (March–May 2026, parallel with 1.6a)

**Theme:** Build the three pillars that made What.cd irreplaceable: **contributor identity**, **collections**, and the **request system**. These are the features that turn passive browsers into invested community members. Plus revision history to enable accountable editing.

**Linear project:** "Phase 2a: Community Foundations" (PSY-63 through PSY-74)

**Why now (not Phase 3):** The current roadmap deferred all community features to Q4 2026 — a full year where the app is a read-only browsing experience. But What.cd's moat was never the data; it was the community that built and maintained the data. Contributor identity, collections, and requests are not "community features to add later" — they are the foundation that makes everything else matter. Every subsequent feature (tags, scenes, charts) is more powerful when built on contributor identity.

**Build order:**

#### 1. Contributor Identity & Profile System (PSY-63, PSY-64)

The most important new work. Not a feature in itself — infrastructure that makes every subsequent community feature create ownership. Gazelle's profile pages were dense identity hubs where users saw themselves reflected in the knowledge graph. Our current profile is a settings form — this transforms it into the center of contributor identity. *(Deep analysis: `docs/learnings/gazelle-user-profiles.md`)*

- ~~**PSY-63: Contributor profile model and API**~~ — **DONE** (58 tests, migrations 000040–000041)
  - **Contribution tracking:** Contribution counts materialized from `audit_log` + entity tables via UNION ALL query. Tracks: shows submitted, venues submitted, venue edits, releases created, labels created, festivals created, artists edited, moderation actions.
  - **Public profile endpoint:** `GET /users/:username` with optional auth. Returns identity header (username, avatar, join date, tier), contribution stats (privacy-gated), recent activity, custom profile sections. Owner sees everything; non-owner sees per-field privacy-gated data.
  - **Privacy controls:** `privacy_settings` JSONB column on users with 3-level per-field visibility: `visible` (full data), `count_only` (aggregate count only), `hidden` (omitted). 7 fields: contributions, saved_shows, attendance, following, collections, last_active, profile_sections. Binary-only fields (last_active, profile_sections) reject count_only. Master switch via `profile_visibility` (public/private). Owner always sees all own data.
  - **User tier model:** `user_tier` VARCHAR on users (new_user/contributor/trusted_contributor/local_ambassador). Displayed on profile. Initially manual; auto-promotion scheduler deferred to Phase 3.
  - **Custom profile sections:** `user_profile_sections` table with CRUD API. Max 3 sections per user, markdown content (up to 10000 chars), position ordering, visibility toggle. Owner sees all sections; public viewers see only visible ones.
  - **11 API endpoints:** 3 public with optional auth (`/users/{username}`, `/users/{username}/contributions`, `/users/{username}/sections`), 8 protected (`/auth/profile/contributor`, `/auth/profile/contributions`, `/auth/profile/visibility`, `/auth/profile/privacy`, `/auth/profile/sections` CRUD).
  - **Deferred to later:** Percentile rankings (Phase 2a exit criteria, additive), impact metrics (downstream views/saves, additive), contribution heatmap (frontend, PSY-64).

- **PSY-64: Contributor profile frontend**
  - **Public profile page** (`/users/:username`): Identity header with avatar + tier badge + join date. Contribution summary cards by type. Visual recent activity feed (entity images, not just text). Collections showcase (visual grid thumbnails). Customizable profile sections (markdown rendered). Percentile ranking display. Contribution heatmap (GitHub-style activity calendar).
  - **"Added by [username]"** attribution on entity detail pages (shows, artists, releases, labels, venues). Links to contributor's public profile.
  - **"Your Impact"** section on private profile: downstream metrics, rank progress (what you need to reach next tier), privacy controls management.
  - **Profile transformation:** Current `/profile` page evolves from bare settings into tabbed layout: Profile (public identity view) | Contributions (detailed history + stats) | Settings (existing settings panel).

#### 2. Collections (PSY-65 through PSY-68)

The primary vessel for community knowledge — a record store at scale. No prescribed categories — collectors use their imagination: "Artists who performed at The Smell in LA 2010-2012", "Pitchfork's Top 100 Albums of the 2010s", "African Psychedelic Punk", "Creation Records 1984-1999", "Shows at Crescent Ballroom that changed my life", "The Phoenix scene in 2019." Open to collaboration by default. Visual grids. Subscribable. Each collection is someone's expertise made navigable — the description is the narrative, the items are the evidence, the graph connections are the context.

- **PSY-65: Collection model and migrations** — `collections` table (title, slug, description, creator_id, collaborative, cover_image_url, is_public), `collection_items` table (entity_type, entity_id, position, added_by_user_id, notes), `collection_subscribers` table (last_visited_at for Gazelle-style unread tracking)
- **PSY-66: Collection service and handlers** — CRUD, item management (add/remove/reorder), subscription, permission model (creator edits, collaborators add items, anyone subscribes, admin features), aggregate stats (item count, contributor count, top tags once tags exist)
- **PSY-67: Collection admin UI** — admin tab for managing featured collections, moderation
- **PSY-68: Collection frontend pages** — `/collections` listing with search, `/collections/:slug` detail with visual grid of entity images (album art, venue photos, artist images, show flyers), "Add to collection" flow on entity detail pages, collection display on contributor profiles
#### 3. Request System (PSY-70 through PSY-72)

The mechanism that converts passive browsers into active contributors. What.cd users bought and imported rare CDs because someone else requested them. Requests are calls to action, not wishlists.

- **PSY-70: Request model and migrations** — `requests` table (title, description, entity_type, requested_entity_id nullable, status, requester_id, fulfiller_id, vote_score), `request_votes` table (vote up/down)
- **PSY-71: Request service and handlers** — CRUD, voting with Wilson score, fulfillment workflow linking contribution to request, auto-generated requests from data quality queries ("This label has 12 artists but only 3 have releases", "This festival has 40 artists but only 10 are in our database")
- **PSY-72: Request frontend pages** — `/requests` listing with filters (entity_type, status, sort by votes), request detail, "Request" buttons on entity pages ("This artist needs releases"), fulfillment flow, request feed on contributor profile

#### 4. Revision History (PSY-73, PSY-74)

Required before opening edit flows to non-admin users. Accountability and rollback capability.

- **PSY-73: Revision history model and service** — `revisions` table (entity_type, entity_id, user_id, field_changes JSONB diff, summary), automatic diff generation on entity updates, history API endpoint
- **PSY-74: Revision history frontend** — "View History" link on entity detail pages, chronological edit list with field-level diffs, admin one-click rollback

**Practical sprint sequence (single-person team):**
```
Sprint 1: ~~PSY-75–81, PSY-83 (pipeline foundation)~~ DONE + ~~PSY-63 (contributor profile backend)~~ DONE
Sprint 2: PSY-82 (rejection feedback) + PSY-34 + PSY-36 (provenance + venue config UI) + PSY-64 (contributor profile frontend)
Sprint 3: PSY-65 + PSY-66 (collections model + service)
Sprint 4: PSY-67 + PSY-68 (collections admin + frontend)
Sprint 5: PSY-70 + PSY-71 (requests model + service) + PSY-73 (revision history backend)
Sprint 6: PSY-72 (requests frontend) + PSY-74 (revision history frontend)
```

**Phase 2a exit criteria:**
- Public contributor profiles live at `/users/:username` with identity header, contribution summary, visual recent activity, collections showcase, and customizable profile sections
- Percentile rankings computed and displayed (at least 3 dimensions: shows, edits, collections)
- Privacy controls live — users can control per-field visibility of contribution history, saved shows, following list
- User tier model in place (new_user / contributor achievable; higher tiers display but require Phase 3 auto-promotion)
- "Added by [username]" attribution on all user-contributed entities
- "Your Impact" section on private profile with downstream metrics
- Contribution heatmap on profile
- Collections CRUD with visual grid display, collaboration, subscriptions, contributor attribution
- At least 10 seed collections created across diverse organizing principles
- Request system with voting, fulfillment tracking, and auto-generated requests
- At least 20 auto-generated requests from data quality queries
- Revision history on all entity types with admin rollback
- "View History" link on all entity detail pages

---

## NEXT: Knowledge Graph Connective Tissue (Phase 2b, May–July 2026)

**Theme:** Build the connections that make the graph navigable: tags, relationships, and scene pages. These are correctly placed — but now they build on contributor identity infrastructure. Every tag applied shows who applied it, every similar artist vote is attributed, every scene has its contributors visible.

**Linear project:** "Phase 2: Knowledge Graph Expansion" (existing PSY-45 through PSY-62)

### Tags & Voting
- [ ] **Tag model, migration, GORM structs** (PSY-49) — `tags`, `entity_tags`, `tag_votes`, `tag_aliases` tables. Hierarchical taxonomy with parent-child. Official tags immune to pruning.
- [ ] **Tag service, handlers, API routes** (PSY-50) — CRUD, voting with Wilson score ranking, alias resolution, auto-pruning (CleanupService every 15 min). Tag applications show contributor attribution.
- [ ] **Tag frontend** (PSY-51) — `/tags` index, `/tags/:slug` detail, tag pills with voting on all entity pages, "add tag" autocomplete, tag filtering on list pages. "[View tagging rules]" link visible on every tag input.
- [ ] **Tag administration UI** (PSY-46) — admin tag CRUD, alias management, bulk merge/rename, pruning config, moderation queue.

### Artist Relationships
- [ ] **Artist relationship model** (PSY-52) — `artist_relationships` + `artist_relationship_votes` tables. Types: similar, side_project, member_of. Auto-derived "shared bills" flag.
- [ ] **Similar artist voting service** (PSY-53) — vote on similarity (up/down), Wilson score ranking, auto-derived "shared bills" job (daily, from `show_artists` co-occurrences). The iconic What.cd feature — and PH adds empirical "shared bills" data from live shows that What.cd never had.
- [ ] **Similar artist visualization** — interactive relationship map. Font size/node size proportional to similarity. Edge thickness proportional to score. Cross-connections between similar artists. *(design doc: `docs/strategy/similar-artists.md`)*

### Scene Pages (Tier 1) *(design doc: `docs/strategy/scene-pages.md`)*
- [ ] **Scene page backend** (PSY-59) — `SceneService` with computed city aggregations. Virtual scenes, no new table. Data thresholds: 3+ venues AND 5+ upcoming shows.
- [ ] **Scene page frontend** (PSY-60) — `/scenes` list, `/scenes/:slug` detail with Scene Pulse (shows/month sparkline, new artists, active venues, trend arrows), upcoming shows, top venues, active artists, festivals. Scene pages link to Scene Guide collections.

### Admin Data Quality & Entity Management
- [ ] **"Needs Work" data quality dashboard** (PSY-45) — query-driven lists of incomplete entities ranked by impact. Each item one-click to fix. Feeds auto-generated requests into the request system (Phase 2a). Completion percentage tracked over time.
- [ ] **Artist merge/split tool** (PSY-47) — merge duplicates (reassign relationships, create alias redirect, audit log), split misidentified artists. `artist_aliases` table. Critical before geographic expansion.

### Show & Festival Data Enrichment
- [ ] **Bill position surfacing** (PSY-54) — expose `set_type` and `position` in API responses, render billing roles on show detail/cards, admin reordering UI. Schema exists since migration 000001.
- [ ] **Decade/year entity rankings** — temporal rankings from community votes, scoped by decade and year (What.cd's "No. 96 for the 1970s, No. 6 for 1976" pattern). Wilson score per time scope, cached with TTL.
- [ ] **Festival intelligence** — lineup overlap, "artists you follow at this festival", breakout tracking. *(design doc: `docs/strategy/festival-intelligence.md`)*
- [ ] **Show-to-recording links** — connect live recordings to show entities

**Build order:**
1. **Independent foundations (can parallel):** PSY-49 (tag model), PSY-52 (artist relationship model), PSY-47 (artist merge/split), PSY-54 (bill position), PSY-59 (scene backend)
2. **Depends on models:** PSY-50 (tag service, needs PSY-49), PSY-53 (similar artist service, needs PSY-52)
3. **Depends on services:** PSY-51 (tag frontend, needs PSY-50), PSY-46 (tag admin, needs PSY-50), PSY-60 (scene frontend, needs PSY-59)
4. **Integration:** PSY-45 "Needs Work" dashboard (integrates with request system from Phase 2a), collection-tag aggregation (after PSY-51)

**Design docs completed:**
- Scene pages → `docs/strategy/scene-pages.md`
- Similar artist visualization → `docs/strategy/similar-artists.md`
- Festival intelligence → `docs/strategy/festival-intelligence.md`

**Phase 2b exit criteria:** Genre tags on 80%+ of artists with community voting active. Similar artist graph navigable with voting and "shared bills." Scene page live for Phoenix. "Needs Work" dashboard generating auto-requests. Bill position surfaced in UI. Artist merge/split operational. Tag aliases managing variant spellings.

---

## THEN: Engagement & Social (Phase 2c, July–August 2026)

**Theme:** The knowledge graph has depth (entities, tags, relationships, scenes, collections) and the community has identity (profiles, attribution, contributions). Now add the social and engagement features that make people come back daily.

### Engagement
- [ ] **Going/Interested buttons** (PSY-55) — toggle on shows/festivals using existing `user_bookmarks` (no migration). Attendance counts on cards and detail pages. "My Shows" view.
- [ ] **Follow system** (PSY-56) — follow artists, venues, labels, collections, users via `user_bookmarks` with `action = 'follow'`. Follower counts on entity pages. "Following" section on profile.
- [ ] **Top charts** (PSY-57) — trending shows (most going/interested), popular artists (most followed), hot tags (most applied this week), active venues. `/charts` page with period selector.

### Notifications *(design doc: `docs/strategy/notification-filters.md`)*
- [ ] **Notification filter model and matching engine** — `notification_filters` with array criteria. Matching on show approval. Email delivery via Resend. "Notify me" buttons on entity pages.
- [ ] **Notification filter frontend** — filter management, create/edit with multi-select autocomplete, notification history.

### Venue Intelligence (needs tags from Phase 2b)
- [ ] **Venue genre profiles** (PSY-61) — per-venue tag distribution from booking patterns (min 10+ shows), scene-level genre chart.
- [ ] **Venue similarity** (PSY-62) — Jaccard index on shared artist sets, 3+ shared artists threshold.

### Analytics & Platform Health
- [ ] **Platform analytics dashboard** (PSY-48) — time-series growth charts, user engagement trends, data quality trends. Now includes community health: contributions per week, active contributors, collection growth, request fulfillment rate.

### Other
- [ ] Artist claim flow (Spotify OAuth)
- [ ] Featured content — "Featured Show" / "Featured Collection" / "Community Pick" badges. What.cd's staff picks pushed people outside their comfort zone — PH's equivalent: prominent featured placement that exposes users to genres/venues they wouldn't seek out.

**Phase 2c exit criteria:** Going/Interested and Follow systems live. Top charts page active. Notification filters matching on show approval. Venue genre profiles derived from booking patterns. Platform analytics tracking community health.

---

## BACKGROUND: Pipeline Maturation (Phase 1.6b, ongoing)

**Theme:** The pipeline issues deferred from Phase 1.6a get addressed incrementally, as background work between community feature sprints. Not a phase gate — a maintenance track.

- [ ] **AI billing enrichment** (PSY-30) — extend extraction for headliner/support/opener
- [ ] **Automated scheduling** (PSY-31) — scheduled runs with change detection, chromedp worker pool, strategy adaptation (`venue_extraction_runs` tracking, anomaly detection)
- [ ] **Consolidate discovery UI** (PSY-33) — retire standalone discovery app, add "Data Pipeline" admin tab
- [ ] **Post-import enrichment pipeline** (PSY-35) — automatic artist matching, MusicBrainz lookup, API cross-referencing
- [ ] iCal/RSS feed detection — auto-detect structured feeds during venue onboarding

**Exit criteria (ongoing):** Pipeline handles 20+ venues reliably. Automated scheduling running daily. At least 3 data source tiers working.

---

## OPPORTUNISTIC: Admin Dashboard Polish (Phase 1.7)

**Theme:** Admin-only UX improvements. Picked up as quick wins between feature sprints, not a blocking phase.

**Linear project:** "Phase 1.7: Admin Dashboard UX" (PSY-37 through PSY-44)

- [ ] PSY-37: Dashboard layout polish (fix clipping, dark mode contrast)
- [ ] PSY-38: Clickable stat cards
- [ ] PSY-39: Smart empty states
- [ ] PSY-40: Dashboard quick actions
- [ ] PSY-41: Recent activity feed
- [ ] PSY-42: Stat card trend indicators
- [ ] PSY-43: Admin tab grouping
- [ ] PSY-44: Admin Cmd+K commands

---

## Q4 2026: Community at Scale (Phase 3)

**Theme:** With contributor identity, collections, requests, revision history, tags, and engagement all live, Phase 3 focuses on what genuinely needs community scale: opening edit flows to non-admin users, building moderation infrastructure, and trust tier progression.

### Open Contribution Flows
- [ ] Extend pending-edit approval workflow to all entities
- [ ] User contribution flows (add labels, releases, tag entities, write descriptions)
- [ ] "Needs Attention" public-facing dashboard — extend admin PSY-45 to public contributor view with contribution prompts

### Moderation Infrastructure (Gazelle-inspired)
- [ ] **Unified moderation queue** — generalize all pending items into one filterable queue with claim/resolve workflow (Gazelle's `reportsv2` pattern)
- [ ] **Generalized content flagging** — structured reason categories per entity type
- [ ] **Auto-promotion scheduler** — activate the tier model from Phase 2a. Scheduled job (daily) evaluates all users against tier criteria: Contributor (5+ approved edits, 2+ weeks, verified email), Trusted Contributor (25+ approved, 95%+ approval rate, 2+ months), Local Ambassador (50+ approved, city-active, 6+ months). Auto-demotion if quality drops. PM notifications explaining changes. *(Tier model and display ships in Phase 2a; this activates the automation.)*
- [ ] **Contributor leaderboard** — top contributors by week/month/all-time with contribution types and accuracy rate. Uses percentile ranking infrastructure from Phase 2a.
- [ ] **Global site notices** — admin-set banners for community announcements
- [ ] **Staff/moderation inbox** — centralized message system for appeals, questions, moderator coordination

### Community-Powered Scene Knowledge
- [ ] **Show field notes** — brief attendee observations (not star ratings). "The opener stole the show." Qualitative data no other platform captures.
- [ ] **Setlist integration** — community-contributed or setlist.fm-sourced track-level show data
- [ ] **Promoter / Booker entity** — community knowledge about who books what
- [ ] **Musician entity** — individuals distinct from bands, with membership history. Scene family tree.

### Depth & Preservation
- [ ] **Release editions** — when community contribution flows are mature, allow edition-level detail: year, label, catalog number, country, format, mastering notes. What.cd's Oxygène page had 14+ editions. Start simple, let community demand drive it.
- [ ] **Knowledge graph export** — data exports in standard formats (JSON-LD, CSV). CC0 for factual data. An open API is insurance that the community's work survives any platform failure.

**Phase 3 exit criteria:** 100+ community contributions. 10+ active contributors with trust tier progression. Unified moderation queue processing all types. At least 3 auto-promoted Trusted Contributors. Contributor leaderboard driving engagement. Show field notes on 20%+ of attended shows. Data export available.

---

## Q1 2027: Geographic Expansion (Phase 4)

**Theme:** Prove the model works beyond Phoenix.

### Multi-City Infrastructure
- [ ] Second city launch (Tucson — low-risk test)
- [ ] Third city launch (non-Arizona — validate community-bootstrapped expansion)
- [ ] Cross-city navigation (touring artists connecting scenes, label presence, festival circuit patterns)
- [ ] Scene comparison and discovery
- [ ] Festival data as expansion bootstrapping
- [ ] **Automated venue discovery** — Google Maps API + OpenStreetMap
- [ ] **AI extraction at scale** — adding a city = discovering venue URLs + extraction schedule. Zero per-venue code.

### City-Scoped Admin Tools
- [ ] Per-city admin dashboard
- [ ] Pipeline health monitor
- [ ] City onboarding checklist

**Key validation:** Can a city launch without custom code? With AI extraction + festival data + venue discovery + community contributions, yes.

### Supporter / Monetization (Gazelle Donor Model)

Cosmetic rewards for supporters — **never functional advantages**. Gazelle proved this builds trust: the richest user and the poorest had the same influence on the knowledge graph. *(Analysis: `docs/learnings/gazelle-user-profiles.md`)*

- [ ] **Supporter tier model** — `supporter_rewards` table with tiered cosmetic unlocks
- [ ] **Tier 1:** "Supporter" badge on profile and contributions, 1 additional custom profile section
- [ ] **Tier 2:** Custom avatar border/frame, 2nd additional profile section
- [ ] **Tier 3:** Custom title under username, 3rd additional profile section
- [ ] **Tier 4+:** Custom badge icon, additional cosmetics TBD
- [ ] **Payment integration** — Stripe subscriptions, $10-40/month range (What.cd user signal: "$20-40/month, probably")

**Prerequisite:** Community must exist first. This is a Phase 4+ revenue stream, not a launch feature. People pay for something they feel ownership of.

---

## 2027+: Discovery Engine (Phase 5)

**Theme:** The full What.cd discovery experience, anchored in live music. Unlock temporal intelligence only our show data makes possible.

- [ ] "Explore" mode (browse genres across cities, discover scenes)
- [ ] Travel mode ("I'm visiting Cincinnati next week" → personalized scene guide)
- [ ] **Voter picks / collaborative filtering** — "People who saved this show also saved..."
- [ ] Personalized recommendations (saves + follows + attendance)
- [ ] "Fans of [artist] also go to..." (from attendance data)
- [ ] **Temporal scene graph** — time-slider on scene pages
- [ ] **Artist trajectory visualization** — career arc from house shows to headlining
- [ ] **Bill composition intelligence** — "Who opened for [headliner] before they broke?"
- [ ] **Festival circuit analysis** — festival-to-festival overlap patterns
- [ ] **Scene family tree visualization** — band member crossover across a city
- [ ] **Taste profile integration** — connect Spotify/Last.fm to user profiles, show listening data (privacy-controlled), enable taste compatibility between users, feed into collaborative filtering. Gazelle's Last.fm integration created organic social connections between users with similar taste.
- [ ] API / MCP server (knowledge graph as platform)
- [ ] Weekly personalized email digest
- [ ] Venue analytics dashboard (free + paid tiers)

### Future Discovery Pipeline
- [ ] Computer Use for multi-page calendars (Anthropic Computer Use API as tier 4)
- [ ] Social media extraction (venue Instagram/Facebook → events)
- [ ] Email newsletter parsing (forwarded venue digests → events)
- [ ] Web search synthesis ("upcoming shows at [venue]" → structured events)
- [ ] Bandsintown API integration, Setlist.fm integration

### Future Data Enrichment
- [ ] Radio Station & Radio Show entities *(design doc: `docs/strategy/radio-entities.md`)*
- [ ] Curated radio playlist parsing (WFMU, NTS, KEXP as discovery signals)
- [ ] Discogs integration
- [ ] Broader Phoenix data seeding

---

## Open Decisions

- **Monetization:** Leaning toward Gazelle's cosmetic donor model — supporter tiers with cosmetic-only rewards (badges, profile customization, custom titles), never functional advantages. User signal: a power user donated ~$100/yr and said he'd pay "$20-40/month." Gazelle proved cosmetic rewards sustain platforms when users feel ownership. Prerequisite is community, not features. Supporter tier scaffolding designed (see Phase 4), implementation deferred until community exists.
- **Mobile strategy:** Continue native iOS or evaluate PWA
- **API openness:** Fully open (MusicBrainz model) vs. tiered access
- **Geographic scope:** US-first or global-ready data model. What.cd had "Bach cello suites, Chinese indie rock, Nigerian hip-hop, Thai psych-funk." PH's graph will span geographies through touring artists, festivals, and labels. Architect for global scope even while launching city-by-city.
- **Graph DB:** Stay PostgreSQL or evaluate Neo4j when traversal queries become core
- **Release depth:** Full What.cd-level detail (every pressing/format) or curated highlights? Start simple, let community depth emerge.
- **Data survivability:** Plan for exports and data portability from the start — the community's knowledge must survive any platform failure.

## Risks

- **Single-person team:** Bus factor of 1 across all tracks
- **Cold start per city:** Each new city needs initial data + at least one community champion
- **Community quality:** Maintaining data accuracy without heavy moderation burden
- **Scraper fragility:** Venue sites change without warning (mitigated by AI extraction — reads any page like a human)
- **Incentive design:** Community-driven platforms need sustainable motivation structures. What.cd's entrance exam and ratio requirements were effective but controversial — we use open registration with reputation-based quality assurance instead.
- **External link rot:** Bandcamp/Spotify/YouTube links can go stale; need periodic verification or community reporting
- **Community cold start:** Contributor identity and collections ship but nobody contributes. Mitigated by: (1) admin as first visible contributor, (2) seed collections as examples, (3) auto-generated requests as specific contribution opportunities, (4) What.cd diaspora (RED, Orpheus, RYM users) actively seeking this

## Design Docs

All Phase 2 design docs complete:
- Scene pages & venue intelligence → `docs/strategy/scene-pages.md`
- Notification filters → `docs/strategy/notification-filters.md`
- Similar artist visualization → `docs/strategy/similar-artists.md`
- Radio Station & Radio Show entities → `docs/strategy/radio-entities.md`
- Festival intelligence → `docs/strategy/festival-intelligence.md`

Learnings & reference:
- Gazelle/What.cd implementation patterns → `docs/learnings/gazelle-patterns.md`
- What.cd user psychology & product-market fit → `docs/learnings/whatcd-user-insights.md`
