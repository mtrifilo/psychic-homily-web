# End-to-End Testing with Playwright

**Date:** 2026-02-01 (updated 2026-02-09)
**Status:** Phase 1 Complete, Phase 2 Complete, Tier 3 Partial (64 tests passing — Tier 1 + Tier 2 complete, Tier 3 #17-23 done)

## Overview

Plan for adding Playwright E2E tests to cover critical user journeys. The application currently has unit/component tests via Vitest but no E2E coverage. This document outlines a phased approach to implementing comprehensive E2E testing.

**Deployment Strategy:**
- **Local development:** Run tests against `localhost:3000` while writing/debugging tests
- **CI (staging gate):** Full suite runs against staging after deploy, blocks production on failure

---

## Current Testing State

| Aspect | Status |
|--------|--------|
| Unit/Component Tests | Vitest (16 test files) |
| E2E Tests | Playwright (64 tests — Tier 1 + Tier 2 complete, Tier 3 partial) |
| Test Coverage | React hooks (useAuth, useShows, etc.) + E2E user journeys |

---

## Automatic Error & Exception Detection

Every test should double as a regression detector. Playwright can capture runtime errors passively while a user journey runs, so we get error coverage "for free" without writing explicit assertions for each failure mode.

### Shared Error-Collecting Fixture

**Implemented:** `frontend/e2e/fixtures/error-detection.ts`

Collects typed `ErrorEntry` objects (`{ type, message }`) for `pageerror`, `console.error`, `request-failed`, and `server-error`. Filters known acceptable patterns (e.g., `401.*\/auth\/profile`, `favicon`). Auto-asserts `errors.length === 0` in fixture teardown.

### What This Catches Automatically

| Error Type | Source | Example |
|------------|--------|---------|
| Uncaught exceptions | `page.on('pageerror')` | Unhandled promise rejection, null reference, React render crash |
| Console errors | `page.on('console')` | Failed `fetch()`, React hydration mismatch, missing env vars |
| Network failures | `page.on('requestfailed')` | DNS resolution failure, CORS block, connection refused |
| Server errors | `page.on('response')` | 500 Internal Server Error, 502 Bad Gateway |

### Usage

Tests import from the fixture instead of from `@playwright/test` directly:

```typescript
// e2e/pages/shows.spec.ts
import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test('browse shows list', async ({ page, errors }) => {
  await page.goto('/shows')
  await expect(page.getByRole('heading', { name: /upcoming shows/i })).toBeVisible()
  // ... normal assertions ...
  // errors fixture auto-asserts no errors at teardown
})
```

### Filtering Known/Acceptable Errors

Ignored patterns are defined in `IGNORED_PATTERNS` in `error-detection.ts`. Currently filters:
- `401.*\/auth\/profile` — expected when not logged in
- `favicon` — browser quirk on favicon load failures
- `\/api\/auth\/profile.*401` — auth check on unauthenticated pages

---

## Prioritized User Journeys

### Priority Tiers

Journeys ranked by **business impact x likelihood of breakage**. Tier 1 tests should be implemented first — they cover the pages with the most traffic and the core product loop.

### Tier 1 — Highest Value (implement first, ~8 tests)

These cover the core product loop: discover shows, log in, save them.

| # | Journey | Route(s) | Auth | Why First |
|---|---------|----------|------|-----------|
| 1 | Homepage loads, shows render | `/` | No | Most-visited page, catches SSR/hydration breaks |
| 2 | Browse shows list + pagination | `/shows` | No | Core discovery, tests API pagination |
| 3 | Show detail page renders | `/shows/[slug]` | No | SEO-critical, deep link target |
| 4 | Login with email/password | `/auth` | No | Gateway to all authenticated features |
| 5 | Venue detail page renders | `/venues/[slug]` | No | SEO-critical, public page |
| 6 | Artist detail page renders | `/artists/[slug]` | No | SEO-critical, public page |
| 7 | Save/unsave a show | `/shows/[slug]` | Yes | Core engagement action, tests auth + API |
| 8 | Logout | any | Yes | Basic auth lifecycle |

### Tier 2 — High Value (~8 tests)

Authenticated features and the submission pipeline.

| # | Journey | Route(s) | Auth | Why |
|---|---------|----------|------|-----|
| 9 | Registration flow | `/auth` | No | User acquisition funnel |
| 10 | Submit a show (manual form) | `/submissions` | Yes | Core content creation, requires verified email |
| 11 | View saved shows collection | `/collection` | Yes | Retention feature |
| 12 | Favorite/unfavorite a venue | `/venues/[slug]` | Yes | Engagement feature |
| 13 | City filter on shows list | `/shows` | No | Key UX feature, tests query params |
| 14 | Venue list page | `/venues` | No | Public browsing |
| 15 | Profile page loads | `/profile` | Yes | Account management |
| 16 | Protected route redirects to login | `/submissions`, `/collection` | No | Auth guard works correctly |

### Tier 3 — Medium Value (~8 tests)

AI features, admin workflows, edge cases.

| # | Journey | Route(s) | Auth | Why |
|---|---------|----------|------|-----|
| 17 | AI form filler - image extraction | `/submissions` | Yes | High-value feature, complex API interaction | **Done** — `e2e/pages/ai-filler.spec.ts` |
| 18 | AI form filler - text extraction | `/submissions` | Yes | Alternate extraction path | **Done** — `e2e/pages/ai-filler.spec.ts` |
| 19 | Admin: approve pending show | `/admin/pending-shows` | Admin | Operational — shows don't go live without this | **Done** — `e2e/admin/pending-shows.spec.ts` |
| 20 | Admin: verify venue | `/admin/unverified-venues` | Admin | Operational | **Done** — `e2e/admin/verify-venue.spec.ts` |
| 21 | Admin: approve/reject venue edit | `/admin/venue-edits` | Admin | Operational | **Done** — `e2e/admin/venue-edits.spec.ts` |
| 22 | Email verification flow | `/verify-email` | No | Requires intercepting token | **Done** — `e2e/auth/verify-email.spec.ts` |
| 23 | Magic link authentication | `/auth/magic-link` | No | Alternate auth path | **Done** — `e2e/auth/magic-link.spec.ts` |
| 24 | View my submissions | `/submissions` | Yes | Content management |

### Tier 4 — Lower Priority (~6 tests)

Blog, categories, edge cases, responsive.

| # | Journey | Route(s) | Auth | Why |
|---|---------|----------|------|-----|
| 25 | Blog listing + detail | `/blog`, `/blog/[slug]` | No | Content pages, less critical |
| 26 | Category browse | `/categories/[category]` | No | Alternate discovery path |
| 27 | DJ sets listing + detail | `/dj-sets`, `/dj-sets/[slug]` | No | Secondary content type |
| 28 | Account recovery | `/auth/recover` | No | Low-frequency auth flow |
| 29 | Mobile viewport - navigation | all | No | Responsive layout |
| 30 | Mobile viewport - show submission | `/submissions` | Yes | Mobile form usability |

---

## User Journeys to Test

### Authentication Flows
| Journey | Complexity | Notes |
|---------|------------|-------|
| Email/password login | Medium | Standard flow |
| Registration + email verification | Medium | Requires token handling |
| Google OAuth | High | Needs mock OAuth provider |
| Passkey/WebAuthn | High | Requires browser API simulation |
| Magic link authentication | High | Need to intercept tokens |
| Account recovery | Medium | Similar to magic link |
| Logout | Low | Straightforward |

### Content Creation
| Journey | Complexity | Notes |
|---------|------------|-------|
| Submit a show | Medium | Requires authenticated + verified user |
| AI form filler extraction | Medium | API mocking or real Claude calls |
| Edit/delete submitted show | Medium | Owner permissions |
| Show status transitions | Low | publish/unpublish/private |

### Discovery & Browsing
| Journey | Complexity | Notes |
|---------|------------|-------|
| Browse shows list | Low | Public, pagination/filters |
| View show detail | Low | Public page |
| View venue detail | Low | Public page |
| View artist detail | Low | Public page |
| Save show to collection | Low | Authenticated user |
| Favorite venues | Low | Authenticated user |

### Admin Workflows
| Journey | Complexity | Notes |
|---------|------------|-------|
| Approve venue edits | Medium | Admin role required |
| Verify unverified venues | Medium | Admin role required |
| Import shows | Medium | Admin role required |

---

## Complexity Factors

### High Complexity Areas
1. **WebAuthn/Passkey** - Requires special Playwright CDP commands
2. **OAuth flow** - Needs mock provider or test credentials
3. **Email verification/magic links** - Need strategy for token extraction
4. **Database state** - Tests need clean/seeded data between runs

### Backend Coordination
- Backend runs on `localhost:8080` in development
- Frontend proxies API calls for cookie-based auth
- Options for E2E:
  - **Mock API responses** (faster, less realistic)
  - **Full backend integration** (slower, more realistic)
  - **Hybrid** (mock complex flows, real for simple CRUD)

---

## Phased Implementation Plan

### Phase 1: Foundation (MVP)
**Effort:** ~1 week

#### Setup Tasks
- [x] Install Playwright (`bun add -D @playwright/test`)
- [x] Create `playwright.config.ts` with globalSetup/globalTeardown
- [x] Create `backend/docker-compose.e2e.yml` (ephemeral PostgreSQL, no volumes)
- [x] Create `e2e/setup-db.sh` (seed data + insert future shows + test user accounts)
- [x] Create `e2e/global-setup.ts` (start DB, seed, start backend, capture auth state)
- [x] Create `e2e/global-teardown.ts` (stop backend, destroy DB container)
- [x] Create error-detection fixture (`e2e/fixtures/error-detection.ts`)
- [x] Create auth fixtures (`e2e/fixtures/auth.ts`, `e2e/fixtures/index.ts`)
- [x] Add npm scripts (`test:e2e`, `test:e2e:ui`, etc.)
- [x] Add `e2e/.auth/` and Playwright artifacts to `.gitignore`

#### Initial Test Suite: Tier 1 + Tier 2 (~16 tests)

All tests use the error-detection fixture, so each test also validates zero console errors, zero uncaught exceptions, zero 5xx responses, and zero failed network requests.

- [x] Homepage loads, shows render (Tier 1) — `e2e/pages/home.spec.ts`
- [x] Browse shows list + pagination (Tier 1) — `e2e/pages/shows.spec.ts`
- [x] Show detail page renders (Tier 1) — `e2e/pages/show-detail.spec.ts`
- [x] Venue detail page renders (Tier 1) — `e2e/pages/venue-detail.spec.ts`
- [x] Artist detail page renders (Tier 1) — `e2e/pages/artist-detail.spec.ts`
- [x] Login with email/password (Tier 1) — `e2e/auth/login.spec.ts`
- [x] Save/unsave a show (Tier 1) — `e2e/pages/save-show.spec.ts`
- [x] Logout (Tier 1) — `e2e/auth/login.spec.ts`
- [x] Registration flow (Tier 2) — `e2e/auth/register.spec.ts`
- [x] Submit show via manual form (Tier 2) — `e2e/pages/submit-show.spec.ts`
- [x] View saved shows collection (Tier 2) — `e2e/pages/collection.spec.ts`
- [x] Favorite/unfavorite a venue (Tier 2) — `e2e/pages/favorite-venue.spec.ts`
- [x] City filter on shows list (Tier 2) — `e2e/pages/city-filter.spec.ts`
- [x] Venue list page (Tier 2) — `e2e/pages/venues.spec.ts`
- [x] Profile page loads (Tier 2) — `e2e/pages/profile.spec.ts`
- [x] Protected route redirects to login (Tier 2) — `e2e/pages/protected-routes.spec.ts`

#### Configuration

**Implemented:** `frontend/playwright.config.ts`

Key settings:
- `testDir: './e2e'`, single `chromium` project (Desktop Chrome)
- `globalSetup` / `globalTeardown` for full lifecycle management
- `webServer: { command: 'bun run dev', reuseExistingServer: !process.env.CI }`
- `trace: 'on-first-retry'`, `screenshot: 'only-on-failure'`
- `retries: 0` locally, `retries: 2` in CI

#### NPM Scripts (in `package.json`)
```json
{
  "test:e2e": "playwright test",
  "test:e2e:ui": "playwright test --ui",
  "test:e2e:debug": "playwright test --debug",
  "test:e2e:headed": "playwright test --headed"
}
```

#### Local Development Workflow
```bash
# IMPORTANT: Stop the dev backend first (port 8080 must be free)
# The E2E setup starts its own backend against the ephemeral DB

# Run all tests (from /frontend)
bun run test:e2e

# Run with interactive UI (great for debugging)
bun run test:e2e:ui

# Run a specific test file
bun run test:e2e e2e/pages/home.spec.ts

# Run tests matching a pattern
bun run test:e2e -g "homepage"

# Debug mode (step through tests)
bun run test:e2e:debug
```

---

### Phase 2: Full Coverage
**Effort:** ~2-3 weeks additional

#### Auth Edge Cases (8-10 tests)
- [ ] Invalid login credentials
- [ ] Registration validation errors
- [x] Email verification required flow — `e2e/auth/verify-email.spec.ts`
- [x] Magic link authentication — `e2e/auth/magic-link.spec.ts`
- [ ] Account recovery flow
- [ ] Session expiration handling
- [ ] Protected route redirects
- [ ] OAuth flow (with mock provider)

#### Content Management (10-12 tests)
- [x] AI form filler - text extraction — `e2e/pages/ai-filler.spec.ts`
- [x] AI form filler - image extraction — `e2e/pages/ai-filler.spec.ts`
- [ ] Edit submitted show
- [ ] Delete submitted show
- [ ] Show visibility transitions
- [ ] Form validation errors
- [ ] Collection management (all tabs)
- [ ] Favorite/unfavorite venues

#### Admin Workflows (6-8 tests)
- [x] Pending shows: display, approve, reject — `e2e/admin/pending-shows.spec.ts` (3 tests)
- [x] Verify unverified venue: display, verify — `e2e/admin/verify-venue.spec.ts` (2 tests)
- [x] Venue edits: display, approve, reject — `e2e/admin/venue-edits.spec.ts` (3 tests)
- [ ] Show import workflow

#### Cross-Browser & Responsive (4-6 tests)
- [ ] Mobile viewport - navigation
- [ ] Mobile viewport - forms
- [ ] Firefox browser
- [ ] Safari/WebKit browser

---

### Phase 3: CI/CD & Maintenance
**Effort:** ~3-5 hours

#### CI Integration
- [ ] GitHub Actions workflow triggered after staging deploy
- [ ] Block production deploy if E2E tests fail
- [ ] Screenshot/video artifacts uploaded on failure
- [ ] Slack/Discord notification on test failures
- [ ] Manual trigger option for re-running tests

#### Deployment Pipeline
```
Push to main
    │
    ▼
Deploy to Staging
    │
    ▼
Run E2E Tests against Staging  ──── Fail ────▶ Alert team, block prod deploy
    │
    Pass
    │
    ▼
Manual approval / Auto-promote to Production
```

#### Maintenance Setup
- [ ] Visual regression testing (optional)
- [ ] Flaky test detection and quarantine
- [ ] Test coverage reporting
- [ ] Documentation for writing new tests

---

## Directory Structure

```
/frontend
├── e2e/
│   ├── .auth/                  # Generated by globalSetup (gitignored)
│   │   ├── user.json           # Authenticated user storage state
│   │   └── admin.json          # Admin user storage state
│   ├── .env                    # Ephemeral DB credentials (not secret)
│   ├── global-setup.ts         # Start DB, seed, start backend, capture auth
│   ├── global-teardown.ts      # Stop backend, destroy DB container
│   ├── setup-db.sh             # Seed data + create test accounts
│   ├── fixtures/
│   │   ├── error-detection.ts  # Auto-fail on console errors, exceptions, 5xx
│   │   ├── auth.ts             # Auth state fixtures (authenticatedPage, adminPage)
│   │   └── index.ts            # Combined fixture (auth + error detection)
│   ├── pages/
│   │   ├── home.spec.ts
│   │   ├── shows.spec.ts
│   │   ├── show-detail.spec.ts
│   │   ├── venue-detail.spec.ts
│   │   ├── artist-detail.spec.ts
│   │   ├── save-show.spec.ts
│   │   ├── venues.spec.ts
│   │   ├── city-filter.spec.ts
│   │   ├── collection.spec.ts
│   │   ├── profile.spec.ts
│   │   ├── favorite-venue.spec.ts
│   │   ├── protected-routes.spec.ts
│   │   ├── submit-show.spec.ts
│   │   └── ai-filler.spec.ts      # AI form filler tests (#17-18)
│   ├── helpers/
│   │   └── jwt.ts                 # JWT generation helper (jose) for verification/magic link tokens
│   ├── auth/
│   │   ├── login.spec.ts
│   │   ├── register.spec.ts
│   │   ├── verify-email.spec.ts   # Email verification flow (#22)
│   │   └── magic-link.spec.ts     # Magic link authentication (#23)
│   ├── admin/
│   │   ├── pending-shows.spec.ts   # Admin approve/reject pending shows (#19)
│   │   ├── verify-venue.spec.ts    # Admin verify unverified venues (#20)
│   │   └── venue-edits.spec.ts     # Admin approve/reject venue edits (#21)
│   └── utils/
│       ├── test-helpers.ts
│       └── api-mocks.ts
├── playwright.config.ts
└── package.json

/backend
├── docker-compose.e2e.yml      # Ephemeral PostgreSQL (no volumes)
└── ...existing files...
```

---

## Test Data Strategy

### Recommended: Ephemeral Database Per Test Run

The project already has all the infrastructure needed for a fully isolated, fresh database per test run:

- `backend/docker-compose.yml` — PostgreSQL 18 + golang-migrate runner
- `backend/db/migrations/` — 27 numbered migrations (full schema)
- `backend/cmd/seed/main.go` — Seeds venues, artists, and shows from YAML/Markdown data files

**The database is fresh every time.** No volume persistence means each `docker compose up` starts with an empty PostgreSQL, runs all migrations, then seeds. When tests finish, `docker compose down` destroys the container and all data.

### How It Works

```
globalSetup (before all tests)
    │
    ├── 1. docker compose -f docker-compose.e2e.yml up -d
    │       → Starts PostgreSQL (no named volume = ephemeral)
    │       → Runs all 27 migrations automatically
    │
    ├── 2. go run ./cmd/seed
    │       → Seeds venues from data/venues.yaml
    │       → Seeds artists from data/bands.yaml
    │       → Seeds shows from content/shows/*.md
    │
    ├── 3. Insert e2e test accounts via SQL
    │       → e2e-test user (verified email, known password)
    │       → e2e-admin user (admin role)
    │
    ├── 4. go run ./cmd/server (test backend)
    │       → Connects to ephemeral DB
    │       → Runs on port 8080
    │
    ├── 5. bun run dev (test frontend)
    │       → Next.js on port 3000
    │       → Proxies API to localhost:8080
    │
    ├── 6. Log in as test users via Playwright
    │       → Save auth state to e2e/.auth/user.json
    │       → Save auth state to e2e/.auth/admin.json
    │
    ▼
Run all Playwright tests (fully isolated, deterministic data)
    │
    ▼
globalTeardown (after all tests)
    │
    ├── Stop backend + frontend processes
    └── docker compose -f docker-compose.e2e.yml down
            → Container destroyed, all data gone
```

### E2E Docker Compose File

**Implemented:** `backend/docker-compose.e2e.yml`

PostgreSQL 18 on port 5433 (avoids conflict with dev DB on 5432) + golang-migrate runner. No named volumes — data destroyed on `down`.

### E2E Seed Script

**Implemented:** `frontend/e2e/setup-db.sh` (run from `backend/` directory)

Steps:
1. Waits for the migrate container to finish (checks exit code)
2. Runs `go run ./cmd/seed` to populate venues, artists, and shows from YAML/Markdown
3. Inserts 55 **future-dated** approved shows (1-55 days out) using PL/pgSQL loop — necessary because all seed shows have 2025 dates and `GetUpcomingShows` filters to `event_date >= today`. 55 shows ensures pagination is triggered (backend default limit is 50)
4. Inserts test users with pre-computed bcrypt hash for password `e2e-test-password-123`
5. Creates `user_preferences` rows for both test users
6. Inserts admin workflow test data: 2 pending shows, 1 unverified venue, 2 pending venue edits (all linked to test user as submitter)

### Playwright globalSetup / globalTeardown

**Implemented:** `frontend/e2e/global-setup.ts` and `frontend/e2e/global-teardown.ts`

**Global setup** sequence:
1. `docker compose -f docker-compose.e2e.yml up -d` (no `--wait` — the migrate one-shot container causes `--wait` to fail)
2. Poll `pg_isready` until PostgreSQL is healthy
3. Run `setup-db.sh` (seed data)
4. Check port 8080 is free (errors with clear message if dev backend is still running)
5. `spawn('go', ['run', './cmd/server'])` with E2E env vars, write PID to `e2e/.backend-pid`
6. Poll `http://localhost:8080/health` until 200
7. Wait for frontend on `http://localhost:3000`
8. Log in as both test users via Playwright browser, save `storageState` to `e2e/.auth/`

**Global teardown**: reads PID file, kills process group (`-pid` for detached), runs `docker compose down`.

**Gotchas discovered during implementation:**
- `docker compose --wait` fails when a one-shot container (migrate) exits with code 0 — use `up -d` and poll manually
- `getByLabel('Password')` matches both the input and the "Show password" toggle button — use `#password` selector
- `getByRole('button', { name: 'Sign in' })` matches both the passkey button and submit button — use `exact: true`
- Shadcn `CardTitle` renders as `<div>` not a heading — use `getByText()` instead of `getByRole('heading')`
- TanStack Form's `canSubmit` starts `true` before validation triggers — don't assert `toBeDisabled()` on initial empty form; assert after user interaction
- Backend checks HaveIBeenPwned — test registration passwords must be unique/uncommon (e.g. `Xq9!mzPh2wLk_e2e`), common passwords like `TestPassword123!` get rejected
- **Shadcn dialogs stay in DOM after closing** — `getByText('Show Title')` can match both the card heading and the dialog description text. Always use `getByRole('heading', { name: '...' })` for card/dialog titles
- **Cookie consent dialog coexists as `role="dialog"`** — `getByRole('dialog')` matches both the cookie banner and your app dialog. Use `getByRole('dialog', { name: 'Dialog Title' })` to scope to a specific dialog
- **Admin seed data placement in `setup-db.sh`** — admin test data (pending shows, unverified venue, pending edits) must be inserted AFTER `UPDATE venues SET verified = true` (line 78) and AFTER test user inserts, so the new unverified venue isn't auto-verified and `submitted_by` references exist

**Defects found and fixed during E2E implementation:**
- **Seed command missing slugs** (`cmd/seed/main.go`): Venues, artists, and shows created by the seed command had null slugs because the seed doesn't call `utils.GenerateSlug()`. The backfill migrations (000016, 000017) run before the seed, so they can't fix data that doesn't exist yet. Fixed by adding slug generation to the seed command.
- **SSR metadata fetching production API in dev** (`app/shows/[slug]/page.tsx`, `app/venues/[slug]/page.tsx`, `app/artists/[slug]/page.tsx`, `app/sitemap.ts`): Server-side `generateMetadata` used `process.env.NEXT_PUBLIC_API_URL || 'https://api.psychichomily.com'`, which falls through to production in development. Fixed by adding `NODE_ENV === 'development'` check to use `http://localhost:8080` locally — matching the logic already in `lib/api.ts`.
- Note: Setting `NEXT_PUBLIC_API_URL` in `.env.development` does NOT work — it makes the browser bypass the `/api` proxy, breaking cookie-based auth (SameSite restrictions on cross-origin fetch).
- **Browser-level API mocking with `page.route()`**: For tests that depend on external services (AI extraction, etc.), use `page.route('**/api/endpoint', route => route.fulfill({ ... }))` to intercept requests at the browser level before they reach the Next.js proxy. The backend never receives the request.
- **JWT token generation for auth tests**: `e2e/helpers/jwt.ts` uses `jose` to create valid HS256 JWTs matching the Go backend's token structures. Functions: `createVerificationToken(userId, email)` and `createMagicLinkToken(userId, email)`. Uses the known E2E JWT secret.
- **Unverified test user**: `e2e-unverified@test.local` (password: `e2e-test-password-123`) has `email_verified=false`. Used for email verification tests. Auth state is NOT captured — tests use the public verification page.
- **Seed venues not verified** (`cmd/seed/main.go`): Venues created by the seed command have `verified = false` (GORM zero-value default). The public `/venues` endpoint filters `WHERE verified = true`, so seeded venues don't appear on the venue list page. Fixed by adding `UPDATE venues SET verified = true` to the E2E setup script.

### Fresh vs. Persistent: Decision Matrix

| Approach | Data Lifetime | Startup | Best For |
|----------|---------------|---------|----------|
| **No volumes (recommended)** | Destroyed on `down` | ~5-8s (migrations + seed) | CI, deterministic runs |
| **Named volume** | Persists across runs | ~1s (skip migrations) | Fast local iteration |
| **tmpfs volume** | Destroyed on `down`, in-memory | ~3-5s (faster I/O) | CI with speed priority |

Recommended default: **no volumes**. The 5-8s startup cost is negligible compared to the confidence of fully deterministic test data.

For local development iteration, you can skip teardown (`--no-teardown` flag) and reuse the database between runs to save time, then do a clean run before committing.

### Why Not Test Against Staging?

The original doc considered running tests against staging with dedicated test accounts and `[E2E TEST]` prefixed data. The ephemeral DB approach is strictly better:

| Concern | Staging Approach | Ephemeral DB Approach |
|---------|-----------------|----------------------|
| Data pollution | Must prefix + cleanup | Impossible — DB is destroyed |
| Flaky data state | Previous runs leave artifacts | Always clean |
| Parallel safety | Shared accounts conflict | Each worker can get its own DB |
| Seed data control | Whatever staging has | Exactly what you seed |
| Speed | Network latency to staging | All local |
| Offline development | Needs network | Works offline |

Staging tests may still be useful as a **smoke test** after deploy (does the real infra work?), but the primary e2e suite should run against the ephemeral stack.

### Environment Variables & Test Credentials

Hardcoded in `global-setup.ts` and `setup-db.sh` (not secrets — the database only exists during the test run):

| Variable | Value |
|----------|-------|
| DB URL | `postgres://e2euser:e2epassword@localhost:5433/e2edb?sslmode=disable` |
| Test user | `e2e-user@test.local` / `e2e-test-password-123` |
| Admin user | `e2e-admin@test.local` / `e2e-test-password-123` |
| JWT secret | `e2e-jwt-secret-key-for-testing-only` |
| OAuth secret | `e2e-oauth-secret-key-for-testing-only` |

Backend env vars set in `global-setup.ts`: `DATABASE_URL`, `JWT_SECRET_KEY`, `OAUTH_SECRET_KEY`, `CORS_ALLOWED_ORIGINS=http://localhost:3000`, `SESSION_SECURE=false`, `DISCORD_NOTIFICATIONS_ENABLED=false`.

---

## Authentication Fixtures

**Implemented:** `frontend/e2e/fixtures/auth.ts` and `frontend/e2e/fixtures/index.ts`

Auth state is captured once in `globalSetup` and reused across all tests via Playwright's `storageState`. No per-test login needed.

- `auth.ts` extends `error-detection.ts` to provide `authenticatedPage` and `adminPage` fixtures, each with their own browser context loaded from `e2e/.auth/user.json` or `e2e/.auth/admin.json`
- `index.ts` re-exports from `auth.ts` as the combined fixture entry point

Usage:
```typescript
// Unauthenticated test with error detection
import { test } from '../fixtures/error-detection'

// Authenticated test with error detection
import { test } from '../fixtures'
test('my test', async ({ authenticatedPage }) => { ... })

// Admin test with error detection
import { test } from '../fixtures'
test('admin test', async ({ adminPage }) => { ... })
```

---

## Estimated Total Effort

| Phase | Scope | Effort |
|-------|-------|--------|
| Phase 1 | MVP (10-15 tests) | ~1 week |
| Phase 2 | Full coverage (40-50 tests) | ~2-3 weeks |
| Phase 3 | CI/CD integration | ~3-5 hours |
| **Total** | **Comprehensive suite** | **3-4 weeks** |

---

## Staging Smoke Tests (Optional, Separate)

The primary e2e suite runs against the ephemeral local stack. A smaller **smoke test** suite could optionally run against staging after deploy to verify real infrastructure works.

### Smoke Test Scope (~3-5 tests)
- Homepage loads
- Login works
- Show detail page renders
- API responds (health check)

### Staging Considerations
- Dedicated test accounts (credentials in CI secrets)
- No data-mutating tests (read-only journeys)
- Network latency — use generous timeouts
- Rate limiting — space out requests
- Keep separate from the main suite (`bun run test:e2e:smoke`)

---

## Success Criteria

- [ ] All critical user journeys have E2E coverage
- [ ] Tests run reliably against staging (< 5% flaky rate)
- [ ] Test suite completes in < 10 minutes
- [ ] E2E tests block production deploy on failure
- [ ] New features include E2E tests
- [ ] Clear documentation for writing tests

---

## Resources

- [Playwright Documentation](https://playwright.dev/docs/intro)
- [Playwright with Next.js](https://nextjs.org/docs/app/building-your-application/testing/playwright)
- [Authentication in Playwright](https://playwright.dev/docs/auth)
