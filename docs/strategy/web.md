# Web Track

> Next.js frontend + Go backend. The primary platform for the Music Scene Index — the spiritual successor to What.cd, anchored in live music.

## Current Status

v1 feature-complete, pre-launch hardening. 69 E2E tests, 68.5% backend coverage, 836+ frontend unit tests. PostHog + Sentry live. ICS calendar feed and show reminders shipped.

Core entities: Artists, Venues, Shows with search, multi-city filters, saved shows, show reminders (email, 24h before), admin approval workflows, audit logging.

## Next Priorities

1. ~~**Show reminders** (email, 24h before)~~ -- done
2. **Email preferences UI** -- in progress, close it out
3. **Frontend redesign: sidebar nav + wider layout** -- structural foundation for knowledge graph UI. See `docs/strategy/ui-redesign.md`
4. **Frontend redesign: Cmd+K search + entity detail template** -- can parallel with entity backend work
5. **Data layer foundation** -- generic `user_bookmarks` table (replaces per-entity saved/favorite tables), TIMESTAMPTZ standardization
6. **Festival entity** -- distinct from Show, with series_slug, billing tiers, multi-venue support
7. **Knowledge graph vertical slice** -- Releases (with artist roles) + Labels + enriched artist pages + festival appearances (Phase 1.5)
8. **Show/artist card redesign + visual polish** -- richer cards with bill hierarchy, tags, discovery cues
9. **Knowledge graph expansion** -- Tags with voting, similar artists with voting/visualization, scene pages, notification filters (Phase 2)

## Roadmap

### Now: Frontend Redesign + Data Layer Foundation + Knowledge Graph (Phase 1 / 1.5)

- [x] Calendar sync (ICS feed)
- [x] Artists/venues pages with search + multi-city filters
- [x] Show reminders (email, 24h before, with one-click unsubscribe)
- [ ] Email preferences UI (in progress)
- [ ] **Frontend redesign: sidebar nav + wider layout** -- collapsible sidebar replacing top nav bar, content area widened from `max-w-4xl` to `max-w-6xl`/`max-w-7xl`, 2-column grid for detail pages. Do BEFORE entity pages so new entities use the new layout from the start. See `docs/strategy/ui-redesign.md` Task 1.
- [ ] **Frontend redesign: Cmd+K command palette** -- global search dialog (`cmdk` library), route navigation, cross-entity search. See `docs/strategy/ui-redesign.md` Task 2.
- [ ] **Frontend redesign: entity detail template** -- reusable `EntityDetailLayout` with header zone, tabs, sidebar panel. Refactor Venue/Artist detail first, new entity pages (Festival, Release, Label) use it from the start. See `docs/strategy/ui-redesign.md` Task 3.
- [ ] **Data layer foundation** — generic `user_bookmarks` table replacing `user_saved_shows` + `user_favorite_venues`. Supports all entity types and action types (save, follow, bookmark, going, interested). TIMESTAMPTZ standardization. Refactor services/handlers/hooks.
- [ ] **Festival entity** — model, migrations, CRUD API, admin UI. `series_slug` + `edition_year` for recurring festivals. `festival_artists` with `billing_tier`/`day_date`/`stage`/`set_time`/`venue_id`. `festival_venues` for multi-venue takeover festivals. `location_name` for non-venue locations.
- [ ] **Festival pages** — `/festivals` listing, `/festivals/:series_slug` series overview, `/festivals/:series_slug/:year` detail with tiered lineup display, day tabs, "artists you follow" highlights
- [ ] **Releases entity** — model, migrations, CRUD API, admin UI, `/releases/:slug` pages with "Listen / Buy" external links (Bandcamp, Spotify, Discogs, YouTube, Apple Music). Artist-release roles: main, featured, producer, remixer, composer, DJ.
- [ ] **Labels entity** — model, migrations, CRUD API, admin UI, `/labels` and `/labels/:slug` pages
- [ ] **Artist pages enriched** — discography grouped by role (albums, guest appearances, production credits), label affiliations, "also on this label", festival appearances with billing tier
- [ ] **Data seeding** -- admin festival entry (major US festivals), MusicBrainz + Bandcamp enrichment for Phoenix artists
- [ ] **Show card redesign** -- bill hierarchy (headliner bold, support with "w/"), date badge, inline save, tag pills (placeholder-ready). See `docs/strategy/ui-redesign.md` Task 4.
- [ ] **Artist card redesign** -- tag pills, label affiliation, consistent card borders. See `docs/strategy/ui-redesign.md` Task 5.
- [ ] **Visual language polish** -- bolder typography, card border treatments, density toggles on list pages. See `docs/strategy/ui-redesign.md` Task 6.

