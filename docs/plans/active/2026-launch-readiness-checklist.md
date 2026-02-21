# 2026 Launch Readiness Plan for Psychic Homily

## Executive Summary

Based on codebase exploration and 2026 best practices research, this plan identifies gaps between your current implementation and what's needed for a safe public launch. Items are prioritized by legal/security risk.

**Launch Context**:
- **Timeline**: 1-2 weeks (ASAP)
- **Expected Users**: ~10 in first year
- **Risk Tolerance**: Some items deferred due to small scale

---

## Current State Summary

### What's Already Implemented (Strong Foundation)
- **Authentication**: Password (bcrypt), OAuth (Google/GitHub), Magic Links, Passkeys/WebAuthn
- **Password Security**: HaveIBeenPwned breach checking, strength validation
- **Session Management**: HTTP-only cookies, configurable SameSite, JWT with type validation
- **Account Deletion**: Soft delete with 30-day grace period, deletion summary
- **Logging**: Structured logging with request ID correlation
- **CI/CD**: GitHub Actions with zero-downtime deployment, rollback capability
- **Backups**: Local + GCS backup scripts with restore procedures
- **Documentation**: Comprehensive deployment and ops guides
- **Pre-Release Security Audit**: IDOR fix, production secret validation, input validation (email, text length, price range, offset), N+1 query fix, advisory lock for race conditions, frontend UX hardening (status-code 404 detection, retry buttons, Load More pagination, clickable duplicate links) — see `/docs/plans/completed/pre-release-audit-checklist.md`

---

## Priority 1: CRITICAL (Legal/Security Risk - Must Have Before Launch)

### 1.1 Privacy Policy Page
**Status**: ✅ COMPLETE
**File**: `frontend/app/privacy/page.tsx`
**URL**: `/privacy`

**Implemented**:
- Categories of data collected and sources
- Specific purposes for data use
- Third-party sharing (Resend, Railway, Google/GitHub OAuth, Discord, Spotify/Bandcamp/SoundCloud embeds)
- User rights by jurisdiction (All users, California/CCPA, EEA/GDPR)
- Global Privacy Control (GPC) signal support
- Cookie usage disclosure
- Data retention periods (30-day soft delete)
- Contact information for privacy requests

**Action items remaining**:
- [x] Update to disclose PostHog analytics usage (after PostHog implemented)

### 1.2 Terms of Service Page
**Status**: ✅ COMPLETE
**File**: `frontend/app/terms/page.tsx`
**URL**: `/terms`

**Implemented**:
- Acceptance of terms with clickwrap notice
- Eligibility (16+ age requirement)
- User accounts and security
- User-generated content license and ownership
- Acceptable use policy
- Intellectual property rights
- DMCA/Copyright policy with agent designation
- **Artist and Venue Information** (Section 10) - correction/removal requests, right of publicity considerations
- Third-party services disclaimer
- Disclaimers and limitation of liability ($100 cap)
- Indemnification
- Dispute resolution with binding arbitration (30-day opt-out)
- Class action waiver
- Termination and survival clauses
- Governing law (Arizona)

**Action items remaining**:
- [x] Create email addresses: `privacy@`, `legal@`, `dmca@`, `corrections@`, `hello@` @psychichomily.com (configured via name.com forwarding to psychichomily@gmail.com)
- [ ] Register DMCA agent at copyright.gov (~$6) - RECOMMENDED
- [x] Add ToS checkbox to registration flow (implemented in `/frontend/app/auth/page.tsx` and `/frontend/components/auth/passkey-signup.tsx`)
- [x] Add links to footer (`frontend/components/Footer.tsx` - Privacy Policy, Terms of Service, Contact)
- [ ] ~~Have lawyer review before launch~~ DEFERRED (acceptable at small scale)

### 1.3 Rate Limiting
**Status**: ✅ COMPLETE
**Files**:
- `backend/internal/api/middleware/ratelimit.go` - Rate limit middleware
- `backend/internal/api/routes/routes.go` - Applied to auth routes

