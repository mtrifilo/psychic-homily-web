# Backend Test Coverage Plan

**Date:** 2026-02-10
**Overall coverage:** services 75.2%, middleware 92.7%, handlers 54.1%, utils 100%, overall 64.0%+ (all tests passing, 243 handler tests + 55 middleware tests)

## Current Coverage by File

### Well-Covered (>50%)
| File | Coverage | Notes |
|------|----------|-------|
| `services/auth.go` | 100% (8/8) | Fully tested with mocks |
| `handlers/health.go` | 100% (2/2) | |
| `api/routes/routes.go` | 91% (11/12) | Route registration tests |
| ~~`services/extraction.go`~~ | ~~75% (6/8)~~ | **Done 2026-02-08** — see Tier 6 below |
| `middleware/jwt.go` | ~~71% (5/7)~~ | **Updated 2026-02-19** — 92.7% package, 55 unit + 10 integration tests. All middleware variants covered including full happy-path with real DB. |
| ~~`services/apple_auth.go`~~ | ~~55% (5/9)~~ | **Done 2026-02-08** — see Tier 6 below |

### Partially Covered (10-50%)
| File | Coverage | Notes |
|------|----------|-------|
| ~~`handlers/oauth_handlers.go`~~ | ~~42% (3/7)~~ | **Done 2026-02-12** — 100% (7/7), all functions covered |
| ~~`services/jwt.go`~~ | ~~45% (5/11)~~ | **Done 2026-02-08** — see Tier 6 below |
| ~~`services/password_validator.go`~~ | ~~28% (2/7)~~ | **Done 2026-02-08** — see Tier 2 below |
| ~~`services/user.go`~~ | ~~24% (8/33)~~ | **Done 2026-02-08** — 83.5% avg across all 33 functions, see below |
| `handlers/favorite_venue.go` | 16% (1/6) | Constructor only |
| ~~`config/config.go`~~ | ~~16% (2/12)~~ | **Done 2026-02-08** — 100% (12/12), all functions covered |
| ~~`services/saved_show.go`~~ | ~~14% (1/7)~~ | **Done 2026-02-08** — see Tier 4 below |
| ~~`services/favorite_venue.go`~~ | ~~12% (1/8)~~ | **Done 2026-02-08** — see Tier 4 below |
| ~~`services/show_report.go`~~ | ~~11% (1/9)~~ | **Done 2026-02-08** — see Tier 3 below |
| ~~`services/api_token.go`~~ | ~~11% (1/9)~~ | **Done 2026-02-08** — see Tier 5 below |
| ~~`services/data_sync.go`~~ | ~~11% (1/9)~~ | **Done 2026-02-08** — see Tier 5 below |
| `handlers/auth.go` | ~~10%~~ 21.9% (19/19) | **Done 2026-02-08** — all 19 functions covered (pre-service-call paths) |
| `handlers/venue.go` | ~~10%~~ 21.9% (5/10) | **Done 2026-02-08** — auth guards + ID validation covered |

### No Coverage (0% or constructor-only on large files)
| File | Funcs | Lines | Notes |
|------|-------|-------|-------|
| ~~`services/show.go`~~ | ~~1/33~~ | ~~2,167~~ | **Done 2026-02-08** — see below |
| ~~`services/venue.go`~~ | ~~1/26~~ | ~~1,290~~ | **Done 2026-02-08** — see below |
| ~~`handlers/admin.go`~~ | ~~1/29~~ | ~~2,062~~ | **Done 2026-02-08** — see Tier 3 below |
| ~~`handlers/show.go`~~ | ~~1/19~~ | ~~1,574~~ | **Done 2026-02-08** — see Tier 4 below |
| ~~`handlers/auth.go`~~ | ~~2/19~~ | ~~1,775~~ | **Done 2026-02-08** — see Tier 2 below |
| ~~`services/data_sync.go`~~ | ~~1/9~~ | ~~877~~ | **Done 2026-02-08** — see Tier 5 below |
| ~~`handlers/passkey.go`~~ | ~~1/12~~ | ~~817~~ | **Done 2026-02-08** — see Tier 2 below |
| ~~`handlers/venue.go`~~ | ~~1/10~~ | ~~683~~ | **Done 2026-02-08** — see Tier 4 below |
| ~~`services/discovery.go`~~ | ~~1/11~~ | ~~615~~ | **Done 2026-02-08** — see Tier 5 below |
| ~~`services/artist.go`~~ | ~~1/11~~ | ~~562~~ | **Done 2026-02-08** — see below |
| ~~`services/webauthn.go`~~ | ~~1/23~~ | ~~466~~ | **Done 2026-02-08** — see Tier 5 below |
| ~~`services/discord.go`~~ | ~~1/15~~ | ~~432~~ | **Done 2026-02-09** — see below |
| ~~`middleware/security.go`~~ | ~~0/5~~ | — | **Done 2026-02-08** — 100% (5/5) |
| ~~`middleware/ratelimit.go`~~ | ~~0/4~~ | — | **Done 2026-02-08** — ~90% (4/4 functions tested) |
| ~~`utils/slug.go`~~ | ~~0/5~~ | — | **Done 2026-02-08** — see Tier 5 below |

---

## Prioritized Checklist

### Tier 1: Core Business Logic (highest risk, most complex)

These files contain the core domain logic. Bugs here directly affect users and data integrity.

- [x] **`services/show.go`** (2,167 lines, 33 functions) — **Done 2026-02-08**
  - [x] `CreateShow` — show creation with artist/venue resolution (89.5%)
  - [x] `UpdateShow` / `UpdateShowWithRelations` — show editing, artist/venue replacement (87.5% / 78%)
  - [x] `GetShow` / `GetShowBySlug` — show retrieval with preloads (88.9%)
  - [x] `GetShows` — filtering by city, date range (90%)
  - [x] `GetUpcomingShows` — cursor pagination (78.4%, **defect found and fixed** — see Defects section)
  - [x] `GetUserSubmissions` / `GetPendingShows` / `GetRejectedShows` — listing + counts (84-90%)
  - [x] `DeleteShow` — delete with association cleanup (70%)
  - [x] Duplicate detection — same headliner/venue/date blocked, case-insensitive (92.3%)
  - [x] Show status transitions — approve, reject, unpublish, make-private, publish (73-76%)
  - [x] Artist orphan detection on update
  - [x] Status flags — sold out, cancelled toggles (60%)
  - [x] `buildShowResponse` — model-to-response conversion (92.1%)
  - [x] `determineShowStatus` / `fnvHash` / `encodeCursor` — (100%)
  - [x] `ExportShowToMarkdown` — export with round-trip, filename format, not-found (73.9%) — **Done 2026-02-09**
  - [x] `ParseShowMarkdown` — valid, minimal, no desc, multiline, invalid YAML, no frontmatter, empty, multi-artists+venues, heading stop (96.3%) — **Done 2026-02-09**
  - [x] `GetAdminShows` — no filters, status filter, pagination, city filter (64.1%) — **Done 2026-02-09**
  - 86 tests total: 34 unit (nil-db) + 52 integration (testcontainers)