### Next: Knowledge Graph Expansion (Phase 2, Q3 2026)

- [ ] **Genre/tag system** — hierarchical taxonomy + freeform tags, **tag voting** (up/down per entity), tag browsing and filtering UI
- [ ] **Artist <--> Artist relationships** — similar, side projects, members of
- [ ] **Similar artist voting** — community votes on similarity, scores determine ranking
- [ ] **Similar artist visualization** — interactive relationship map or cloud
- [ ] **"Toured with" / "shared bills with"** — auto-derived from show data
- [ ] **Scene pages** — `/scenes/:city` landing page with scene health metrics (shows/month, genre diversity, new artists, venue activity)
- [ ] **Venue personality profiles** — auto-derived genre profiles from booking patterns, "venues like this one"
- [ ] **Bill position surfacing & accuracy** — schema exists (`position` + `set_type` on `show_artists`, discovery service populates them). Remaining: expose `set_type` in API (beyond `is_headliner` bool), frontend display, admin correction UI
- [ ] **Festival intelligence** — festival-to-festival lineup overlap, "artists you follow at this festival", breakout artist tracking
- [ ] **Show-to-recording links** — connect live recordings to show entities
- [ ] **Notification filters** — "notify me of [genre] shows at [venue]"
- [ ] **Top charts** — trending shows, most-followed artists, popular tags, top contributors
- [ ] "Going" / "Interested" buttons on shows and festivals (built on `user_bookmarks.action`)
- [ ] Attendance counts on show cards and festival pages
- [ ] Artist claim flow (Spotify OAuth)
- [ ] User follow system for artists, venues, labels (built on `user_bookmarks.action = 'follow'`)
- [ ] Discogs integration (catalog data, genre taxonomy)
- [ ] **Radio Station & Radio Show entities** — `/radio`, `/radio/:slug` pages with live stream embeds, donation links, pledge widgets
- [ ] **Curated radio playlist parsing** — WFMU, NTS, KEXP as discovery/enrichment signals ("as heard on" badges, artist↔radio show links)

### Later: Community & Scale (Phases 3-5)

- [ ] Community contribution flows (add/edit all entities with pending review)
- [ ] **Revision history** — edit history on all community-editable content with revert
- [ ] **"Needs Attention" dashboards** — artists without bios, releases missing links, labels without descriptions (Gazelle "Better" section successor)
- [ ] **Request system with voting** — community fills gaps in the catalog, votes to prioritize
- [ ] **Collections with categories** — Genre Introduction, Label Roster, Staff Picks, Scene Guide, Charts, Personal
- [ ] **Show field notes** — brief attendee observations capturing the live experience
- [ ] **Setlist integration** — community-contributed or setlist.fm-sourced track-level show data
- [ ] **Promoter / Booker entity** — who books what, the hidden connectors that shape scenes
- [ ] **Musician entity** — individual people with band membership history, enabling scene family trees
- [ ] Reputation system (contribution quality --> trust level --> auto-approve)
- [ ] Multi-city scene browsing and comparison
- [ ] Travel mode, personalized recommendations
- [ ] **Voter picks / collaborative filtering** — "People who saved this also saved..."
- [ ] Similar artist discovery (shared labels, shared bills, genre overlap, touring patterns)
- [ ] **Temporal scene graph** — time-slider to browse how a city's scene evolves over time
- [ ] **Artist trajectory visualization** — career arc from house shows to headlining, with festival billing tier progression
- [ ] **Bill composition intelligence** — opener-to-headliner patterns, cross-genre billing analysis
- [ ] **Festival circuit analysis** — festival-to-festival artist overlap, genre clustering, "festivals like this one"
- [ ] **Scene family tree visualization** — interactive band member crossover map
- [ ] API / MCP server

## Key Files

| Area | Files |
|------|-------|
| Frontend entry | `frontend/app/` (Next.js App Router) |
| API client | `frontend/lib/api.ts`, `frontend/lib/hooks/` |
| Backend routes | `backend/internal/api/routes/routes.go` |
| Service container | `backend/internal/services/container.go` |
| Models | `backend/internal/models/` |
| Migrations | `backend/db/migrations/` (latest: 000034) |
| E2E tests | `frontend/e2e/`, `frontend/playwright.config.ts` |