**Implemented**:
- Auth endpoints (`/auth/login`, `/auth/register`, `/auth/magic-link/*`): **10 req/min per IP**
- OAuth endpoints (`/auth/login/{provider}`, `/auth/callback/{provider}`): **10 req/min per IP**
- Passkey endpoints (`/auth/passkey/login/*`, `/auth/passkey/signup/*`): **20 req/min per IP**
- Returns proper 429 Too Many Requests with JSON response and Retry-After header
- Logs rate limit hits for monitoring

**Library**: `go-chi/httprate` v0.15.0

### 1.4 Security Headers
**Status**: ✅ COMPLETE
**Files**:
- `backend/internal/api/middleware/security.go` - Security headers middleware
- `backend/cmd/server/main.go` - Applied middleware

**Headers implemented**:
| Header | Value | Purpose |
|--------|-------|---------|
| `X-Content-Type-Options` | `nosniff` | Prevent MIME sniffing |
| `X-Frame-Options` | `DENY` | Prevent clickjacking |
| `X-XSS-Protection` | `1; mode=block` | Legacy XSS filter |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Control referrer info |
| `Permissions-Policy` | `geolocation=(), microphone=(), camera=(), payment=(), usb=()` | Disable unused features |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | Force HTTPS (production only) |
| `Content-Security-Policy` | `default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'` | Strict CSP for API |
| `X-Permitted-Cross-Domain-Policies` | `none` | Block Flash/PDF cross-domain |

**Verification**: Test with https://securityheaders.com after deployment

### 1.5 Data Export (GDPR Right to Portability)
**Status**: ✅ COMPLETE
**Files**:
- `backend/internal/api/handlers/auth.go` - Added `ExportDataHandler`
- `backend/internal/services/user.go` - Added `ExportUserData` and `ExportUserDataJSON` functions
- `backend/internal/api/routes/routes.go` - Added route `/auth/account/export`
- `frontend/lib/api.ts` - Added `EXPORT_DATA` endpoint
- `frontend/lib/hooks/useAuth.ts` - Added `useExportData` hook
- `frontend/components/SettingsPanel.tsx` - Added Export Data section with download button

**Exports**:
- User profile (id, email, name, admin status, email verified, timestamps)
- Email preferences (notifications, marketing)
- Connected OAuth accounts (provider, provider ID, linked date)
- Passkeys (credential ID, name, created/last used dates)
- Saved shows (show ID, saved date)
- Submitted shows (show ID, status, timestamps)

**Format**: JSON file download with human-readable formatting

---

## Priority 2: HIGH (Security/Trust - Should Have Before Launch)

### 2.1 Cookie Consent Banner
**Status**: ✅ COMPLETE
**Files**:
- `frontend/lib/context/CookieConsentContext.tsx` - Consent state management with GPC detection
- `frontend/components/layout/CookieConsentBanner.tsx` - Banner component
- `frontend/components/layout/CookiePreferencesDialog.tsx` - Preferences dialog

**Implemented**:
- Accept All / Reject All buttons with equal prominence
- Global Privacy Control (GPC) signal detection and respect
- Do Not Track (DNT) signal detection
- 6-month consent duration with automatic expiry
- Consent versioning for re-prompting on policy changes
- Cookie preferences accessible via footer link
- localStorage persistence
- `canUseAnalytics` flag for gating analytics

### 2.2 PostHog Analytics
**Status**: ✅ COMPLETE
**Files**:
- `frontend/lib/posthog.ts` - PostHog client initialization with GDPR-safe defaults
- `frontend/components/layout/PostHogProvider.tsx` - React provider with consent integration

**Implemented**:
- `opt_out_capturing_by_default: true` - No tracking until consent given
- Integrates with Cookie Consent context (`canUseAnalytics` flag)
- Automatic pageview tracking on route changes
- User identification on login (email, admin status)
- Session recordings with `maskAllInputs: true` for privacy
- Automatic opt-out and data reset when consent revoked