- [x] **`services/venue.go`** (1,290 lines, 26 functions) — **Done 2026-02-08**
  - [x] `CreateVenue` — admin auto-verified, non-admin unverified, duplicate name+city blocked (88%)
  - [x] `GetVenue` / `GetVenueBySlug` — retrieval with error codes (89%)
  - [x] `GetVenues` — filtering by city, state, name (94%)
  - [x] `UpdateVenue` — basic fields, duplicate detection, self-update OK (81%)
  - [x] `DeleteVenue` — success, not-found, has-shows blocked (83%)
  - [x] `SearchVenues` — empty query, prefix match, trigram match, no match (93%)
  - [x] `FindOrCreateVenue` — create new, find existing, case-insensitive, admin verified, slug backfill, validation (94%)
  - [x] `VerifyVenue` — success, idempotent, not-found (61%)
  - [x] `GetVenuesWithShowCounts` — verified-only, sort by count, city filter, pagination (83%)
  - [x] `GetShowsForVenue` — upcoming, past, all, not-found (72%)
  - [x] `GetVenueCities` — distinct cities with counts, verified-only (91%)
  - [x] Pending venue edits — create, duplicate blocked, approve applies changes, reject with reason, cancel own, wrong-user blocked
  - [x] `buildVenueResponse` — unverified hides address/zipcode (100%)
  - [x] `GetUnverifiedVenues` — with show counts, pagination (82%)
  - [x] `GetVenueModel` — raw model retrieval (88%)
  - 61 tests total: 23 unit (nil-db) + 38 integration (testcontainers)

- [x] **`services/artist.go`** (562 lines, 11 functions) — **Done 2026-02-08**
  - [x] `CreateArtist` — success with all fields + slug generated, duplicate name blocked (case-insensitive), social fields, city/state (88%)
  - [x] `GetArtist` / `GetArtistBySlug` / `GetArtistByName` — retrieval + not-found errors, case-insensitive name lookup (89%)
  - [x] `GetArtists` — filtering by city, state, name ILIKE partial match (94%)
  - [x] `UpdateArtist` — basic fields, name change regenerates slug, duplicate name blocked, self-update OK, not-found (90%)
  - [x] `DeleteArtist` — success, not-found (ARTIST_NOT_FOUND), has-shows blocked (ARTIST_HAS_SHOWS) (83%)
  - [x] `SearchArtists` — empty query, short prefix match (<=2 chars), long trigram match (3+ chars), no match (93%)
  - [x] `GetShowsForArtist` — upcoming/past/all time filters, not-found, venue info in response, other artists on bill, excludes non-approved, respects limit (92%)
  - [x] `buildArtistResponse` — model-to-response conversion (100%)
  - [x] Nil-db unit tests for all 9 public methods
  - 44 tests total: 11 unit (nil-db) + 33 integration (testcontainers)

### Tier 2: Auth & Security (security-critical)

Bugs here mean authentication bypasses, account takeovers, or data leaks.

- [x] **`handlers/auth.go`** (1,775 lines, 19 functions) — **Done 2026-02-08**
  - [x] All 19 handler functions covered (pre-service-call validation/auth paths)
  - [x] `LoginHandler` — empty credentials, missing password → `VALIDATION_FAILED`
  - [x] `OAuthLoginHandler` — invalid provider, nil auth service → `SERVICE_UNAVAILABLE`
  - [x] `LogoutHandler` — success (cookie cleared), with/without user context
  - [x] `RefreshTokenHandler` — nil auth service, no user context
  - [x] `GetProfileHandler` — nil auth service, no user context
  - [x] `RegisterHandler` — empty credentials, missing email, nil user service, weak password
  - [x] `SendVerificationEmailHandler` — no user context, already verified
  - [x] `ConfirmVerificationHandler` — empty/invalid token
  - [x] `SendMagicLinkHandler` — empty email
  - [x] `VerifyMagicLinkHandler` — empty/invalid token
  - [x] `ChangePasswordHandler` — no user context, empty passwords, same password
  - [x] `GetDeletionSummaryHandler` — no user context
  - [x] `DeleteAccountHandler` — no user context, OAuth-only user, empty password
  - [x] `ExportDataHandler` — no user context
  - [x] `RecoverAccountHandler` — empty credentials
  - [x] `RequestAccountRecoveryHandler` — empty email
  - [x] `ConfirmAccountRecoveryHandler` — empty/invalid token
  - [x] `GenerateCLITokenHandler` — no user context, non-admin
  - 34 tests total (direct function calls, no httptest needed)

- [x] **`handlers/passkey.go`** (817 lines, 12 functions) — **Done 2026-02-08**
  - [x] Constructor, BeginRegister/FinishRegister/ListCredentials/DeleteCredential NoAuth → `UNAUTHORIZED`
  - [x] BeginSignup empty email → `VALIDATION_FAILED`
  - 6 tests total

- [x] **`middleware/security.go`** (0% → 100%) — **Done 2026-02-08**
  - [x] Security headers (X-Frame-Options, CSP, etc.)
  - [x] `SecurityHeadersWithConfig` — custom CSP, custom frame options, HSTS with preload
  - [x] `isProduction`, `buildCSP`, `DefaultSecurityConfig`
  - 12 tests total

- [x] **`middleware/ratelimit.go`** (0% → ~90%) — **Done 2026-02-08**
  - [x] Rate limit response format (status 429, Content-Type, Retry-After, JSON body)
  - [x] All 3 rate limiter constructors return non-nil middleware
  - 7 tests total

