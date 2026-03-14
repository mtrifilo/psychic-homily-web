# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## First Steps

When starting a new task, read `docs/llm-context.md` first. It has a task-to-doc routing table that tells you exactly which files to read for context, plus a current project checkpoint and key guardrails. Only drill into specific docs when your task requires it.

## Package Managers

- **Frontend**: Always use `bun` (not npm/yarn/pnpm)
- **Backend**: Use `go` commands

## Project Structure

- `/frontend` - Next.js 16 app (React 19, TanStack Query, Tailwind CSS 4, Vitest). New features use `features/` modules (co-located components/hooks/types); existing features remain in `components/` + `lib/hooks/`.
- `/backend` - Go API (Chi router, Huma v2, GORM, PostgreSQL 18)
- `/ios` - Native iOS app (Swift 6, SwiftUI, iOS 18+, XcodeGen)
- `/discovery` - Local Bun+Playwright app for scraping venue events and importing to the backend
- `/docs` - LLM workspace: specs, strategy, plans, learnings (start with `docs/llm-context.md`)
- `/human-docs` - Human-facing guides: contributing, workflow, release, FAQ, troubleshooting

## Running Locally

```bash
# Frontend (from /frontend)
bun install && bun run dev

# Backend (from /backend)
go run ./cmd/server
```

## Testing

```bash
# Frontend unit/component tests (from /frontend)
bun run test              # Watch mode
bun run test:run          # Single run
bun run test -- path/to/file.test.ts  # Single file

# Backend tests (from /backend)
go test ./...                                    # All tests
go test ./internal/services/ -run TestShowSuite  # Single suite
go test ./internal/api/handlers/ -run TestName   # Single test

# Backend coverage (from /backend)
./scripts/coverage.sh

# E2E tests (from /frontend, stop dev backend first — port 8080 must be free)
bun run test:e2e          # Headless
bun run test:e2e:ui       # Interactive Playwright UI
```

## Architecture

### Backend Request Flow

```
HTTP Request → Chi Router → Global Middleware → Huma Adapter → Route Group Middleware → Handler → Service → GORM/DB
```

**Middleware layers (applied in order):**
1. Global (Chi): Request ID → Sentry → Logging → CORS → Security Headers
2. Route groups (Huma): JWT auth (strict/lenient/optional), rate limiting

**Dependency injection:** Services are organized into domain sub-packages under `internal/services/` and eagerly instantiated in `services/container.go` (`NewServiceContainer(db, cfg)`) → passed to handler constructors → handlers registered in `routes/routes.go`.

### Backend Conventions

- **Handlers** (`internal/api/handlers/`): HTTP layer only — parse Huma request structs, extract user from context via `middleware.GetUserFromContext()`, call services, map responses. Constructor takes pre-instantiated services.
- **Services** (`internal/services/`): Business logic + DB operations organized into domain sub-packages. Constructor pattern: `NewXService(db *gorm.DB)`.
- **Service sub-packages:**
  - `services/contracts/` — all service interfaces and shared request/response types
  - `services/catalog/` — show, venue, artist, festival, label, release
  - `services/auth/` — auth (OAuth), JWT, Apple auth, WebAuthn, password validator
  - `services/engagement/` — bookmark, saved show, favorite venue, calendar, reminder
  - `services/notification/` — email, Discord
  - `services/pipeline/` — extraction, fetcher, discovery, orchestrator, venue source config, music discovery, scheduler
  - `services/user/` — user, contributor profile
  - `services/admin/` — admin stats, API token, artist report, audit log, cleanup, data sync, show report, revision
  - Root `services/` — `container.go` (wiring), `interfaces.go` (compile-time checks), `aliases.go` (backward-compat type aliases), `collection.go`, `request.go` (not yet extracted into sub-packages)
- **Models** (`internal/models/`): GORM structs with `TableName()` methods. Use `*json.RawMessage` for JSONB columns (not `datatypes.JSON`).
- **Routes**: Public/protected/admin routes registered in `routes/routes.go`. Admin routes don't use separate middleware — handlers check `user.IsAdmin` internally.
- **Migrations**: Numbered SQL files in `db/migrations/` (`000XXX_name.up.sql` / `.down.sql`).
- **Fire-and-forget**: Discord notifications and audit log writes log errors but never fail parent operations.

#### Huma API Framework Quirks

- **All request body fields are required by default** — even pointer types (`*bool`, `*string`) are treated as required unless explicitly marked optional with struct tags
- **Query/path/header params must NOT use pointer types** (`*uint`, `*string`) — Huma panics at route registration. Use value types with zero-value checks instead.
- Huma returns 422 "validation failed" errors when required fields are missing from the request body
- If you see "expected required property X to be present" errors, ensure the frontend always sends that field

### Frontend Conventions