**Setup**:
1. ~~Create PostHog account at posthog.com (free tier: 1M events/month)~~ ✅ Done
2. ~~Create project, get API key (starts with `phc_`)~~ ✅ Done
3. ~~Add to production environment variables~~ ✅ Done
4. Enable Session Replay in PostHog dashboard (optional)

**Action items remaining**:
- [x] Create PostHog account and get API key
- [x] Add key to production environment variables
- [x] Update Privacy Policy to disclose PostHog usage

### 2.3 Enhanced Health Check
**Status**: ✅ COMPLETE
**File**: `backend/internal/api/handlers/health.go`

**Implemented**:
- Database connectivity check with `PingContext()`
- Component-level health status with latency and error details
- Overall status: `healthy`, `degraded`, or `unhealthy`
- Timestamp for monitoring correlation

**Response format**:
```json
{
  "status": "healthy",
  "components": {
    "database": {
      "status": "healthy",
      "latency": "1.23ms"
    }
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Note**: Redis check not implemented (Redis not currently used in backend)

### 2.4 Error Tracking (Sentry)
**Status**: ✅ COMPLETE
**Files**:
- `frontend/sentry.server.config.ts` - Server-side config
- `frontend/sentry.edge.config.ts` - Edge runtime config
- `backend/cmd/server/main.go` - Sentry middleware registered

**Frontend instrumentation** (explicit `Sentry.captureException`/`captureMessage`):
| File | Errors Captured |
|------|-----------------|
| `app/api/oembed/route.ts` | External oEmbed failures (Bandcamp/Spotify) |
| `app/api/bandcamp/album-id/route.ts` | Bandcamp page scraping failures |
| `app/api/ai/extract-show/route.ts` | AI extraction errors (missing key, credits, API errors) |
| `app/api/[...path]/route.ts` | Backend 5xx responses, network failures |
| `app/api/admin/artists/[id]/bandcamp/route.ts` | Update/clear failures |
| `app/api/admin/artists/[id]/spotify/route.ts` | Update/clear failures |
| `app/api/admin/artists/[id]/discover-music/route.ts` | AI discovery failures, update failures |
| `app/sitemap.ts` | Sitemap generation failures (shows/venues/artists fetch) |
| `lib/api.ts` | Auth endpoint network failures, 5xx responses |
| `lib/blog.ts` | Filesystem errors reading blog posts |
| `lib/mixes.ts` | Filesystem errors reading mixes |
| `app/shows/[slug]/page.tsx` | Server component API 5xx errors |
| `app/venues/[slug]/page.tsx` | Server component API 5xx errors |

**Backend instrumentation** (explicit `sentry.CaptureException`/`CaptureMessage`):
| File | Errors Captured |
|------|-----------------|
| `internal/services/discord.go` | Webhook failures, non-2xx responses |
| `internal/services/email.go` | Resend API failures (verification, magic link, recovery) |
| `internal/services/music_discovery.go` | Discovery endpoint failures, 5xx responses |

**Tags used for filtering**:
- `service` - Component name (auth, oembed, sitemap, discord, email, etc.)
- `error_type` - Error category (network_failure, service_error, credits_exhausted, etc.)
- `operation` - Specific operation (update, clear, getBlogPost, etc.)

**Setup required**:
- Frontend: Set `NEXT_PUBLIC_SENTRY_DSN` in production env
- Backend: Set `SENTRY_DSN` in production env

### 2.5 Account Lockout
**Status**: ✅ COMPLETE
**Files**:
- `backend/internal/models/user.go` - Added `FailedLoginAttempts`, `LockedUntil` fields
- `backend/internal/services/user.go` - Added lockout helper methods and authentication logic
- `backend/internal/api/handlers/auth.go` - Added `ACCOUNT_LOCKED` error handling
- `backend/internal/errors/auth.go` - Added `CodeAccountLocked` error code
- `backend/db/migrations/000014_add_account_lockout.up.sql` - Migration for new columns

**Implementation**:
- Lock after 5 failed attempts for 15 minutes
- Returns `ACCOUNT_LOCKED` error code with remaining minutes
- Resets failed attempts on successful login
- Hidden from JSON API responses for security (`json:"-"`)

### 2.6 CORS Debug Mode Off
**Status**: ✅ COMPLETE
**File**: `backend/cmd/server/main.go`

Changed `Debug: true` to `Debug: !isProduction` so CORS debug logging is only enabled in development.

### 2.7 Report Show Issue
**Status**: ✅ COMPLETE (In-app reporting implemented, supersedes mailto link)
**Files**:
- `frontend/components/shows/ReportShowButton.tsx` - Report button component
- `frontend/components/shows/ReportShowDialog.tsx` - Report type selection dialog
- `backend/internal/api/handlers/show_report.go` - API handlers
- `backend/internal/services/show_report.go` - Business logic
- `backend/internal/models/show_report.go` - Database model
- `backend/db/migrations/000018_add_show_reports.up.sql` - Migration

**Implemented**:
- "Report Issue" button on show detail page (authenticated users only)
- Report types: Cancelled, Sold Out, Inaccurate Info
- Optional details textarea for additional context
- Discord notification to admins on new report
- Admin panel "Reports" tab with pending count badge
- Dismiss (spam/invalid) and Resolve (action taken) workflows
- One report per user per show constraint

**API Endpoints**:
| Method | Path | Description |
|--------|------|-------------|
| POST | `/shows/{id}/report` | Submit a report |
| GET | `/shows/{id}/my-report` | Check if user has reported |
| GET | `/admin/reports` | List pending reports |
| POST | `/admin/reports/{id}/dismiss` | Mark as spam/invalid |
| POST | `/admin/reports/{id}/resolve` | Mark as action taken |

### 2.8 Discord Notifications
**Status**: ✅ COMPLETE
**Files**:
- `backend/internal/services/discord.go` - Discord webhook service
- `backend/internal/config/config.go` - Configuration loading

**Code implemented**:
- New user registration notifications
- New show submission notifications
- Show status change notifications (approve/reject)
- Show report notifications
- Error capture to Sentry on webhook failures

**Current state**:
- ✅ Discord server "Psychic Homily" exists
- ✅ `#alerts-stage` channel with webhook (Stage environment)
- ✅ `#alerts-production` channel with webhook (Production environment)
- ✅ Both environments configured in Railway