- [x] **`middleware/jwt.go`** (0% → 92.7% package overall) — **Done 2026-02-09**
  - [x] `JWTMiddleware` — no token, invalid header, invalid token, expired token, cookie fallback, header preference (91.9%)
  - [x] `GetUserFromContext` — with user, no user, wrong type (100%)
  - [x] `writeJWTError` — full response validation (100%)
  - [x] `HumaJWTMiddleware` — no token, invalid JWT, expired JWT, valid JWT (DB fail), cookie fallback, invalid API token, invalid auth header, session config cookie clearing, request ID propagation (88.7%)
  - [x] `LenientHumaJWTMiddleware` — no token, invalid token, valid token (DB fail), expired within grace (DB fail), expired beyond grace, cookie fallback (89.7%)
  - [x] `OptionalHumaJWTMiddleware` — no token (proceeds), valid JWT (DB fail, proceeds without user), invalid JWT (proceeds), expired JWT (proceeds), invalid API token (proceeds), cookie fallback, non-Bearer auth header (87.5%)
  - [x] `writeHumaJWTError` — basic response, with session config (Set-Cookie), nil session config (100%)
  - 30 tests total (11 existing + 19 new Huma tests)

- [x] **`middleware/request_id.go`** (0% → 100%) — **Done 2026-02-09**
  - [x] `RequestIDMiddleware` — generates UUID, uses client-provided ID, sets logger in context, calls next (100%)
  - [x] `HumaRequestIDMiddleware` — generates UUID, uses client-provided ID, sets logger in context, calls next (100%)
  - [x] `GetRequestIDFromContext` — with ID, without ID, wrong type (100%)
  - [x] Round-trip integration, uniqueness, context key type safety
  - 14 tests total

- [x] **`services/password_validator.go`** (28% → 98%) — **Done 2026-02-08**
  - [x] `NewPasswordValidator` — constructor, commonPasswords populated (100%)
  - [x] `ValidatePassword` — valid, too short, too long, common, breached, API error warning, multiple errors, boundary lengths (100%)
  - [x] `IsBreached` — found, not found, case-insensitive suffix, API error, network error, empty response, request headers, k-anonymity prefix (92%)
  - [x] `IsCommonPassword` — known common, case-insensitive, not common (100%)
  - [x] `CalculatePasswordStrength` — empty, short, 12/16/20 char thresholds, all char types, entropy ratio, cap at 100 (97%)
  - [x] `GetStrengthLabel` — all 5 boundary ranges (100%)
  - 31 tests total (all unit, mock HTTP transport for HIBP API)

- [x] **`services/user.go`** (1,231 lines, 33 functions) — **Done 2026-02-08** — 24% → 83.5% avg
  - [x] `HashPassword` / `VerifyPassword` — bcrypt hashing, correct/wrong password (100%)
  - [x] `IsAccountLocked` — nil, locked (future), expired (past) (100%)
  - [x] `GetLockTimeRemaining` — nil, future, expired (100%)
  - [x] `IsAccountRecoverable` — nil, active, within grace, expired (100%)
  - [x] `GetDaysUntilPermanentDeletion` — nil user, active, deleted 5/31 days, nil DeletedAt (100%)
  - [x] `CreateUserWithPassword` — success, duplicate email, preferences created, password hashed (66.7%)
  - [x] `AuthenticateUserWithPassword` — success, wrong password, nonexistent, OAuth-only, locked, inactive (95.8%)
  - [x] `IncrementFailedAttempts` / `ResetFailedAttempts` — below threshold, at threshold (lock), reset (83-84%)
  - [x] `UpdatePassword` — success, wrong current password (72.2%)
  - [x] `SetEmailVerified` — success (75%)
  - [x] `SoftDeleteAccount` — success, with reason, not found (91.7%)
  - [x] `RestoreAccount` — success, not found (87.5%)
  - [x] `GetExpiredDeletedAccounts` — expired vs recent filtering (87.5%)
  - [x] `GetDeletionSummary` — shows, saved shows, passkeys counts (70%)
  - [x] `PermanentlyDeleteUser` — cascaded deletion, show submitted_by nullified (75%)
  - [x] `CreateUserWithoutPassword` — success, duplicate email (66.7%)
  - [x] `ExportUserData` / `ExportUserDataJSON` — full export with OAuth/shows/saved/passkeys, valid JSON (75-79%)
  - [x] `GetOAuthAccounts` — success with multiple providers (80%)
  - [x] `CanUnlinkOAuthAccount` — has password (can unlink), only auth (cannot) (86.7%)
  - [x] `UnlinkOAuthAccount` — success (66.7%)
  - [x] `ListUsers` — success, search filter, auth methods (73.9%)
  - [x] `GetUserByEmailIncludingDeleted` — active, deleted, not found (88.9%)
  - [x] Nil-db unit tests for all 23 methods (incl. IncrementFailedAttempts, ResetFailedAttempts, UpdatePassword, SetEmailVerified)
  - ~90 tests total: 35 unit (nil-db + pure logic) + 55 integration (testcontainers)

### Tier 3: Admin Operations (data integrity)

Admin actions modify data without user review. Bugs here silently corrupt the database.

- [x] **`handlers/admin.go`** (2,062 lines, 29 functions) — **Updated 2026-02-10**
  - [x] Admin guard: all 26 handlers tested for NoUser (403) + NonAdmin (403) = 52 sub-tests
  - [x] ApproveShow/RejectShow/VerifyVenue/ApproveVenueEdit/RejectVenueEdit InvalidID → 400
  - [x] RejectShow/RejectVenueEdit EmptyReason → 400
  - [x] ImportShowPreview/Confirm InvalidBase64 → 400
  - [x] BulkExportShows EmptyIDs + TooMany(>50) → 400
  - [x] BulkImportPreview/Confirm EmptyShows + TooMany(>50) → 400
  - [x] DiscoveryImport EmptyEvents + TooMany(>100) → 400
  - [x] DiscoveryCheck EmptyEvents + TooMany(>200) → 400
  - [x] CreateAPIToken ExpirationTooLong(>365) → 400
  - [x] RevokeAPIToken InvalidID → 400
  - [x] DataImport EmptyItems + TooMany(>500) → 400
  - [x] ExportShows InvalidDate → 400
  - [x] **Integration tests (34 new)**: GetPendingShows (empty, success), ApproveShow (success, not found, already approved, with verify venues), RejectShow (success, not found, empty reason), GetRejectedShows, VerifyVenue (success, not found), GetUnverifiedVenues (empty, success), GetPendingVenueEdits (empty, success), ApproveVenueEdit (success, not found), RejectVenueEdit (success), GetAdminShows (success, status filter), GetAdminStats, GetAdminUsers (success, pagination), CreateAPIToken (success, default expiration, exceeded max), ListAPITokens, RevokeAPIToken (success, not found), ExportShows/Artists/Venues
  - 111 tests total (77 unit + 34 integration)

