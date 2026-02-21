# Production Readiness Audit

Audited: 2026-02-08

## Status

Auth hardening (frontend) completed prior to this audit. Backend auth was confirmed solid — no changes needed.

This document covers the remaining non-auth findings.

## High Priority

### 1. CORS wildcard allows all `*.vercel.app` origins
- [x] Lock down CORS in production

**File:** `backend/cmd/server/main.go:124-132`

The `AllowOriginFunc` accepts any origin ending in `.vercel.app`. Combined with `AllowCredentials: true`, an attacker could deploy their own Vercel site and make authenticated cross-origin requests to the API.

**Fix:** Remove the `*.vercel.app` wildcard in production, or restrict to a specific pattern like `psychic-homily-*.vercel.app`. The `CORS_ALLOWED_ORIGINS` env var already supports explicit origins — just ensure preview URLs are added there if needed.

### 2. No rate limiting on write endpoints
- [x] Add rate limiting to show creation
- [x] Add rate limiting to AI processing endpoint
- [x] Add rate limiting to report submission
- [ ] ~Consider rate limiting admin bulk operations~ (skipped — behind admin auth, low risk)

**File:** `backend/internal/api/routes/routes.go`

Auth endpoints have rate limiting (10 req/min) and passkey endpoints (20 req/min), but all other write endpoints are unprotected:

| Endpoint | Risk |
|---|---|
| `POST /shows` | Could flood admin queue |
| `POST /shows/ai-process` | Calls Anthropic API — expensive |
| `POST /shows/{id}/report` | Could spam admins with reports |
| Admin bulk operations | Abuse potential |

**Fix:** Add per-user or per-IP rate limits using the existing `ratelimit` middleware. Suggested limits:
- Show creation: 10/hour per user
- AI processing: 5/min per user
- Report submission: 5/min per user

### 3. Missing security headers on frontend
- [x] Add security headers to `next.config.ts`

**File:** `frontend/next.config.ts`

No HTTP security headers configured. Missing:
- `X-Frame-Options: DENY` — prevents clickjacking
- `X-Content-Type-Options: nosniff` — prevents MIME sniffing
- `Referrer-Policy: strict-origin-when-cross-origin` — limits referrer leakage
- `Content-Security-Policy` — restricts resource loading (scope to Sentry, PostHog, API domain)

**Fix:** Add `headers()` export to `next.config.ts`.

## Medium Priority

### 4. Environment variable validation gap
- [x] Fail fast if critical env vars are missing in production
- [x] Stop logging default values for secrets

**File:** `backend/internal/config/config.go:228-256`

If `ENVIRONMENT` is unset, it defaults to `"development"` and skips secret validation. Production could silently run with placeholder JWT secrets if env vars are misconfigured. The fallback logging also prints default secret values.

**Fix:** In non-local deployments, require `ENVIRONMENT` to be explicitly set. Don't log fallback values for secret-related env vars.

### 5. N+1 query in `UpdateShowWithRelations`
- [x] Batch venue/artist lookups

**File:** `backend/internal/services/show.go:600-619`

Fetches venues and artists one-by-one in a loop instead of using `WHERE IN`. If a show has 10 venues, this runs 10 queries. Other endpoints (list, get) already use `Preload()` correctly.

**Fix:** Replace the loop with a single `WHERE id IN (?)` query for venues and artists.

### 6. No route-level error boundaries
- [x] Add `error.tsx` for critical routes

**Frontend**

Only `global-error.tsx` exists. A crash in any route takes down the entire page. Adding `error.tsx` to key route segments would allow contextual error recovery.

Priority routes: `app/shows/[slug]/`, `app/admin/`, `app/collection/`, `app/venues/[slug]/`, `app/artists/[slug]/`.

## Low Priority

### 7. Error responses may leak implementation details
- [x] Sanitize error messages in all handlers

Sanitized all response-facing `err.Error()` calls across 8 handler files (admin.go, show.go, venue.go, show_report.go, saved_show.go, favorite_venue.go, health.go, oauth_handlers.go). Error details now logged server-side only; clients receive generic messages with request_id for correlation.

### 8. Console statements in production frontend
- [x] Remove or gate `console.log` calls

Removed 13 `console.log` calls from `discover-music/route.ts` (verbose debug/trace logs + API key debug log). Remaining `console.log` calls are only in `scripts/` (CLI tools) and `e2e/` (test infrastructure). `console.error` calls in catch blocks kept for server-side error logging.

### 9. Accessibility gaps
- [x] Add `role="alert"` to form validation errors
- [ ] Manual keyboard navigation testing on admin workflows

Added `role="alert"` to 22 validation/error elements across 9 files: `FieldInfo` component (covers all TanStack Form fields), auth pages (login, signup, recover, passkey), settings (change password, OAuth accounts, API tokens, settings panel). Also added `aria-invalid` to `FormField` textarea (was only on Input). Radix UI handles dialog keyboard nav internally, but admin-specific workflows haven't been tested.

## Confirmed Solid (no action needed)

- **SQL injection**: All queries use GORM parameterized methods
- **XSS**: React auto-escaping, safe `dangerouslySetInnerHTML` (JSON-LD only)
- **Pagination**: All list endpoints paginated with enforced max limits
- **Graceful shutdown**: SIGTERM/SIGINT handled, 10s timeout, cleanup service stopped
- **Logging & observability**: Structured logging (JSON in prod), Sentry on both ends, audit log for admin actions
- **SEO**: Metadata, JSON-LD, sitemap, robots.txt all configured
- **Auth cookies**: HttpOnly, proper expiry, auto-cleared on 401
- **Admin guards**: Enforced on both frontend (redirect + `enabled` flags) and backend (`IsAdmin` check + 403)
- **Client secrets**: No secrets in `NEXT_PUBLIC_*` vars, Anthropic key server-only
- **External links**: All use `rel="noopener noreferrer"`, venue URLs normalized to prevent `javascript:` injection
- **Ownership verification**: All user-scoped endpoints (saved shows, favorite venues, show reports) extract user ID from JWT context and scope DB queries with `WHERE user_id = ?`
- **Show status filtering**: All public show endpoints (`GetShows`, `GetUpcomingShows`, `GetShowCities`) filter to approved-only — pending/rejected shows never exposed
- **Inactive user exclusion**: `ValidateToken` and `ValidateTokenLenient` check `IsActive` after DB lookup — deactivated users cannot authenticate even with valid JWTs