**Why separate channels**: Prevents alert fatigue from stage testing, clear visual separation of environments

---

## Deferred Items (Acceptable Risk at Small Scale)

These items are typically recommended but deferred given ~10 user scale:

### LLC Formation
**Typical recommendation**: Form business entity for liability protection
**Decision**: Deferred until user base grows or revenue justifies
**Risk accepted**: Personal liability, manageable at small scale
**Revisit when**: Approaching 100+ users or generating revenue

### Legal Review of ToS/Privacy Policy
**Typical recommendation**: Have lawyer review before launch
**Decision**: Current AI-generated policies are comprehensive and reasonable
**Revisit when**: Before significant scaling or if legal issue arises

### ~~Uptime Monitoring~~ MOVED TO LAUNCH REQUIREMENTS
See section 3.2 - UptimeRobot (5-minute setup, moved to launch blockers)

---

## Priority 3: MEDIUM (Production Hardening - Plan for Soon After Launch)

### 3.1 Email Preferences UI
**Status**: Database fields exist, no UI
**Files to create**: `frontend/components/settings/notification-preferences.tsx`

### 3.2 Uptime Monitoring (UptimeRobot)
**Status**: ✅ COMPLETE
**Risk**: Won't know if site goes down until users report it
**Tool**: UptimeRobot (free tier: 50 monitors, 5-minute intervals)

