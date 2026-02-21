# Testing Learnings

Reference for agents working on backend tests, E2E tests, or test infrastructure.

## Backend Test Infrastructure

- **Testcontainers image**: `postgres:18` (match prod)
- **Integration tests**: Use testcontainers pattern from `user_test.go`
- **Coverage script**: `backend/scripts/coverage.sh` — excludes non-logic packages
- **Migration 000027**: Uses `CREATE INDEX CONCURRENTLY` — must strip keyword in test migrations (not allowed in transactions)

## Backend Coverage Summary

| Layer | Coverage | Tests |
|-------|----------|-------|
| Services | 76.8% | ~600+ |
| Handlers | ~75%+ | 411 (262 unit + 149 integration) |
| Middleware | 92.7% | 65 (55 unit + 10 integration) |
| Utils | 100% | — |
| Config | 100% | — |
| **Overall** | **68.5%** | — |

### Service Tests by File

- **Tier 1**: show.go (86), venue.go (61), artist.go (44)
- **Tier 2**: password_validator.go (31, mock HTTP)
- **Tier 3**: audit_log.go (16), show_report.go (34)
- **Tier 4**: saved_show.go (22), favorite_venue.go (30)
- **Tier 5**: cleanup.go (8), slug.go (21, 100%), api_token.go (32), discovery.go (40), data_sync.go (42), admin_stats.go (12), webauthn.go (34), discord.go (40, 98.6%), email.go (14, 100%)
- **Tier 6**: jwt.go (+14), apple_auth.go (+12), extraction.go (+28)
- **Models**: user_webauthn.go (32, 100%), user.go (~90, 83.5% avg)

### Handler Tests by File

- Unit (262 total): auth (34+49=83), admin (77+55=132), artist (20+20=40), show (18+35=53), saved_show (23), favorite_venue (22), show_report (25), artist_report (24), audit_log (7), venue (9), passkey (6), oauth_account (4), apple_auth (2), oauth_handlers (11)
- Integration (149 total): admin (34), show (27), artist (24), venue (19), oauth_handlers (12), favorite_venue (12), saved_show (10), show_report (11)
- Mock-based tests (227 total, 20 mock structs): hand-written struct-with-func-fields mocks in `handler_unit_mock_helpers_test.go`, covering service interaction paths (success, error, pagination, audit log, Discord, cookie auth, token generation, email flows, account recovery)
- Round 1 (68 tests): SavedShow, FavoriteVenue, AuditLog, ShowReport, ArtistReport
- Round 2 (159 tests): Artist (20), Show (35), Admin (55), Auth (49)
- All handler files have tests, admin guard verified for all 26 admin handlers

## Backend Test Gotchas

- **GORM bool**: `IsActive: false` on `Create` is zero-value — GORM skips it, DB default `TRUE` wins. Fix: create as true, then `Update` to false.
- **GORM AAGUID**: Maps `AAGUID` to `aa_guid` but DB column is `aaguid` — use raw SQL for `webauthn_credentials`.
- **WebAuthn v0.15**: `options.Response.CredentialExcludeList` (not `ExcludeCredentials`)
- **Huma middleware testing**: Use `humatest.NewContext(nil, req, rr)` — creates test `huma.Context` from standard http types
- **JWT middleware integration tests**: `jwt_integration_test.go` — 10 tests with testcontainers covering full happy path (JWT → DB user lookup → user in context). Covers HumaJWT (bearer, cookie, inactive, deleted), Lenient (valid, expired-within-grace, inactive), Optional (valid, inactive), Chi (bearer). Uses `testing.Short()` skip.
- **Handler integration gotchas**: Verified venues auto-approve shows, `FavoriteVenue` is idempotent (`FirstOrCreate`), `UnpublishShow` sets "private" not "pending", `VenueEditStatus` is `models.VenueEditStatus` type (not string)
- **OAuth handler testing**: Mock `OAuthCompleter` via `AuthService.SetOAuthCompleter()` to bypass gothic
- **Discord tests**: `httptest.NewServer` + `chan []byte` for async goroutine payloads; construct `DiscordService` directly with test `httpClient`
- **Email tests**: Resend `NewCustomClient(server.Client(), apiKey)` + override `client.BaseURL` to httptest server URL
- **Raw SQL inserts**: For Show/Venue/WebAuthn in user.go tests — GORM model has columns from later migrations

## E2E Test Infrastructure (Playwright)

- **Stack**: Playwright + ephemeral Docker PostgreSQL (port 5433) + Go backend + Next.js frontend
- **Config**: `frontend/playwright.config.ts`, tests in `frontend/e2e/`
- **Run**: `bun run test:e2e` (from frontend/), requires port 8080 free
- **64 tests**: Tier 1 (23) + Tier 2 (24) + Tier 3 (17)
- **Docker**: `backend/docker-compose.e2e.yml` — don't use `--wait` flag (migrate one-shot container causes failure)

### E2E Test Gotchas

- **Auth selectors**: Use `#password` (not `getByLabel('Password')`), `{ name: 'Sign in', exact: true }`
- **Flaky save-show**: Tests now serial (`test.describe.configure({ mode: 'serial' })`) — DB race
- **SSR API_BASE_URL**: Detail pages use `NODE_ENV` check for dev fallback — do NOT set `NEXT_PUBLIC_API_URL` in env (breaks cookie proxy)
- **API mocking**: `page.route('**/api/...')` intercepts at browser level; use 200+`success:false` (not 5xx) to avoid error fixture
- **AI textarea**: Use `getByPlaceholder('Paste show details')` — `locator('textarea')` matches ShowForm description too
- **Rate limit**: Auth endpoints rate-limited 10/min/IP — query E2E DB directly via `psql` instead of calling `/auth/login`
- **Dialog strict mode**: Shadcn dialogs stay in DOM; use `getByRole('heading', { name: '...' })` for card titles and `getByRole('dialog', { name: '...' })` to scope
- **JWT helper**: `e2e/helpers/jwt.ts` uses `jose` for HS256 tokens (verify-email/magic-link tests)
- **Seed data**: `go run ./cmd/seed` from `backend/` dir, then `setup-db.sh` SQL inserts
- **Test users**: `e2e-user@test.local` / `e2e-admin@test.local` / `e2e-unverified@test.local`, password: `e2e-test-password-123`