- [x] **`services/audit_log.go`** (157 lines, 4 functions) — **Done 2026-02-08**
  - [x] `LogAction` — fire-and-forget logging, with/without metadata, nil metadata (83%)
  - [x] `GetAuditLogs` — retrieval with pagination, filters (entity_type, action, actor_id, combined) (90%)
  - [x] `buildResponse` — actor email preload, metadata deserialization (100%)
  - [x] All 8 instrumented actions verified (approve_show, reject_show, verify_venue, approve/reject_venue_edit, dismiss/resolve/resolve_with_flag report)
  - [x] Nil-db unit tests for LogAction (no-panic) and GetAuditLogs
  - 16 tests total: 3 unit + 13 integration (testcontainers)

- [x] **`services/show_report.go`** (319 lines, 8 functions) — **Done 2026-02-08**
  - [x] `CreateReport` — success, all 3 report types, invalid type, show not found, duplicate blocked, different users OK (83%)
  - [x] `GetUserReportForShow` — found + not found (returns nil, nil) (89%)
  - [x] `GetPendingReports` — with show info, excludes reviewed, pagination (85%)
  - [x] `DismissReport` — success with notes, not found, already reviewed blocked (89%)
  - [x] `ResolveReport` / `ResolveReportWithFlag` — resolve with/without flag, cancelled sets is_cancelled, sold_out sets is_sold_out, inaccurate sets no flag, flag=false skips update (84-100%)
  - [x] `GetReportByID` — success with show preload, not found (88%)
  - [x] `buildReportResponse` — show info, ReviewedAt RFC3339 formatting (90%)
  - [x] Nil-db unit tests for all 7 public methods
  - 34 tests total: 9 unit (nil-db) + 25 integration (testcontainers)

### Tier 4: User-Facing Features

These affect individual users but have lower blast radius than core/admin operations.

- [x] **`handlers/show.go`** (1,574 lines, 19 functions) — **Updated 2026-02-10**
  - [x] Constructor, CreateShow_UnverifiedEmail (403)
  - [x] Update/Delete/Unpublish/MakePrivate/Publish NoAuth (401) + InvalidID (400)
  - [x] GetMySubmissions NoAuth (401)
  - [x] SetShowSoldOut/Cancelled NoAuth (401) + InvalidID (400)
  - [x] ExportShow NonDevEnvironment (404)
  - [x] **Integration tests (27 new)**: CreateShow (success, admin auto-approve, unverified blocked), GetShow (by ID, not found, pending submitter/other user), GetShows (success, empty), GetUpcomingShows (success, excludes past, empty), UpdateShow (owner, admin, non-owner, not found), DeleteShow (owner, non-owner, not found), GetMySubmissions (success, empty), GetShowCities, UnpublishShow (owner, not found), SetShowSoldOut (owner, non-owner), SetShowCancelled (owner)
  - 45 tests total (18 unit + 27 integration)

- [x] **`services/saved_show.go`** (316 lines, 7 functions) — **Done 2026-02-08**
  - [x] `SaveShow` — success, show not found, idempotent via FirstOrCreate (83%)
  - [x] `UnsaveShow` — success, not saved returns no error (88%)
  - [x] `GetUserSavedShows` — with ordering, empty, includes venue+artist, pagination, only own shows (91%)
  - [x] `buildShowResponse` — model-to-response conversion (82%)
  - [x] `IsShowSaved` — boolean check (86%)
  - [x] `GetSavedShowIDs` — batch check, empty input, none matched (92%)
  - [x] Nil-db unit tests for all 6 public methods + constructor
  - 22 tests total: 7 unit (nil-db) + 15 integration (testcontainers)

- [x] **`services/favorite_venue.go`** (484 lines, 8 functions) — **Done 2026-02-08**
  - [x] `FavoriteVenue` — success, venue not found, idempotent (83%)
  - [x] `UnfavoriteVenue` — success, not favorited (88%)
  - [x] `GetUserFavoriteVenues` — with ordering, empty, with upcoming show count, pagination, only own (88%)
  - [x] `IsVenueFavorited` — boolean check (86%)
  - [x] `GetUpcomingShowsFromFavorites` — success with venue info, artists, no favorites, excludes non-approved, multiple venues, pagination (88%)
  - [x] `getOrderedArtistsForShows` — artist batch loading (89%)
  - [x] `GetFavoriteVenueIDs` — batch check, empty input, none matched (92%)
  - [x] Nil-db unit tests for all 7 public methods + constructor
  - 30 tests total: 8 unit (nil-db) + 22 integration (testcontainers)

- [x] **`services/show_report.go`** — **Done 2026-02-08** (see Tier 3 above)

- [x] **`handlers/venue.go`** (683 lines, 10 functions) — **Updated 2026-02-10**
  - [x] Constructor, UpdateVenue/GetMyPendingEdit/CancelMyPendingEdit/DeleteVenue NoAuth (401) + InvalidID (400)
  - [x] **Integration tests (19 new)**: SearchVenues (success, no results), ListVenues (success, empty, city filter), GetVenue (by ID, not found), GetVenueShows, GetVenueCities, UpdateVenue (admin direct, non-admin pending edit, non-owner forbidden, not found), DeleteVenue (admin, owner, non-owner, not found), GetMyPendingEdit (none, exists)
  - 28 tests total (9 unit + 19 integration)

- [x] **`handlers/saved_show.go`** (5 handlers) — **Updated 2026-02-18**
  - [x] Constructor, SaveShow/UnsaveShow NoAuth (401) + InvalidID (400)
  - [x] GetSavedShows NoAuth (401), CheckSaved NoAuth (401) + InvalidID (400)
  - [x] **Mock-based unit tests (14 new)**: SaveShow (Success, ServiceError), UnsaveShow (Success, ServiceError), GetSavedShows (Success, ServiceError, PaginationClamping), CheckSaved (Saved, NotSaved, ServiceError), CheckBatchSaved (NoAuth, EmptyList, Success, NegativeID, ServiceError)
  - [x] **Integration tests (10)**: SaveShow (success, already saved idempotent, not found), UnsaveShow (success, not saved), GetSavedShows (empty, with shows, pagination), CheckSaved (true, false)
  - 33 tests total (23 unit + 10 integration)

