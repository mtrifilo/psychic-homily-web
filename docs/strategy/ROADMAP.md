# Roadmap

> Quarterly priorities across all tracks. For the product vision and entity model, see `docs/vision.md`. For track-specific details, see individual track files.

## Guiding Principle

Live shows are the gateway into the knowledge graph. Every phase builds outward from the live experience: shows lead to artist discovery, which leads to releases and labels, which leads to more artists and more shows. The knowledge graph grows from the live foundation — never away from it.

**Build the thing that differentiates us first.** Social features (Going/Interested) are commodity — every event platform has them. The knowledge graph is what makes this the What.cd successor. Prioritize the graph.

**Bake in What.cd DNA from the start.** Features like artist-release roles, tag voting, and similar artist voting are low incremental cost when built alongside the entities they enrich. Don't defer them to a "polish" phase — build them right the first time.

## Current Priority Order

| Priority | Focus | What | Status |
|----------|-------|------|--------|
| 1 | Web | Email preferences UI, then knowledge graph vertical slice | In progress |
| 2 | iOS | Polish, test, ship to App Store | Blocked (Apple Developer enrollment) |
| 3 | Discovery | Provider reliability, automated scheduling | Maintenance mode |

---

## Q1-Q2 2026: Foundation + Knowledge Graph Vertical Slice (Phase 1)

**Theme:** Close out remaining utility work, then immediately start proving the What.cd vision with a vertical slice of the knowledge graph.

### Wrap Up (Phase 1 utility)
- [x] Artists list page with search + multi-city filters
- [x] Venues page search + multi-city filters
- [x] Calendar sync (ICS feed for saved shows)
- [x] Show reminders (email, 24h before, with one-click unsubscribe)
- [ ] Email preferences UI (in progress)

### Frontend Redesign (Phase 1, before and alongside entity work)

Structural UI redesign to evolve from "show tracker" to "music knowledge graph." Current flat top nav and narrow `max-w-4xl` layout can't accommodate 15+ entity types. Full spec in `docs/strategy/ui-redesign.md`.

**Build order (optimized for agent execution):**

1. **Sidebar navigation + wider layout** -- collapsible sidebar replacing top nav, content area widened to `max-w-6xl`/`max-w-7xl`, 2-column grid for detail pages. Do BEFORE entity pages ship so new pages use the new layout from the start. (Frontend only, no backend deps)
2. **Cmd+K command palette** -- global search dialog with route navigation and cross-entity search. Uses `cmdk` library. (Frontend only, can parallel with entity backend work)
3. **Entity detail page template** -- reusable `EntityDetailLayout` with header zone, tabs, sidebar panel. Refactor Venue/Artist detail first, then new entities use it from day one. (Best done WITH first new entity page)
4. **Show card redesign** -- bill hierarchy, date badge, inline save, tag pills. (Independent, anytime)
5. **Artist card redesign** -- tag pills, label affiliation, card borders. (Independent, anytime)
6. **Visual polish** -- bolder typography, card borders, density toggles. (Sweep after structural changes)

**Design references:** Vercel (sidebar, command palette), Discogs (entity navigation), Linear (keyboard-first, density), Gazelle (tag voting, typed relationships, community curation patterns modernized).

### Data Layer Foundation + Knowledge Graph Vertical Slice (Phase 1.5)

Lay the schema foundation for the full knowledge graph, then prove the minimum viable version. A user should navigate from a show to an artist to their releases to their label to label mates, and from a festival to 40 artists on the bill. This is the moment the app stops being "another show tracker."

**Build order:**

1. **Data layer foundation**
   - Generic `user_bookmarks` table replacing `user_saved_shows` and `user_favorite_venues` — supports all entity types and action types (save, follow, bookmark, going, interested). No users yet, clean migration.
   - TIMESTAMPTZ standardization (initial schema uses TIMESTAMP without timezone)
   - Refactor saved show and favorite venue services/handlers/hooks to use generic bookmark infrastructure

2. **Festival entity** — model, migrations, CRUD API, admin UI, pages
   - Distinct from Show: `series_slug` + `edition_year` (UNIQUE constraint) for recurring festivals
   - `festival_artists` junction: `billing_tier` (headliner/sub_headliner/mid_card/undercard/local/dj/host), `position`, `day_date`, `stage`, `set_time`, `venue_id`
   - `festival_venues` junction for multi-venue takeover festivals
   - `location_name` for non-venue locations (parks, fairgrounds)
   - URL structure: `/festivals/:series_slug/:year`
   - Festival listing page (`/festivals`) and series overview (`/festivals/:series_slug`)

