# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## First Steps

When starting a new task, read `dev-docs/llm-context.md` first. It has a task-to-doc routing table that tells you exactly which files to read for context, plus a current project checkpoint and key guardrails. Only drill into specific docs when your task requires it.

## Package Managers

- **Frontend**: Always use `bun` (not npm/yarn/pnpm)
- **Backend**: Use `go` commands

## Project Structure

- `/frontend` - Next.js 16 app (React 19, TanStack Query, Tailwind CSS 4, Vitest)
- `/backend` - Go API (Chi router, Huma v2, GORM, PostgreSQL 18)
- `/discovery` - Local Bun+Playwright app for scraping venue events and importing to the backend
- `/dev-docs` - Implementation docs, plans, learnings, and roadmap (start with `dev-docs/llm-context.md`)

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

**Dependency injection:** Services are eagerly instantiated in `services/container.go` (`NewServiceContainer(db, cfg)`) → passed to handler constructors → handlers registered in `routes/routes.go`.

### Backend Conventions

- **Handlers** (`internal/api/handlers/`): HTTP layer only — parse Huma request structs, extract user from context via `middleware.GetUserFromContext()`, call services, map responses. Constructor takes pre-instantiated services.
- **Services** (`internal/services/`): Business logic + DB operations. Constructor pattern: `NewXService(db *gorm.DB)` — if nil, falls back to `db.GetDB()` singleton.
- **Models** (`internal/models/`): GORM structs with `TableName()` methods. Use `*json.RawMessage` for JSONB columns (not `datatypes.JSON`).
- **Routes**: Public/protected/admin routes registered in `routes/routes.go`. Admin routes don't use separate middleware — handlers check `user.IsAdmin` internally.
- **Migrations**: Numbered SQL files in `db/migrations/` (`000XXX_name.up.sql` / `.down.sql`).
- **Fire-and-forget**: Discord notifications and audit log writes log errors but never fail parent operations.

#### Huma API Framework Quirks

- **All request body fields are required by default** — even pointer types (`*bool`, `*string`) are treated as required unless explicitly marked optional with struct tags
- Huma returns 422 "validation failed" errors when required fields are missing from the request body
- If you see "expected required property X to be present" errors, ensure the frontend always sends that field

### Frontend Conventions

- **API client** (`lib/api.ts`): `apiRequest()` utility with `credentials: 'include'` for HTTP-only cookie auth. In development, browser requests proxy through Next.js (`/api/*` → `localhost:8080`); in production, requests go direct to `api.psychichomily.com`.
- **Hooks** (`lib/hooks/`): TanStack Query hooks per domain. Queries use `queryKeys` from `lib/queryClient.ts`. Mutations invalidate via `createInvalidateQueries()`.
- **Query client** (`lib/queryClient.ts`): 5-min staleTime, smart retry (no retry on 4xx, up to 3 on 5xx). Global error handlers detect session expiry and invalidate auth profile.
- **Auth**: `AuthContext` wraps app, checks `/auth/profile` on mount. Auth token is HTTP-only cookie — frontend never accesses it directly. Supports email/password, magic link, OAuth (Google/GitHub), and passkeys (WebAuthn).
- **Admin**: Tab-based UI in `app/admin/page.tsx` with dynamic imports. Components in `components/admin/` with barrel export in `index.ts`.
- **Component dirs**: Domain directories (artists, shows, venues) have `index.ts` barrel files. Shadcn primitives in `components/ui/` — don't modify directly.
- **URLs**: Artists, venues, and shows use SEO-friendly slugs. Handlers support both numeric IDs and slugs.

### Backend Test Patterns

- **Service integration tests**: Use testcontainers (`postgres:18`), `testify/suite`. Migrations loaded from `../../db/migrations/`.
- **Handler integration tests**: Direct function calls (no httptest/router needed for Huma handlers). Shared setup in `handler_integration_helpers_test.go`.
- **Unit tests**: Pure functions tested without DB. Nil DB → `"database not initialized"` error path.
- **Migration 27 gotcha**: Uses `CREATE INDEX CONCURRENTLY` — must strip keyword in test migrations (not allowed in transactions).
- **GORM bool gotcha**: `IsActive: false` on Create is zero-value, GORM skips it → DB default wins. Fix: create as true, then Update to false.

### E2E Test Patterns (Playwright)

Global setup starts Docker PostgreSQL (port 5433), runs migrations, seeds data (`go run ./cmd/seed` + `setup-db.sh`), starts Go backend, captures auth state.

- **Test users**: `e2e-user@test.local` / `e2e-admin@test.local` (password: `e2e-test-password-123`)
- **Auth fixtures**: `e2e/.auth/user.json`, `e2e/.auth/admin.json`
- **Error detection**: Auto-fail on console errors/5xx responses (`e2e/fixtures/error-detection.ts`)
- **API mocking**: `page.route('**/api/...')` intercepts at browser level; use 200+`success:false` (not 5xx) to avoid error fixture

### Discovery App

Local tool for importing venue events. Lives in `/discovery` with Bun server + Playwright scraping + React UI. Venue providers in `src/server/providers/`. Backend integration via `DiscoveryService` (`services/discovery.go`) which deduplicates by `source_venue` + `source_event_id` and auto-approves shows for verified venues.

**Provider types:**
- `ticketweb` — Playwright-based, waits for `window.all_events` global (Valley Bar, Crescent Ballroom)
- `jsonld` — HTTP fetch + JSON-LD `MusicEvent` parsing, Playwright enrichment for performer lineup (The Van Buren, Arizona Financial Theatre)
- `wix` — HTTP-only (no Playwright), fetches sitemap XML → concurrent page fetches → JSON-LD `Event` extraction (Celebrity Theatre)