- [x] **`handlers/favorite_venue.go`** (5 handlers) — **Updated 2026-02-18**
  - [x] Constructor, FavoriteVenue/UnfavoriteVenue NoAuth (401) + InvalidID (400)
  - [x] GetFavoriteVenues/GetFavoriteVenueShows NoAuth (401), CheckFavorited NoAuth (401) + InvalidID (400)
  - [x] **Mock-based unit tests (12 new)**: FavoriteVenue (Success, ServiceError), UnfavoriteVenue (Success, ServiceError), GetFavoriteVenues (Success, ServiceError, PaginationClamping), CheckFavorited (True, False, ServiceError), GetFavoriteVenueShows (Success, ServiceError, DefaultTimezone)
  - [x] **Integration tests (12)**: FavoriteVenue (success, already favorited idempotent, not found), UnfavoriteVenue (success, not favorited), GetFavoriteVenues (empty, with venues, pagination), CheckFavorited (true, false), GetFavoriteVenueShows (empty, with shows)
  - 34 tests total (22 unit + 12 integration)

- [x] **`handlers/show_report.go`** (5 handlers) — **Updated 2026-02-18**
  - [x] Constructor, ReportShow/GetMyReport NoAuth (401) + InvalidID (400)
  - [x] GetPendingReports/DismissReport/ResolveReport NoAuth+NonAdmin (403) + InvalidID (400)
  - [x] **Mock-based unit tests (11 new)**: ReportShow (Success, ServiceError), GetMyReport (Success, NoReport, ServiceError), GetPendingReports (Success, ServiceError), DismissReport (Success, ServiceError), ResolveReport (Success, WithFlag, ServiceError)
  - [x] **Integration tests (11)**: ReportShow (success, with details, already reported), GetMyReport (exists, not exists), GetPendingReports (empty, with reports), DismissReport (success, not found), ResolveReport (success, with show flag, not found)
  - 36 tests total (25 unit + 11 integration)

- [x] **`handlers/audit_log.go`** (1 handler) — **Done 2026-02-18**
  - [x] GetAuditLogs NoAuth (403) + NonAdmin (403)
  - [x] **Mock-based unit tests (5 new)**: Success, ServiceError, LimitClamping, FiltersPassedThrough, Constructor
  - 7 tests total (all unit, mock-based)

- [x] **`handlers/artist_report.go`** (5 handlers) — **Done 2026-02-18**
  - [x] ReportArtist/GetMyArtistReport NoAuth (401) + InvalidID (400)
  - [x] GetPendingArtistReports/DismissArtistReport/ResolveArtistReport NoAuth+NonAdmin (403) + InvalidID (400)
  - [x] **Mock-based unit tests (11 new)**: ReportArtist (Success, ServiceError), GetMyArtistReport (Success, NoReport, ServiceError), GetPendingArtistReports (Success, ServiceError), DismissArtistReport (Success, ServiceError), ResolveArtistReport (Success, ServiceError)
  - 24 tests total (all unit, 13 auth guard + 11 mock-based)

- [x] **`handlers/oauth_account.go`** (2 handlers) — **Done 2026-02-08**
  - [x] Constructor, GetOAuthAccounts NoAuth (401), UnlinkOAuth NoAuth (401) + InvalidProvider (400)
  - 4 tests total

- [x] **`handlers/apple_auth.go`** (1 handler) — **Done 2026-02-08**
  - [x] Constructor, AppleCallback EmptyToken → success=false, VALIDATION_FAILED
  - 2 tests total

### Tier 5: Supporting Services

Lower priority but still worth covering for production confidence.

- [x] **`services/data_sync.go`** (877 lines, 9 functions) — **Done 2026-02-08**
  - [x] `ExportShows` — empty, default/max limit, status filters (approved/pending/all), date filter, location filter, with artists+venues, pagination (87.8%)
  - [x] `ExportArtists` — empty, default limit, search, pagination, with social (84.2%)
  - [x] `ExportVenues` — empty, default limit, search, verified filter, city filter, with social (84%)
  - [x] `ImportData` — empty request, all counters correct (93.9%)
  - [x] `importArtist` — success with slug, duplicate, duplicate backfill slug, empty name, dry run (92.3%)
  - [x] `importVenue` — success with slug, duplicate, missing fields, dry run (69.2%)
  - [x] `importShow` — success creates venue+artist, duplicate, duplicate backfill slugs, missing fields, invalid date, status parsing, dry run (72.5%)
  - [x] `backfillShowSlugs` — exercised via duplicate show import tests (93.9%)
  - [x] Full export→import round-trip test
  - [x] Nil-db unit tests for all 4 public methods
  - 42 tests total: 6 unit (nil-db + constructor) + 36 integration (testcontainers)

- [x] **`services/admin_stats.go`** (93 lines, 2 functions) — **Done 2026-02-08**
  - [x] `GetDashboardStats` — empty DB, pending shows/venue edits/reports, unverified venues, total counts, total users, recent activity (7-day window), full scenario (56.5%)
  - [x] `NewAdminStatsService` — constructor with nil/explicit DB (100%)
  - [x] Nil-db panics test
  - 12 tests total: 3 unit + 9 integration (testcontainers)

- [x] **`services/discovery.go`** (615 lines, 11 functions) — **Done 2026-02-08**
  - [x] `parseEventDate` — ISO date, timestamp, RFC3339, AM/PM, 12:00 noon/midnight, whitespace, invalid, unparseable time (100%)
  - [x] `parseArtistsFromTitle` — comma, "with", slash, pipe, plus, ampersand (short/long names), empty, whitespace (100%)
  - [x] `splitAndTrim` — basic, empty parts, whitespace-only, no separator (100%)
  - [x] `ImportEvents` — success with slug, source duplicate, unknown venue, headliner duplicate (pending_review), dry run (100%)
  - [x] `CheckEvents` — found, not found, empty input (89.5%)
  - [x] `checkHeadlinerDuplicate` — exercised via headliner duplicate test (100%)
  - [x] `importEvent` — all paths via ImportEvents (85.3%)
  - [x] `createShowFromEvent` — show + venue + artist creation, slug generation (78.8%)
  - [x] Rejected show skip test (existing rejected show blocks reimport)
  - [ ] `ImportFromJSON` / `ImportFromJSONWithDB` — file-based import (0%, tested via ImportEvents instead)
  - 40 tests total: 31 unit (pure functions) + 9 integration (testcontainers)