**Setup (5 minutes)**:
1. Create account at uptimerobot.com
2. Add HTTP monitor for `https://psychichomily.com/api/health`
3. Add HTTP monitor for `https://psychichomily.com` (frontend)
4. Configure alert contacts (email, optional: SMS, Slack, Discord)
5. Set check interval to 5 minutes

**Monitor settings**:
- Type: HTTP(s)
- Keyword: `"status":"healthy"` for health endpoint
- Alert after 2 consecutive failures (avoid flapping)

### 3.3 Centralized Logging (Axiom)
**Status**: Deferred to post-launch (Railway logs sufficient for ~10 users)
**Risk**: Limited log retention in Railway, no search or alerting
**Tool**: Axiom (free tier: 500GB ingest/month - very generous)

**Setup**:
1. Create account at axiom.co
2. Create dataset for psychic-homily logs
3. Get API token

**Backend integration**:
- Option A: Railway log drain to Axiom (easiest, no code changes)
- Option B: Add Axiom Go SDK for structured logging

**Frontend integration** (optional):
- Add `@axiomhq/js` for client-side error/event logging
- Or rely on Sentry for frontend errors

**Benefits over Railway logs**:
- 30-day retention (vs Railway's limited retention)
- Full-text search across all logs
- Create alerts on error patterns
- Dashboard for log analytics

### 3.4 Automated Backup Schedule
**Status**: Manual scripts exist
**Recommendation**: Cron job or Railway scheduled task for daily backups

### 3.5 Session Revocation
**Status**: No token revocation mechanism
**Risk**: Compromised tokens remain valid until expiry
**Recommendation**: Add token blacklist or switch to short-lived tokens + refresh tokens

### 3.6 In-App Show Issue Reporting
**Status**: ✅ COMPLETE (Implemented as part of 2.7)
**Files created**:
- `frontend/components/shows/ReportShowButton.tsx` - Button component
- `frontend/components/shows/ReportShowDialog.tsx` - Modal form component
- `frontend/components/admin/ShowReportCard.tsx` - Admin report card
- `frontend/components/admin/DismissReportDialog.tsx` - Dismiss confirmation
- `frontend/components/admin/ResolveReportDialog.tsx` - Resolve confirmation
- `frontend/app/admin/reports/page.tsx` - Admin reports page
- `frontend/lib/hooks/useShowReports.ts` - User hooks
- `frontend/lib/hooks/useAdminReports.ts` - Admin hooks
- `backend/internal/api/handlers/show_report.go` - API handlers
- `backend/internal/services/show_report.go` - Business logic
- `backend/internal/models/show_report.go` - Database model
- `backend/db/migrations/000018_add_show_reports.up.sql` - Migration

**Features implemented**:
- Issue type selection: Cancelled, Sold Out, Inaccurate Info
- Optional details textarea
- Stores in database for admin review
- Discord notification to admins
- Admin UI with Reports tab in admin panel
- Dismiss (spam/invalid) and Resolve (action taken) actions
- Pending reports count badge on admin tab

**Benefits achieved**:
- Structured data for easier processing
- Track report volume and patterns
- Works well on mobile
- Button changes to "Reported" state after submission
- Full admin dashboard for reports

---

## Priority 4: NICE TO HAVE (Future Improvements)

- Multi-factor authentication (TOTP)
- OpenTelemetry distributed tracing
- AI-powered observability (per [2026 trends](https://middleware.io/blog/observability-predictions/))
- Status page for users
- Prometheus metrics export

---

## Implementation Order

| Phase | Items | Status |
|-------|-------|--------|
| **Done** | Privacy Policy (1.1), ToS (1.2), Rate Limiting (1.3), Security Headers (1.4), Data Export (1.5), Cookie Consent (2.1), PostHog Analytics (2.2), Health Check (2.3), Sentry (2.4), Account Lockout (2.5), CORS Debug (2.6), Report Show Issue (2.7), Discord Notifications (2.8), UptimeRobot (3.2), In-App Issue Reporting (3.6), Pre-Release Security Audit (see `pre-release-audit-checklist.md`) | ✅ Complete |
| **Launch Blockers** | None! | ✅ Ready to launch |
| **Config Complete** | ~~PostHog account + API key (2.2)~~, Sentry DSN (2.4), Privacy Policy update (2.2) | ✅ Done |
| **Recommended** | DMCA Agent Registration (~$6) | 30 min |
| **Post-Launch** | Email Preferences UI (3.1), Axiom Logging (3.3), Automated Backups (3.4), Session Revocation (3.5) | Ongoing |
| **Deferred** | LLC Formation, Legal Review | When scale justifies |
| **Future** | MFA, OpenTelemetry, Status Page, Prometheus | Roadmap |

---

## Verification Plan

1. **Privacy/Terms**: Pages load, links in footer work
2. **Rate Limiting**: Test with `curl` loop, verify 429 responses
3. **Security Headers**: Check with https://securityheaders.com
4. **Data Export**: Download and verify JSON contains all user data
5. **Cookie Consent**: Test accept/reject flows, verify GPC signal handling
6. **PostHog**: Open incognito, verify no posthog requests before accepting cookies; accept cookies, navigate pages, verify pageviews in PostHog dashboard
7. **Health Check**: Kill DB, verify degraded response
8. **Sentry**: Verify errors appear in dashboard by testing:
   - Frontend: Click "Discover Music" on an artist without `ANTHROPIC_API_KEY` configured (503 error)
   - Backend: Check Discord webhook failures appear if webhook URL is invalid
   - Filter by tags: `service:auth`, `service:sitemap`, `service:oembed`, etc.
9. **Report Issue**: Click "Report Issue" on show page, submit report, verify Discord notification and admin panel shows it
10. **Discord Notifications**: Test each environment separately:
    - Stage: Register new user or submit show, verify message appears in `#alerts-stage`
    - Production: Same test, verify message appears in `#alerts-production`
    - Verify embeds show correct colors (green=new user, blue=new show, orange=status change)
11. **UptimeRobot**: Verify monitors show "Up", test alert by briefly stopping service

---

## Post-Implementation Verification

After implementing launch blockers:
1. [x] Cookie banner appears on first visit
2. [x] Rejecting cookies prevents PostHog initialization (code complete, verify after adding API key)
3. [x] Accepting cookies initializes PostHog (code complete, verify after adding API key)
4. [x] Sentry instrumentation added to frontend API routes, lib files, server components, and backend services
5. [x] Sentry DSN added to production env (frontend: `NEXT_PUBLIC_SENTRY_DSN`, backend: `SENTRY_DSN`)
6. [ ] Sentry captures test error in dashboard (trigger by visiting a page with a backend error)
7. [x] Privacy Policy mentions PostHog
8. [x] "Report Issue" button on show detail pages opens dialog (authenticated users)
9. [x] Submitting report sends Discord notification (code complete)
10. [x] Admin panel shows Reports tab with pending count
11. [x] Admin can dismiss or resolve reports
12. [x] Discord webhook configured for Stage (`#alerts-stage` channel)
13. [x] Discord webhook configured for Production (`#alerts-production` channel)
14. [ ] Test Discord notification in Stage environment (create user or submit show)
15. [x] UptimeRobot monitors configured (API health + frontend, Discord alerts)
17. [ ] All 5 email addresses receive test emails
18. [x] PostHog account created and API key added to production env
19. [ ] PostHog dashboard shows pageviews after accepting cookies

---

## Key Resources

- [OWASP Top 10 2025](https://owasp.org/www-project-top-ten/)
- [OWASP Cheat Sheet Series](https://cheatsheetseries.owasp.org/index.html)
- [2026 Privacy Compliance Checklist](https://secureprivacy.ai/blog/privacy-compliance-checklist-2026)
- [CCPA 2026 Requirements](https://secureprivacy.ai/blog/ccpa-requirements-2026-complete-compliance-guide)
- [Production Readiness Checklist](https://www.cortex.io/post/how-to-create-a-great-production-readiness-checklist)