3. **Releases entity** — model, migrations, CRUD API, admin UI
   - Type: LP, EP, single, compilation, live
   - External links: Bandcamp, Spotify, Discogs, YouTube, Apple Music (the "Listen / Buy" section that replaces What.cd's download button)
   - Cover art, year
   - Artist <--> Release relationship with **role types** (main, featured, producer, remixer, composer, DJ) — bake in the What.cd credit model from day one

4. **Labels entity** — model, migrations, CRUD API, admin UI
   - Name, city, founded year, status, socials, description
   - Artist <--> Label relationship (junction table)
   - Release <--> Label relationship

5. **Artist pages enriched** — discography section, label affiliations, festival appearances
   - Artist detail page shows releases with external links, grouped by role (albums, guest appearances, production credits — like What.cd)
   - Artist detail page shows label affiliations and "Also on this label" (the discovery moment)
   - Artist detail page shows festival appearances with billing tier — visible career trajectory

6. **Release pages** (`/releases/:slug`) with "Listen / Buy" external links
7. **Label pages** (`/labels`, `/labels/:slug`) with roster and catalog
8. **Festival pages** (`/festivals/:series_slug/:year`) with tiered lineup display, day tabs, "artists you follow" highlights

9. **Data seeding** — populate knowledge graph
   - Admin festival entry — enter major US festivals (M3F, Levitation, Psycho Las Vegas, Desert Daze, etc.). One festival = ~10 min for 40+ graph connections. Geographic expansion without scrapers.
   - MusicBrainz integration (artist-label relationships, release metadata, artist roles)
   - Bandcamp enrichment (release links, label rosters)

**Exit criteria:** Sidebar nav + Cmd+K search live. Entity detail template in use on all detail pages. 50+ Phoenix artist discographies populated. 200+ releases with external links. 20+ labels cataloged. 10+ festivals entered with lineups. Generic bookmarks system live. A user can navigate: show --> artist --> releases (with role credits) --> label --> label mates --> their shows. Festival --> 40 artists --> their releases and shows. Show/artist cards redesigned with bill hierarchy, tag pills, and discovery cues.

### iOS (unchanged, blocked)
- [ ] Apple Developer enrollment ($99/year)
- [ ] Re-enable capabilities (Sign in with Apple, App Groups, Keychain Sharing)
- [ ] Polish & error states
- [ ] TestFlight --> App Store submission

### Discovery
- [ ] Audit all 5 providers against live sites
- [ ] Fix broken scrapers
- [ ] Automated daily/weekly schedule
- [ ] Error alerting (Discord/Sentry)
- [ ] **AI billing enrichment** — add screenshot step to Playwright scrapers, send event detail pages through Claude (extraction service pattern) to determine accurate headliner/support/opener billing. Replaces "first listed = headliner" heuristic with semantic understanding. Low cost (Haiku on ~100 events/month).
- [ ] Expand `set_type` vocabulary: `headliner` → `direct_support` → `opener` (three-tier minimum), plus `dj`, `special_guest`
- [ ] **Festival data entry** — admin entry of major US festival lineups. AI-assisted: screenshot festival posters, extract structured lineup with billing tiers from font size/position. Geographic expansion without scrapers.

---

## Q3 2026: Knowledge Graph Expansion (Phase 2)

**Theme:** Complete the knowledge graph with tags, relationships, scene pages, and the features that make the graph navigable and alive. This is where passive browsing becomes active exploration.

### Tags & Voting
- [ ] Genre/tag system (hierarchical taxonomy + freeform tags)
- [ ] Entity <--> Tag relationships (artists, venues, labels, releases)
- [ ] **Tag voting** — users upvote/downvote tags on each entity (What.cd core feature)
- [ ] Genre/tag browsing and filtering UI

### Artist Relationships
- [ ] Artist <--> Artist relationships (similar, side projects, members of)
- [ ] **Similar artist voting** — community votes on similarity (up/down), scores determine ranking
- [ ] **Similar artist visualization** — interactive relationship map or cloud (iconic What.cd feature)
- [ ] "Toured with" / "shared bills with" auto-derived from show data

### Scene Pages & Venue Intelligence
- [ ] City landing page (`/scenes/:city`) aggregating local venues, artists, labels, tags, shows
- [ ] **Scene health metrics** — shows/month, genre diversity index, new artist appearances, venue activity trends
- [ ] Phoenix as first fully-populated scene
- [ ] Scene page SEO and social sharing
- [ ] **Venue personality profiles** — auto-derived genre profiles from booking patterns, "venues like this one" based on booking overlap

### Show & Festival Data Enrichment
- [ ] **Bill position surfacing & accuracy** — `position` and `set_type` fields already exist on `show_artists` (migration 000001) and the discovery service already assigns headliner/opener. Remaining work: expose `set_type` in API responses (beyond `is_headliner` bool), surface in frontend UI, admin correction UI, and improve data quality via AI billing enrichment (see Discovery section)
- [ ] **Festival intelligence** — festival-to-festival lineup overlap analysis, "artists you follow at this festival" personalized view, breakout artist tracking (undercard → headliner arc within and across festivals)
- [ ] **Show-to-recording links** — connect live recordings (Bandcamp, YouTube) to the show entity where they were captured

### Engagement & Discovery
- [ ] **Notification filters** — "notify me of [genre] shows at [venue]" or "notify me when [label] artists play in Phoenix" (power user feature)
- [ ] **Top charts** — trending shows, most-followed artists, popular tags, top contributors
- [ ] "Going" / "Interested" buttons on shows and festivals (built on `user_bookmarks.action` — schema from Phase 1.5)
- [ ] Attendance counts on show cards and festival pages
- [ ] User follow system for artists, venues, labels (built on `user_bookmarks.action = 'follow'`)
- [ ] Artist claim flow (Spotify OAuth)

### Data Enrichment & Radio
- [ ] Discogs integration (catalog data, genre taxonomy)
- [ ] Broader Phoenix data seeding (labels, genres, releases beyond initial slice)
- [ ] **Radio Station & Radio Show entities** — model, migrations, CRUD API, pages (`/radio`, `/radio/:slug`). Stations have live stream embeds, donation page links, and embeddable pledge widgets. Radio Shows link to stations and track playlists.
- [ ] **Curated radio playlist parsing** — WFMU, NTS, KEXP playlists as discovery signals for releases, labels, and artist-artist affinity (see vision doc for full strategy)

### Discovery Pipeline
- [ ] **Hybrid scraper + AI pipeline** — traditional scrapers for structured data (dates, prices, links), AI for semantic interpretation (billing order, artist disambiguation, festival detection)
- [ ] Venue-level `artist_order_hint` config (`headliner_first` vs `chronological`) for scraper accuracy
- [ ] Admin UI: reorder artists and change set types on show edit page

### iOS
- [ ] Post-launch bug fixes
- [ ] Push notifications

**Exit criteria:** Genre tags on 80% of artists with community voting active. Scene page live with health metrics. Similar artist graph navigable with voting. Venue personality profiles derived. Notification filters available. Going/Interested shipped. Festival intelligence queries live (lineup overlap, breakout tracking).

---

## Q4 2026: Community Engine (Phase 3)

**Theme:** Let the community build the knowledge graph — the engine that made What.cd irreplaceable. This phase adds the tools that channel community energy into catalog completeness.

### Contribution Infrastructure
- [ ] Extend pending-edit approval workflow to all entities
- [ ] User contribution flows (add labels, releases, tag entities, write descriptions)
- [ ] **Revision history** — full edit history on all community-editable content (artist bios, release descriptions, label descriptions) with diff view and revert capability

### Community-Driven Completeness
- [ ] **"Needs Attention" dashboards** — surfaces content needing improvement: artists without bios, releases missing external links, labels without descriptions, venues without complete info. Gamifies contribution by showing exactly what needs work. (Inspired by Gazelle's "Better" section, which surfaced releases needing transcoding, tags, artwork, etc.)
- [ ] **Request system with voting** — users request missing artists, labels, releases, corrections. Community votes to prioritize. Fulfilling requests earns reputation. (What.cd's bounty system drove obsessive completeness)

### Community-Powered Scene Knowledge
- [ ] **Show field notes** — brief attendee observations capturing the live experience (not star ratings). "The opener stole the show", "They debuted two new songs." Qualitative data no other platform captures.
- [ ] **Setlist integration** — community-contributed or setlist.fm-sourced track-level show data. Bridge recorded catalog to live performance.
- [ ] **Promoter / Booker entity** — community-contributed knowledge about who books what. The hidden connectors that shape scenes.
- [ ] **Musician entity** — individuals distinct from bands, with membership history and date ranges. Enables band member crossover mapping — the "scene family tree."

### Curation & Trust
- [ ] **Collections with categories** — Genre Introduction ("Intro to free jazz"), Label Roster, Staff Picks, Scene Guide, Charts, Personal. Categorization makes collections discoverable and useful, not just freeform lists.
- [ ] **Reputation system** — contribution quality --> trust level --> auto-approve privileges
- [ ] Local ambassador program
- [ ] Community moderation tools

**Exit criteria:** 100+ community contributions. 10+ active contributors. 5+ categorized collections. 20+ fulfilled requests. "Needs Attention" dashboard driving measurable data quality improvement. Show field notes on 20%+ of attended shows.

---

## Q1 2027: Geographic Expansion (Phase 4)

**Theme:** Prove the model works beyond Phoenix.

- [ ] Second city launch (Tucson — low-risk test)
- [ ] Third city launch (non-Arizona — validate community-bootstrapped expansion)
- [ ] Cross-city navigation (touring artists connecting scenes, label presence, festival circuit patterns)
- [ ] Scene comparison and discovery
- [ ] Cross-city "shared bills" relationships from touring data
- [ ] Festival data as expansion bootstrapping — festivals in new cities provide initial artist graph connections before venue scrapers exist
- [ ] **AI scraper agents** — point an AI agent at any venue's calendar URL, it navigates and extracts all events using vision. No venue-specific scraper code needed. New city onboarding without writing custom providers.

**Key validation:** Can a city launch without custom scrapers? With festival data + AI scraper agents + community contributions + external APIs, the answer should be yes. Festivals provide the initial graph; community and AI fill in venue-level detail.

---

## 2027+: Discovery Engine (Phase 5)

**Theme:** The full What.cd discovery experience, anchored in live music. Unlock the temporal intelligence that only our show data makes possible.

- [ ] "Explore" mode (browse genres across cities, discover scenes)
- [ ] Travel mode ("I'm visiting Cincinnati next week" --> personalized scene guide)
- [ ] **Voter picks / collaborative filtering** — "People who saved this show also saved...", "People who follow this artist also follow..."
- [ ] Personalized recommendations (listening history + saves + follows + attendance)
- [ ] Similar artist discovery (shared labels, shared bills, genre overlap, touring patterns)
- [ ] "Fans of [artist] also go to..." (from attendance data)
- [ ] Related/similar scene suggestions
- [ ] **Temporal scene graph** — time-slider on scene pages to browse how a city's music ecosystem evolves over months and years. Watch genres rise and fall, venues open and close, artists emerge. The knowledge graph as a living historical record.
- [ ] **Artist trajectory visualization** — career arc from house shows to headlining, rendered as a visual timeline of venue sizes, show roles, and festival billing tiers over time
- [ ] **Bill composition intelligence** — "Who opened for [headliner] before they broke?" / "Openers who later became headliners" / cross-genre billing pattern analysis
- [ ] **Festival circuit analysis** — festival-to-festival artist overlap patterns, genre clustering by festival, "festivals like this one" recommendations
- [ ] **Scene family tree visualization** — interactive map of band member crossover across an entire city's music scene
- [ ] API / MCP server (knowledge graph as platform)
- [ ] Weekly personalized email digest
- [ ] Venue analytics dashboard (free + paid tiers)

---

## Open Decisions

- **Monetization:** Venue subscriptions vs. premium features vs. donation-supported (What.cd model)
- **Mobile strategy:** Continue native iOS or evaluate PWA
- **API openness:** Fully open (MusicBrainz model) vs. tiered access
- **Geographic scope:** US-first or global-ready data model from the start
- **Graph DB:** Stay PostgreSQL or evaluate Neo4j when traversal queries become core
- **Release depth:** Full What.cd-level detail (every pressing/format) or curated highlights? Start simple, let community depth emerge.

## Risks

- **Single-person team:** Bus factor of 1 across all tracks
- **Cold start per city:** Each new city needs initial data + at least one community champion
- **Community quality:** Maintaining data accuracy without heavy moderation burden
- **Scraper fragility:** Venue sites change without warning
- **Incentive design:** Community-driven platforms need sustainable motivation structures (What.cd's invite system and ratio requirements were effective but controversial — we need alternatives that motivate without gatekeeping)
- **External link rot:** Bandcamp/Spotify/YouTube links can go stale; need periodic verification or community reporting