- [x] **`services/api_token.go`** (263 lines, 9 functions) — **Done 2026-02-08**
  - [x] `generateToken` — format (phk_ prefix, 68 chars), uniqueness (75%)
  - [x] `hashToken` — deterministic SHA-256, different inputs → different hashes (100%)
  - [x] `CreateToken` — success, custom expiration (30 days), zero expiration fallback to 90 days (83.3%)
  - [x] `ValidateToken` — success, invalid token, expired, revoked, inactive user, non-admin user (90.9%)
  - [x] `ListTokens` — success (2 tokens, DESC order), excludes revoked, empty (90%)
  - [x] `RevokeToken` — success, not found, already revoked, wrong user (88.9%)
  - [x] `GetToken` — success, not found, wrong user (88.9%)
  - [x] `CleanupExpiredTokens` — removes 31+ day old tokens, keeps recently expired (85.7%)
  - [x] Nil-db unit tests for all 7 public methods
  - 32 tests total: 11 unit (nil-db + pure functions) + 21 integration (testcontainers)
- [x] **`services/discord.go`** (432 lines, 15 functions) — **Done 2026-02-09**
  - [x] `NewDiscordService` — configured, not configured, default HTTP client (100%)
  - [x] `IsConfigured` — enabled/disabled, empty URL (100%)
  - [x] `hashEmail` — normal, short, single char, empty, no at sign (100%)
  - [x] `buildUserName` — full name, first only, last only, neither, nil user, empty strings (100%)
  - [x] `buildVenueList` — no venues, one, multiple (100%)
  - [x] `buildArtistList` — no artists, one, headliner, mixed (100%)
  - [x] `sendWebhook` — success, server error, invalid URL, payload structure (89.5%)
  - [x] `NotifyNewUser` — success, not configured, nil user, no name (100%)
  - [x] `NotifyNewShow` — success with action links, not configured, nil show (100%)
  - [x] `NotifyShowApproved` — success, not configured, nil show (100%)
  - [x] `NotifyShowRejected` — success with reason, not configured, nil show (100%)
  - [x] `NotifyShowStatusChange` — success, not configured, non-pending no actions (100%)
  - [x] `NotifyShowReport` — success, not configured, nil report, long details truncation (95.5%)
  - [x] `NotifyNewVenue` — success with address, not configured, no address, city only (100%)
  - [x] `NotifyPendingVenueEdit` — success, not configured (100%)
  - 40 tests total (pure unit tests with httptest mock)
- [x] **`services/email.go`** (5 functions) — **Done 2026-02-09**
  - [x] `NewEmailService` — configured, not configured (100%)
  - [x] `IsConfigured` — nil client, empty from, both set (100%)
  - [x] `SendVerificationEmail` — success, not configured, API error (100%)
  - [x] `SendMagicLinkEmail` — success, not configured, API error (100%)
  - [x] `SendAccountRecoveryEmail` — success with days remaining, not configured, API error (100%)
  - 14 tests total (httptest mock Resend API)
- [x] **`services/webauthn.go`** (466 lines, 23 functions) — **Done 2026-02-08**
  - [x] `NewWebAuthnService` — success, default RPID, default origins (84.6%)
  - [x] `BeginRegistration` — success, with exclusions from existing creds (80%)
  - [x] `BeginLogin` — no credentials error, with credentials success (75%)
  - [x] `BeginDiscoverableLogin` — success (75%)
  - [x] `BeginRegistrationForEmail` — success (80%)
  - [x] `GetUserCredentials` — empty, multiple (ordered DESC), wrong user (75%)
  - [x] `DeleteCredential` — success, not found, wrong user (83.3%)
  - [x] `UpdateCredentialName` — success, not found, wrong user (83.3%)
  - [x] `StoreChallenge` — success (71.4%)
  - [x] `GetChallenge` — success, not found, wrong operation, expired (90%)
  - [x] `DeleteChallenge` — success (100%)
  - [x] `CleanupExpiredChallenges` — expired removed, active kept (100%)
  - [x] `StoreChallengeWithEmail` / `GetChallengeWithEmail` — success, expired, not found, round-trip (75-90%)
  - [x] Nil-DB panic tests for GetUserCredentials, StoreChallenge, DeleteCredential
  - [ ] `FinishRegistration`, `FinishLogin`, `FinishDiscoverableLogin`, `FinishSignupRegistration` — 0% (need real CBOR/attestation data)
  - 34 tests total: 6 unit + 28 integration (testcontainers)

- [x] **`models/user_webauthn.go`** (147 lines, 9 functions) — **Done 2026-02-08**
  - [x] `WebAuthnID` — correct 8-byte big-endian encoding, zero, large value (100%)
  - [x] `WebAuthnName` — email, username, both (prefers email), neither, empty strings (100%)
  - [x] `WebAuthnDisplayName` — full name, first only, empty first falls back, no name falls back to email (100%)
  - [x] `WebAuthnCredentials` — empty, multiple with all fields converted (100%)
  - [x] `WebAuthnIcon` — with/without avatar (100%)
  - [x] `ToWebAuthnCredential` — all fields, with/without/single transports (100%)
  - [x] `GetTransports` — empty, empty array, all 5 transport types, multiple, round-trip (100%)
  - [x] `contains` / `containsSubstring` — tested indirectly via GetTransports (100%)
  - 32 tests total (all pure unit, no DB)

- [x] **`services/cleanup.go`** (6 functions) — **Done 2026-02-08**
  - [x] `NewCleanupService` — constructor, env var override, invalid/zero/negative env ignored (100%)
  - [x] `Start` / `Stop` — lifecycle, graceful shutdown (100%)
  - [x] `run` — context cancellation exits loop (91%)
  - [x] `RunCleanupNow` — nil DB doesn't panic (100%)
  - [x] `runCleanupCycle` — exercises error path (21% — success path needs testcontainers)
  - 8 tests total (all unit, no DB)

- [x] **`utils/slug.go`** (5 functions) — **Done 2026-02-08**
  - [x] `GenerateSlug` — basic, special chars, multiple hyphens, leading/trailing, empty, numbers, unicode, mixed case (100%)
  - [x] `GenerateArtistSlug` — delegates to GenerateSlug (100%)
  - [x] `GenerateVenueSlug` — name+city+state (100%)
  - [x] `GenerateShowSlug` — date formatting, "at" separator (100%)
  - [x] `GenerateUniqueSlug` — no collision, single collision, multiple collisions, fallback to timestamp (100%)
  - 21 tests total (all unit), **100% coverage**

