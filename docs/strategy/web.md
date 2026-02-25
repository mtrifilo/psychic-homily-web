# Web Track

> Frontend + backend web application development. Covers the Next.js frontend, Go backend, E2E tests, and infrastructure.

## Current Status

v1 feature-complete, in pre-launch hardening phase. 69 E2E tests (3 parallel workers), 68.5% backend coverage, 836+ frontend unit tests. CI fully wired with required checks and Codecov. PostHog + Sentry observability live. All P0/P1 security and legal launch items complete. Favorite cities multi-select shipped. Artists/venues page overhaul shipped (show counts, search, multi-city filters).

## Next Priorities

1. **Calendar sync (ICS feed)** — Phase 1 feature, highest growth impact at low effort
2. **One-click "Add to Google Calendar"** — pairs with ICS feed for daily-habit utility
3. **Show reminders (email, 24h before)** — Phase 1 feature, drives retention
4. **Email preferences UI** — last P3 launch readiness item (deferred until notification emails exist)
5. **AI/Agent API Phase 1** — date range filtering, rate limit headers, stats endpoint

## Roadmap

### Now: Artists Page & Launch Hardening

- [x] All P0/P1 security + legal items
- [x] CI: required checks, Codecov, E2E parallelization
- [x] Observability: PostHog + Sentry
- [x] Backend refactoring P0-P3 (DI, interfaces, handler tests)
- [x] City filter shift+click multi-select UX
- [x] **Artists list page (`/artists`)** — show counts, search, multi-city filters, compact grid
- [x] **Venues page search + multi-city filters** — VenueSearch, shift+click multi-select
- [ ] Email preferences UI (deferred — no notification emails to control yet)
- [ ] Backend middleware tests (P4 refactoring — context logging)

### Q1 2026: Foundation Features

Focus: Core utility that creates daily habits

- [ ] Calendar sync (ICS feed for saved shows)
- [ ] One-click "Add to Google Calendar"
- [ ] Show reminders (email, 24h before)
- [ ] Basic show save analytics
- [ ] AI/Agent API Phase 1: date range filtering, rate limit headers, stats endpoint

Success metric: 20% of active users have 3+ saved shows

### Q2 2026: Social & Artist Engagement

Focus: Network effects and artist-driven growth

- [ ] "Going" / "Interested" buttons on shows
- [ ] Attendance counts on show cards
- [ ] Artist claim flow (Spotify OAuth verification)
- [ ] Basic artist dashboard
- [ ] User follow system

Success metrics: 100+ artists claimed, 30% of shows have 5+ "interested" users

### Q3 2026: Personalization & Monetization

Focus: Personalized experience and revenue activation

- [ ] Spotify listening history → "For You" recommendations
- [ ] Weekly personalized email digest
- [ ] Venue analytics dashboard (free + paid tiers)
- [ ] Post-show photo uploads and ratings/reviews

### Q4 2026+: Scale

- [ ] Multi-city data model refactor
- [ ] Tucson launch (test expansion city)
- [ ] AI/Agent API Phases 2-3: search, MCP server, OpenAI plugin

## Active Plans

| Plan | Focus | Status |
|------|-------|--------|
| `plans/active/2026-launch-readiness-checklist.md` | Security, legal, compliance | All critical done, email prefs UI pending |
| `plans/active/backend-refactoring-checklist.md` | DI, interfaces, code quality | P0-P3 done, P4 pending (middleware tests) |
| `plans/active/backend-test-coverage-plan.md` | Coverage targets by tier | 68.5% overall, 411 handler tests |
| `plans/active/e2e-testing-playwright.md` | E2E test coverage | 69 tests, Tier 1-3 partial |
| `plans/active/observability-posthog-sentry.md` | Analytics + error tracking | Complete |

## Key Files

| Area | Files |
|------|-------|
| Frontend entry | `frontend/app/` (Next.js App Router) |
| API client | `frontend/lib/api.ts`, `frontend/lib/hooks/` |
| Backend routes | `backend/internal/api/routes/routes.go` |
| Service container | `backend/internal/services/container.go` |
| E2E tests | `frontend/e2e/`, `frontend/playwright.config.ts` |
| CI | `.github/workflows/` |

## Related Ideas

- `ideas/future-product-roadmap.md` — 7 features across 4 phases
- `ideas/monetization-strategies.md` — revenue models
- `ideas/admin-feature-ideas.md` — internal tooling
- `ideas/shared-component-refactoring.md` — UI/UX improvements
