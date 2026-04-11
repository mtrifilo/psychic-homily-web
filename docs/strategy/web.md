# Web Track

> **STATUS: ACTIVE.** Primary platform. All phases through 3 complete. ~2500+ frontend tests (190+ files), 69 E2E, 80%+ backend handler coverage.
>
> Next.js frontend + Go backend. The primary platform for the Music Scene Index.

## Current Status

All phases through Phase 3 complete. Comments system (Waves 1-5), Collections UX overhaul, Radio provider fixes all shipped (April 2026). ~2500+ frontend unit tests, 69 E2E, 80%+ backend handler coverage. PostHog + Sentry live.

Core entities: Artists, Venues, Shows, **Releases** (with artist roles + external links), **Labels** (with roster + catalog), **Festivals** (with tiered lineups, multi-venue). Enriched artist pages with discography, label affiliations, festival appearances. Generic bookmarks system. **Contributor profiles** (PSY-63): public profiles with contribution stats/history, 3-level granular privacy, user tiers, custom profile sections, 58 tests. Data seeding CLIs (MusicBrainz, Bandcamp, festival entry). Frontend redesign complete (sidebar nav, Cmd+K, entity detail template, card redesigns, visual polish).

## Next Priorities (Restructured March 2026)

> Community curation is the moat, not the data pipeline. See `docs/learnings/whatcd-user-insights.md` for the What.cd user analysis that drove this restructuring.

1. ~~**Show reminders** (email, 24h before)~~ — done
2. **Email preferences UI** — in progress, close it out
3. ~~**Frontend redesign**~~ — done
4. ~~**Data layer foundation**~~ — done
5. ~~**Knowledge graph vertical slice**~~ — done
6. **Phase 1.6a + Phase 2a (parallel, NOW):**
   - Pipeline foundation (PSY-32 AI extraction, PSY-34 provenance, PSY-36 venue config)
   - Community foundations: contributor identity (PSY-63/64), collections (PSY-65–68), requests (PSY-70–72), revision history (PSY-73/74)
7. **Phase 2b: Knowledge graph connective tissue** — tags (PSY-49–51), relationships (PSY-52/53), scenes (PSY-59/60), data quality (PSY-45/47), bill position (PSY-54)
8. **Phase 2c: Engagement & social** — going/interested (PSY-55), follow (PSY-56), charts (PSY-57), notifications, venue profiles (PSY-61/62)

## Roadmap

### Now: Frontend Redesign + Data Layer Foundation + Knowledge Graph (Phase 1 / 1.5)

- [x] Calendar sync (ICS feed)
- [x] Artists/venues pages with search + multi-city filters
- [x] Show reminders (email, 24h before, with one-click unsubscribe)
- [ ] Email preferences UI (in progress)
- [x] **Frontend redesign: sidebar nav + wider layout** (PSY-16)
- [x] **Frontend redesign: Cmd+K command palette** (PSY-17)
- [x] **Frontend redesign: entity detail template** (PSY-18)
- [x] **Data layer foundation** — generic `user_bookmarks` table (PSY-22), TIMESTAMPTZ standardization (PSY-23)
- [x] **Festival entity** — model (PSY-24), service/handlers 82 tests (PSY-25), admin UI (PSY-26), frontend pages (PSY-27), data entry CLI (PSY-28)
- [x] **Festival pages** — `/festivals` listing, `/festivals/:slug` detail with tiered lineup display and day grouping
- [x] **Releases entity** — model (PSY-5), service 57 tests (PSY-6), admin UI (PSY-7), frontend pages (PSY-8)
- [x] **Labels entity** — model (PSY-9), service (PSY-10), admin UI (PSY-11), frontend pages (PSY-12)
- [x] **Artist pages enriched** — discography, label affiliations, "also on this label", festival appearances (PSY-13)
- [x] **Data seeding** — MusicBrainz CLI (PSY-14), Bandcamp CLI (PSY-15), festival entry CLI (PSY-28)
- [x] **Show card redesign** (PSY-19)
- [x] **Artist card redesign** (PSY-20)
- [x] **Visual language polish** (PSY-21)