- **API client** (`lib/api.ts`): `apiRequest()` utility with `credentials: 'include'` for HTTP-only cookie auth. In development, browser requests proxy through Next.js (`/api/*` → `localhost:8080`); in production, requests go direct to `api.psychichomily.com`.
- **Hooks** (`lib/hooks/`): TanStack Query hooks organized into domain subdirectories (`admin/`, `shows/`, `artists/`, `venues/`, `releases/`, `labels/`, `festivals/`, `auth/`, `user/`, `common/`). Each has a barrel `index.ts`. Import from subdirectory: `@/lib/hooks/shows/useShows`. Queries use `queryKeys` from `lib/queryClient.ts`. Mutations invalidate via `createInvalidateQueries()`.
- **Query client** (`lib/queryClient.ts`): 5-min staleTime, smart retry (no retry on 4xx, up to 3 on 5xx). Global error handlers detect session expiry and invalidate auth profile.
- **Auth**: `AuthContext` wraps app, checks `/auth/profile` on mount. Auth token is HTTP-only cookie — frontend never accesses it directly. Supports email/password, magic link, OAuth (Google/GitHub), and passkeys (WebAuthn).
- **Admin**: Tab-based UI in `app/admin/page.tsx` with dynamic imports. Shared admin components (pending show cards, report cards, dialogs) in `components/admin/` with barrel export. Page-specific admin components (management UIs, dashboard, user cards) live in `app/admin/<route>/_components/`.
- **Component dirs**: Domain directories (artists, shows, venues) have `index.ts` barrel files. Shadcn primitives in `components/ui/` — don't modify directly.
- **Page-specific components** (`_components/`): Components used by exactly one route live in `app/<route>/_components/` using the Next.js `_` prefix convention. Import with `@/` alias (e.g., `@/app/admin/releases/_components/ReleaseManagement`). Components used by 2+ routes stay in `components/`.
- **Feature modules** (`features/`): Co-located feature modules with `components/`, `hooks/`, `types.ts`, and root `index.ts` public API. All features migrated: releases, labels, festivals, blog, auth, collections, requests, shows, artists, venues. Import from root `index.ts` only, never internal paths. Shared code used by 2+ features stays in `lib/` or `components/shared/`.
- **URLs**: Artists, venues, and shows use SEO-friendly slugs. Handlers support both numeric IDs and slugs.

### Backend Test Patterns

- **Service integration tests**: Use testcontainers (`postgres:18`), `testify/suite`. Migrations run via `testutil.RunAllMigrations()`.
- **Handler integration tests**: Direct function calls (no httptest/router needed for Huma handlers). Shared setup in `handler_integration_helpers_test.go`.
- **Unit tests**: Pure functions tested without DB. Nil DB → `"database not initialized"` error path.
- **Migration helper**: `internal/testutil/migrations.go` — `RunAllMigrations(t, sqlDB, migrationDir)` globs all `*.up.sql` files, sorts them, strips `CONCURRENTLY`, and runs them. New migrations work automatically — no test files to update.
- **GORM bool gotcha**: `IsActive: false` on Create is zero-value, GORM skips it → DB default wins. Fix: create as true, then Update to false.

### E2E Test Patterns (Playwright)

Global setup starts Docker PostgreSQL (port 5433), runs migrations, seeds data (`go run ./cmd/seed` + `setup-db.sh`), starts Go backend, captures auth state.

- **Test users**: `e2e-user@test.local` / `e2e-admin@test.local` (password: `e2e-test-password-123`)
- **Auth fixtures**: `e2e/.auth/user.json`, `e2e/.auth/admin.json`
- **Error detection**: Auto-fail on console errors/5xx responses (`e2e/fixtures/error-detection.ts`)
- **API mocking**: `page.route('**/api/...')` intercepts at browser level; use 200+`success:false` (not 5xx) to avoid error fixture

### Discovery App

Local tool for importing venue events. Lives in `/discovery` with Bun server + Playwright scraping + React UI. Venue providers in `src/server/providers/`. Backend integration via `DiscoveryService` (`services/pipeline/discovery.go`) which deduplicates by `source_venue` + `source_event_id` and auto-approves shows for verified venues.

**Provider types:**
- `ticketweb` — Playwright-based, waits for `window.all_events` global (Valley Bar, Crescent Ballroom)
- `jsonld` — HTTP fetch + JSON-LD `MusicEvent` parsing, Playwright enrichment for performer lineup (The Van Buren, Arizona Financial Theatre)
- `seetickets` — Playwright-based, scrapes SeeTickets widget event containers (The Rebel Lounge)
- `emptybottle` — Playwright-based, scrapes date-organized event listings (The Empty Bottle)
- `wix` — HTTP-only (no Playwright), fetches sitemap XML → concurrent page fetches → JSON-LD `Event` extraction (Celebrity Theatre)