### Tier 6: Quick Wins (small effort, complete coverage)

- [x] **`services/jwt.go`** — **Done 2026-02-08** — finished remaining 6/11 functions
  - [x] `CreateVerificationToken` / `ValidateVerificationToken` — create+validate, expired, wrong subject, wrong secret (100% / 83%)
  - [x] `CreateMagicLinkToken` / `ValidateMagicLinkToken` — create+validate, expired, wrong subject (100% / 83%)
  - [x] `CreateAccountRecoveryToken` / `ValidateAccountRecoveryToken` — create+validate, expired, wrong subject, wrong secret (100% / 83%)
  - 14 new tests (added to existing jwt_test.go + jwt_lenient_test.go)

- [x] **`services/apple_auth.go`** — **Done 2026-02-08** — added constructor + GenerateToken
  - [x] `NewAppleAuthService` — constructor fields initialized (100%)
  - [x] `GenerateToken` — delegates to JWTService, verifies claims (100%)
  - 2 new tests (added to existing apple_auth_test.go)
  - Remaining 0%: `FindOrCreateAppleUser`, `linkAppleAccount`, `createAppleUser` (need testcontainers)

- [x] **`services/extraction.go`** — **Done 2026-02-08** — added constructor
  - [x] `NewExtractionService` — constructor fields initialized (100%)
  - 1 new test (added to existing extraction_test.go)
  - Remaining 0%: `matchArtists`, `matchVenue` (need testcontainers); `callAnthropic` (hardcoded URL)

- [x] **`config/config.go`** (16% → 100%) — **Done 2026-02-08**
  - [x] `Load` — development defaults, custom env vars, validation failure (100%)
  - [x] `getWebAuthnOrigins` — explicit env, production default, dev default (100%)
  - [x] `GetEnv`, `getEnvAsBool`, `GetSameSite`, `NewAuthCookie`, `ClearAuthCookie`, `Validate`, `getFrontendURL`, `getCORSOrigins`, `getWebAuthnRPID` — all 100%
  - All 12 functions at 100% coverage

---

## Remaining Coverage Gaps (Next Steps)

### ~~Middleware (36.7% → 92.7%)~~ — **Done 2026-02-09, integration tests added 2026-02-19**
- [x] **`middleware/request_id.go`** — 3 functions at 100%: `RequestIDMiddleware`, `HumaRequestIDMiddleware`, `GetRequestIDFromContext` (14 tests)
- [x] **`middleware/jwt.go` Huma variants** — 4 functions at 87-100%: `HumaJWTMiddleware` (88.7%), `LenientHumaJWTMiddleware` (89.7%), `OptionalHumaJWTMiddleware` (87.5%), `writeHumaJWTError` (100%) (19 unit tests)
  - Used `humatest.NewContext()` from `github.com/danielgtaylor/huma/v2/humatest` for test Huma contexts
- [x] **`middleware/jwt_integration_test.go`** — 10 integration tests with testcontainers (real PostgreSQL) — **Done 2026-02-19**
  - Full happy path: JWT parsed → user loaded from DB → user set in context → `next()` called
  - `HumaJWTMiddleware` (4): valid bearer, valid cookie, inactive user rejected, deleted user rejected
  - `LenientHumaJWTMiddleware` (3): valid non-expired, expired within grace period, inactive user within grace rejected
  - `OptionalHumaJWTMiddleware` (2): valid token (user in context), inactive user (proceeds without user)
  - `JWTMiddleware` Chi (1): valid bearer through http.Handler chain, `GetUserFromContext` verified
  - Uses `JWTMiddlewareIntegrationSuite` (testify/suite), `testing.Short()` skip, same migration list as handler integration tests + migration 29

### Services (76.0% — remaining gaps)
- [x] **`services/discord.go`** (432 lines, 15 functions, 0% → 98.6%) — **Done 2026-02-09** — 40 tests (httptest mock)
- [x] **`services/email.go`** (5 functions, 0% → 100%) — **Done 2026-02-09** — 14 tests (httptest mock Resend API)
- [x] **`services/show.go` remaining** — `ExportShowToMarkdown` (73.9%), `ParseShowMarkdown` (96.3%), `GetAdminShows` (64.1%) — **Done 2026-02-09** — 18 tests added
- [ ] **`services/extraction.go` remaining** — `matchArtists`/`matchVenue` (need testcontainers), `callAnthropic` (hardcoded URL)
- [ ] **`services/apple_auth.go` remaining** — `FindOrCreateAppleUser`/`linkAppleAccount`/`createAppleUser` (need testcontainers)
- [ ] **`services/discovery.go` remaining** — `ImportFromJSON`/`ImportFromJSONWithDB` (file-based import, 0%)

### ~~Handlers (26.2% → 54.1% → ~60%+)~~ — **Updated 2026-02-18**
- [x] **Handler mock-based unit tests (68 new)**: Hand-written mocks in `handler_unit_mock_helpers_test.go`, covering service interaction paths for 5 handlers (SavedShow, FavoriteVenue, AuditLog, ShowReport, ArtistReport). Remaining: Admin, Auth, Show handlers.
- [x] **Handler integration tests**: 149 tests across 8 suites using testcontainers (real DB)
  - `admin_integration_test.go`: 34 tests — show approval/rejection, venue verification, venue edits, admin shows/users/stats, API tokens, data export
  - `show_integration_test.go`: 27 tests — CRUD, upcoming shows, submissions, unpublish, sold out/cancelled
  - `artist_integration_test.go`: 24 tests — search, list, get (ID/slug), get shows, delete (success/has-shows/not-found), admin update, bandcamp/spotify URL management
  - `venue_integration_test.go`: 19 tests — search, list, CRUD, pending edits, delete
  - `oauth_handlers_integration_test.go`: 12 tests — callback error/success paths, CLI callback flow, frontend URL config, expired callback, existing user
  - `favorite_venue_integration_test.go`: 12 tests — favorite/unfavorite, list, check, venue shows
  - `show_report_integration_test.go`: 11 tests — report, dismiss, resolve with flags
  - `saved_show_integration_test.go`: 10 tests — save/unsave, list, check
  - Shared infrastructure in `handler_integration_helpers_test.go` (includes `artistService`)
