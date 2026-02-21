# Project Status

> Last updated: 2026-02-21

## Current Checkpoint

- v1 feature-complete, pre-launch hardening phase
- 69 E2E tests (parallel CI), 68.5% backend coverage, 805+ frontend unit tests
- CI: GitHub Actions with required checks + Codecov coverage reporting
- Favorite cities multi-select + multi-city show filtering shipped
- Discovery app: 5 providers with artist name cleanup + editable imports
- iOS app: Swift code written, Xcode project setup pending

## Recent Completions

- Favorite cities feature: multi-city filtering, save defaults, settings UI
- CI improvements: required status checks on main, Codecov integration, E2E parallelization (3 workers)
- Nav improvements: inline theme toggle, "My Collection" in main nav bar
- Discovery: `cleanArtistName` utility across all providers, editable artist names in ImportPanel
- E2E test fixes: city filter URL format, flaky venue autocomplete stabilized
- Wix discovery provider: HTTP-only provider for Celebrity Theatre
- Handler mock tests: 411 total (262 unit + 149 integration)
- Backend refactoring P0-P3: service container, handler DI, 22 service interfaces
- Production readiness audit (security, error sanitization, accessibility)
- E2E test suite: 69 Playwright tests with error detection fixture
- Observability: PostHog + Sentry fully integrated

## Active Work

| Area | Plan | Status |
|------|------|--------|
| Launch readiness | `plans/active/2026-launch-readiness-checklist.md` | All critical items done |
| Backend refactoring | `plans/active/backend-refactoring-checklist.md` | P0-P3 done, P4 pending (middleware tests) |
| Backend test coverage | `plans/active/backend-test-coverage-plan.md` | 68.5% overall, 411 handler tests |
| E2E tests | `plans/active/e2e-testing-playwright.md` | 69 tests, 3 parallel workers in CI |
| Observability | `plans/active/observability-posthog-sentry.md` | PostHog + Sentry frontend/backend done |
| iOS app | `plans/active/ios-app-v1.md` | Swift files written, Xcode project pending |

## Next Tasks (Suggested Priority)

1. **iOS Xcode project** — wire up Swift files into a buildable project
2. **Backend middleware tests** — Phase 4 of refactoring checklist
3. **Email preferences UI** — last P3 launch readiness item

## Blockers

None currently.
