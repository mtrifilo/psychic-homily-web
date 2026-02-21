# LLM Context Loader

> Read this file first when starting a new session. It tells you what to read next based on your task.

## Project Summary

Psychic Homily is a community-driven live music calendar for the Arizona music scene. Users browse upcoming shows, save favorites, and submit new shows. Admins moderate submissions and manage venues/artists. A separate discovery app scrapes venue websites to auto-import events.

**Stack**: Next.js 16 frontend (React 19, TanStack Query, Tailwind 4) + Go backend (Chi, Huma v2, GORM, PostgreSQL 18). Deployed on Vercel (frontend) and a VPS (backend). CI via GitHub Actions with Codecov coverage.

**State**: v1 feature-complete. 69 E2E tests, 68.5% backend coverage, 805+ frontend unit tests. Favorite cities multi-select shipped. iOS app code written, Xcode project pending.

---

## Task-to-Doc Routing

| Task | Read first | Then |
|------|-----------|------|
| Add a backend feature | `CLAUDE.md` (Backend Conventions) | `plans/active/backend-refactoring-checklist.md` |
| Add a frontend feature | `CLAUDE.md` (Frontend Conventions) | — |
| Write backend tests | `learnings/testing.md` | `plans/active/backend-test-coverage-plan.md` |
| Add an E2E test | `learnings/testing.md` (E2E section) | `plans/active/e2e-testing-playwright.md` |
| Fix a production issue | `learnings/production.md` | — |
| Work on discovery providers | `CLAUDE.md` (Discovery App) | — |
| Add an admin feature | `CLAUDE.md` (Admin section) | `ideas/admin-feature-ideas.md` |
| Plan a new feature | `status.md` | `ideas/future-product-roadmap.md` |
| Work on iOS app | `plans/active/ios-app-v1.md` | — |
| Set up observability | `plans/active/observability-posthog-sentry.md` | — |
| Review launch readiness | `plans/active/2026-launch-readiness-checklist.md` | — |
| Understand monetization | `ideas/monetization-strategies.md` | `ideas/future-product-roadmap.md` |

---

## Key Conventions (Quick Reference)

These are the most common gotchas. Full details in `CLAUDE.md`.

- **JSONB columns**: Use `*json.RawMessage` (not `datatypes.JSON` — that package isn't in go.mod)
- **Services**: Constructor pattern `NewXService(db *gorm.DB)`, registered in `services/container.go`
- **Handlers**: Take pre-instantiated services, registered in `routes/routes.go`
- **Frontend hooks**: In `lib/hooks/`, use `apiRequest` from `lib/api.ts`
- **Query keys**: Defined in `lib/queryClient.ts` → `queryKeys` object
- **Migrations**: Numbered SQL in `db/migrations/` — latest is **000032** (favorite_cities)
- **Test commands**: Backend `go test ./...` | Frontend `bun run test:run` | E2E `bun run test:e2e`
- **Package managers**: Frontend = `bun`, Backend = `go`

---

## Current Checkpoint

- v1 feature-complete, pre-launch hardening phase
- CI: GitHub Actions (backend tests, frontend tests, E2E) with Codecov coverage reporting
- 69 E2E tests (3 parallel workers in CI), 68.5% backend coverage, 805+ frontend unit tests
- Favorite cities multi-select + multi-city show filtering shipped
- Discovery app: 5 venue providers (ticketweb, jsonld, seetickets, emptybottle, wix)
- iOS app: Swift files written, Xcode project setup pending
- All P0/P1 security + legal launch items complete

---

## Guardrails

- **Never** use `datatypes.JSON` — use `*json.RawMessage` for JSONB columns
- **Never** modify files in `frontend/components/ui/` — those are Shadcn primitives
- **Never** commit `backend/server` binary
- **Never** use `npm`/`yarn`/`pnpm` — frontend uses `bun` exclusively
- **Skip** `plans/completed/` — archived reference only
- **Skip** `cmd/backfill-slugs/main.go` — has pre-existing compilation errors, not related to the app

---

## Plan Template (for new plans in `plans/active/`)

```markdown
# [Plan Title]

## Purpose
What this plan achieves and why. 1-3 sentences.

## Status
Current state: done, in progress, next.

## Scope
- Included
- Excluded (non-goals)

## Implementation
[Checklists, specs, or steps]

## Acceptance Criteria
- Testable conditions that define "done"

## Learnings
[Added during work — gotchas, decisions, things to remember]
```