- [x] **`handlers/artist.go`** — **Done 2026-02-12** — 0% → ~88% avg across 13 functions
  - `artist_test.go`: 20 unit tests — constructor, delete auth/validation, admin update auth/validation, bandcamp/spotify auth/validation, helper functions (isValidBandcampURL, isValidSpotifyURL, nilIfEmpty)
  - `artist_integration_test.go`: 24 integration tests — search (success, no results), list (success, empty, city filter), get (by ID, by slug, not found), get shows (success, empty, by slug, not found), delete (success, has shows 409, not found), admin update (success, social links, not found), bandcamp (admin success, clear URL, not found), spotify (admin success, clear URL, not found)
  - 44 tests total (20 unit + 24 integration)
- [x] **`handlers/oauth_handlers.go`** — **Done 2026-02-12** — 42% → 100% (all 7 functions)
  - `oauth_handlers_test.go`: 11 unit tests — generateRandomID, CLI callback store CRUD (store/retrieve, not found, expired, delete, cleanup), constructor, login handler validation (no provider, invalid provider), CLI callback stored with cookie, valid provider no-panic
  - `oauth_handlers_integration_test.go`: 12 integration tests — callback error redirect (frontend, CLI, cookie cleared, memory deleted), no-provider fallback, custom/empty frontend URL, success web flow (cookie + redirect), success CLI flow (token in redirect), GitHub provider, existing user re-auth, expired CLI callback fallback
  - Key approach: mock `OAuthCompleter` interface via `AuthService.SetOAuthCompleter()` to bypass gothic/Goth session requirements while testing real handler code
  - 23 tests total (11 unit + 12 integration)

---

## Testing Approach Notes

- **Services with DB dependencies**: Use testcontainers (PostgreSQL) like `user_test.go`. Run all relevant migrations in `SetupSuite`.
- **Handlers (unit)**: Direct function calls with nil services for pre-service-call validation paths.
- **Handlers (integration)**: Use testcontainers with real services via `handler_integration_helpers_test.go`. Shared `setupHandlerIntegrationDeps()` creates all services. Each suite uses `TearDownTest()` to clean tables between tests. Key gotchas: verified venues auto-approve shows, FavoriteVenue is idempotent (FirstOrCreate), UnpublishShow sets "private" not "pending", VenueEditStatus is `models.VenueEditStatus` type (not plain string).
- **Middleware**: Use httptest with minimal handler chains.
- **External services** (Discord, email, AI extraction): Use interface mocks.
- **Testcontainers image**: Use `postgres:18` to match production.
- **Integration test migrations**: When adding new integration test suites, include all migrations that affect the tables being tested.
  - `user_test.go`: 000001, 000005, 000006, 000011, 000012, 000014, 000015
  - `show_test.go`: 000001, 000004, 000005, 000007, 000008, 000009, 000010, 000012, 000013, 000014, 000020, 000023, 000026, 000027 (with `CONCURRENTLY` stripped), 000028
  - `venue_test.go`: same as show_test.go + 000002 (pg_trgm extension), 000003 (venue search indexes)
  - `artist_test.go`: same as venue_test.go
  - `show_report_test.go`: same as venue_test.go + 000018 (show_reports)
  - `audit_log_test.go`: 000001, 000012, 000014, 000022 (audit_logs) — minimal set
  - `saved_show_test.go`: same as venue_test.go + 000006 (user_saved_shows)
  - `favorite_venue_test.go`: same as venue_test.go + 000015 (user_favorite_venues)
  - `api_token_test.go`: 000001, 000012, 000014, 000021 (api_tokens) — minimal set
  - `discovery_test.go`: same as show_test.go (full schema needed for venue/artist/show associations)
  - `admin_stats_test.go`: same as show_test.go + 000018 (show_reports)
  - `webauthn_test.go`: 000001, 000005, 000011, 000012, 000014 — minimal set for users + webauthn tables
  - `data_sync_test.go`: same as show_test.go (full schema needed for export/import)
  - `handler_integration_helpers_test.go`: 000001, 000002, 000003, 000004, 000005, 000006, 000007, 000008, 000009, 000010, 000012, 000013, 000014, 000015, 000018, 000020, 000021, 000022, 000023, 000026, 000027 (stripped), 000028 — full schema for all handler tests
  - `jwt_integration_test.go` (middleware): same as handler_integration_helpers + 000029, 000030, 000031 — full schema (only users table needed, but keeps migration list in sync)
- **Migration 000027 quirk**: Uses `CREATE INDEX CONCURRENTLY` which cannot run inside a transaction. Test setup strips the `CONCURRENTLY` keyword before executing.

## Production Readiness Audit Items

Items to verify while adding test coverage:

- [x] All admin endpoints require admin role check — **Verified 2026-02-08**: 26 handlers × 2 scenarios tested
- [ ] All user-scoped endpoints verify ownership (saved shows, favorites, reports)
- [ ] Unapproved/rejected shows are not returned by public endpoints
- [ ] Soft-deleted users/shows/venues are excluded from queries
- [ ] Rate limiting is applied to all auth endpoints
- [ ] Security headers are set on all responses
- [ ] JWT tokens include correct claims and expiry
- [ ] Account lockout triggers after configured failed attempts
- [ ] Magic links and verification tokens expire
- [ ] Audit log captures all admin actions
- [ ] Slug generation produces URL-safe, unique values
- [ ] Pagination has reasonable max limits to prevent abuse

---

## Defects Found During Testing

### FIXED: Cursor pagination in `GetUpcomingShows` repeated boundary show

**File:** `services/show.go` — `GetUpcomingShows`, `encodeCursor`, `decodeCursor`
**Severity:** Low (cosmetic duplication in paginated results, no data loss)
**Found:** 2026-02-08 during integration testing
**Fixed:** 2026-02-08 via migration 000028

**Symptom:** The last show on page N appeared again as the first show on page N+1.

**Root cause:** The `event_date` column was `TIMESTAMP` (without timezone), but pgx sends Go `time.Time` query parameters as `TIMESTAMPTZ`. The equality check `event_date = ?` in the cursor query failed because Postgres was comparing a timezone-naive column against a timezone-aware parameter, causing the boundary show to leak into the next page.

**Fix:** Migration 000028 changes `event_date` from `TIMESTAMP` to `TIMESTAMPTZ USING event_date AT TIME ZONE 'UTC'`. This preserves existing data (interpreting stored values as UTC) and ensures exact comparisons between the column and Go `time.Time` parameters.