### Now: Pipeline Foundation + Community Foundations (Phase 1.6a + 2a, parallel)

**Pipeline (Phase 1.6a):**
- [ ] **AI extraction pipeline** (PSY-32) — tiered rendering, change detection, 5+ venues
- [ ] **Data provenance tracking** (PSY-34) — source, confidence, last_verified on core tables
- [ ] **Venue source config** (PSY-36) — per-venue config persisted in DB

**Community (Phase 2a):**
- [x] **Contributor profile backend** (PSY-63) — 58 tests, 11 API endpoints, 3-level privacy, user tiers, custom sections
- [ ] **Contributor profile frontend** (PSY-64) — public profile page, "Added by" attribution, "Your Impact" section
- [ ] **Collections** (PSY-65–68) — the record store at scale. No prescribed categories. Visual grids. Collaboration by default. Subscriptions.
- [ ] **Request system** (PSY-70–72) — requests with voting, fulfillment workflow, auto-generated from data quality queries
- [ ] **Revision history** (PSY-73/74) — JSONB diffs on entity updates, "View History", admin rollback

### Next: Knowledge Graph Connective Tissue (Phase 2b)

- [ ] **Genre/tag system** (PSY-49–51, PSY-46) — hierarchical taxonomy, freeform tags, tag voting, alias resolution, auto-pruning
- [ ] **Artist relationships** (PSY-52/53) — similar, side projects, members of. "Shared bills" auto-derived from show data.
- [ ] **Similar artist visualization** — interactive relationship map *(design doc: `docs/strategy/similar-artists.md`)*
- [ ] **Scene pages** (PSY-59/60) — `/scenes/:slug` with Scene Pulse metrics *(design doc: `docs/strategy/scene-pages.md`)*
- [ ] **"Needs Work" dashboard** (PSY-45) — incomplete entities, feeds auto-requests
- [ ] **Artist merge/split** (PSY-47) — critical before geographic expansion
- [ ] **Bill position surfacing** (PSY-54)
- [ ] **Festival intelligence** *(design doc: `docs/strategy/festival-intelligence.md`)*
- [ ] **Decade/year entity rankings** — Wilson score per time scope

### Then: Engagement & Social (Phase 2c)

- [ ] **Going/Interested** (PSY-55) + **Follow system** (PSY-56) + **Top charts** (PSY-57)
- [ ] **Notification filters** *(design doc: `docs/strategy/notification-filters.md`)*
- [ ] **Venue genre profiles** (PSY-61) + **Venue similarity** (PSY-62)
- [ ] **Platform analytics** (PSY-48) — includes community health metrics
- [ ] **Featured content** — "Featured Show" / "Featured Collection" badges
- [ ] Artist claim flow (Spotify OAuth)

### Later: Community at Scale + Discovery Engine (Phases 3-5)

- [ ] Open edit flows, trust tiers, unified moderation queue, contributor leaderboard
- [ ] Show field notes, setlist integration, Promoter/Musician entities
- [ ] Release editions (community-driven depth), knowledge graph export
- [ ] Radio Station & Radio Show entities *(design doc: `docs/strategy/radio-entities.md`)*
- [ ] Multi-city expansion, travel mode, personalized recommendations
- [ ] Temporal scene graph, artist trajectory, bill composition intelligence
- [ ] Collaborative filtering, scene family tree visualization
- [ ] API / MCP server

## Key Files

| Area | Files |
|------|-------|
| Frontend entry | `frontend/app/` (Next.js App Router) |
| API client | `frontend/lib/api.ts`, `frontend/lib/hooks/` |
| Backend routes | `backend/internal/api/routes/routes.go` |
| Service container | `backend/internal/services/container.go` |
| Models | `backend/internal/models/` |
| Migrations | `backend/db/migrations/` (latest: 000041) |
| E2E tests | `frontend/e2e/`, `frontend/playwright.config.ts` |
