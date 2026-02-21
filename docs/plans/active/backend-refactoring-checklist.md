# Go Backend Refactoring Checklist

> **Purpose**: This document provides a phased implementation plan for refactoring the Go backend to improve testability, maintainability, and code quality while avoiding over-engineering.

## Background & Analysis

### Current State Assessment (January 2026)

**Architecture Overview:**
- **Framework**: Huma v2 (OpenAPI-first) + Chi v5 router
- **Database**: GORM with PostgreSQL
- **Logging**: Go's `slog` package (structured logging)
- **Auth**: JWT, OAuth (Goth), WebAuthn/Passkeys, Magic Links
- **Error Tracking**: Sentry

**Project Structure:**
```
/backend
├── /cmd/server          # Entry point
├── /internal
│   ├── /api/handlers    # HTTP request handlers
│   ├── /api/middleware  # Auth, CORS, security
│   ├── /api/routes      # Route registration
│   ├── /services        # Business logic (14 services)
│   ├── /models          # GORM database models
│   ├── /errors          # Typed error handling
│   ├── /auth            # OAuth setup (Goth)
│   ├── /config          # Configuration
│   ├── /logger          # Structured logging
│   └── /utils           # Utilities
├── /db/migrations       # Database migrations
└── /pkg                 # Public packages
```

### What's Already Good

- [x] Typed custom errors with context in `internal/errors/` (show, venue, artist domains fully wired)
- [x] Comprehensive authentication (JWT, OAuth, Passkeys, Magic Links)
- [x] Request ID correlation in logging
- [x] Security headers middleware (HSTS, CSP, X-Frame-Options)
- [x] Rate limiting on auth endpoints
- [x] Transaction usage for multi-step DB operations
- [x] Cookie configuration centralized in `config.SessionConfig`

### Issues Identified

1. **Global Database Singleton** - Services access DB via `db.GetDB()` global function
   - Makes unit testing difficult
   - Tight coupling between services and database package

2. ~~**String-Based Error Checking** in handlers~~ — **FIXED (P0-1 + P0-1b)**
   - All handlers and middleware now use `errors.As()` with typed errors
   - Zero remaining string-based error checks

3. **N+1 Artist Loading Queries** in secondary services (venue shows, saved shows, favorite venue shows)
   - Each artist loaded individually in a loop instead of batch `WHERE id IN (?)`
   - See P0-1c for details

4. **Manual Service Instantiation** - Routes file manages all dependencies
   - No clear lifecycle management
   - Scattered initialization

5. **Low Test Coverage** - Only 6 test files for 52 source files (11.5%)
   - No middleware tests
   - No model tests
   - Missing service tests (only 3 of 15 services tested)

6. **log.Fatal in config validation** — Crashes process instead of returning error (see P0-1d)

---

## Web Research: Go Best Practices (2025-2026)

### Dependency Injection Approaches

| Approach | Best For | Pros | Cons |
|----------|----------|------|------|
| **Manual DI** | Small-medium apps | Most readable, no dependencies, compile-time safety | More boilerplate for large apps |
| **Google Wire** | Large apps with static graphs | Compile-time code generation, zero runtime overhead | Requires extra tooling, generated code noise |
| **Uber Fx** | Large microservices | Runtime DI, lifecycle management | Steep learning curve, overkill for small apps |

**Recommendation for this codebase**: Manual DI with a simple container. Wire and Fx are overkill for a medium-sized application.

### Structured Logging (slog)

- `slog` is now the Go standard (Go 1.21+) - the codebase already uses it correctly
- Best practice: Include request IDs and user IDs in context for correlation
- Best practice: Pass context to service methods that need logging

### Error Handling

- Use sentinel errors or typed errors with `errors.Is()`/`errors.As()`
- Wrap errors with context using `fmt.Errorf("...: %w", err)`
- Separate internal error details from user-facing messages

### Sources
- [Go DI Approaches - Leapcell](https://leapcell.io/blog/go-dependency-injection-approaches-wire-vs-fx-and-manual-best-practices)
- [Mastering DI with Uber Fx - Medium](https://ali127hub.medium.com/mastering-dependency-injection-in-golang-with-uber-fx-cda80b519aa4)
- [Logging in Go with Slog - Better Stack](https://betterstack.com/community/guides/logging/logging-in-go/)
- [Go Error Handling - JetBrains Guide](https://www.jetbrains.com/guide/go/tutorials/handle_errors_in_go/best_practices/)

---

## Phase 1: Foundation (High Impact, Low Risk)

### P0-1: Standardize Error Handling with Typed Errors ✅ COMPLETED

**Priority**: P0 (Critical) | **Effort**: S | **Risk**: Low | **Completed**: February 2026

- [x] Update services to return typed errors instead of `fmt.Errorf("show not found")`
- [x] Update handlers to use `errors.As()` for type checking
- [x] Create error types for remaining domains (venue, artist)

**What was done:**
- Created `internal/errors/venue.go` (VenueError with codes: NotFound, HasShows, PendingEditExists)
- Created `internal/errors/artist.go` (ArtistError with codes: NotFound, HasShows)
- Added 3 authorization codes to `internal/errors/show.go` (UnpublishUnauthorized, MakePrivateUnauthorized, PublishUnauthorized)
- Replaced 14 `fmt.Errorf` calls in `services/show.go` with typed errors
- Replaced 10 `fmt.Errorf` calls in `services/venue.go` with typed errors
- Replaced 6 `fmt.Errorf` calls in `services/artist.go` with typed errors
- Replaced 1 `fmt.Errorf` in `services/saved_show.go` and 1 in `services/favorite_venue.go`
- Replaced 9 string comparisons in `handlers/show.go` with `errors.As()` + code switch
- Replaced 7 string comparisons in `handlers/venue.go` with `errors.As()`
- Replaced 7 string comparisons in `handlers/artist.go` with `errors.As()`
- All handlers now use consistent `apperrors` import alias

**Remaining (out of scope, see P0-1b below):**
- Auth handler and JWT middleware still use string-based error checks

---

### P0-1b: Extend Typed Errors to Auth & JWT Middleware ✅ COMPLETED

**Priority**: P0 (Critical) | **Effort**: S | **Risk**: Low | **Completed**: February 2026

- [x] `handlers/auth.go` — Replaced 4 string checks with `errors.As()` + code switch (LoginHandler, RegisterHandler, ChangePasswordHandler)
- [x] `middleware/jwt.go` — Replaced 3 string checks with `errors.As()` (JWTMiddleware, HumaJWTMiddleware, LenientHumaJWTMiddleware)
- [x] `services/user.go` — 6 `fmt.Errorf` → typed `AuthError` returns
- [x] `services/jwt.go` — 7 `fmt.Errorf` → typed `AuthError` returns, added `jwt.ErrTokenExpired` check
- [x] `errors/auth.go` — Added `CodeNoPasswordSet`, `Minutes` field, `ErrAccountLockedWithMinutes()`, `ErrNoPasswordSet()`

---

### P0-1c: Fix N+1 Artist & Venue Loading Queries ✅ COMPLETED

**Priority**: P0 (Critical — Performance) | **Effort**: S | **Risk**: Low | **Completed**: February 2026

Multiple services loaded artists/venues for shows using nested loops: one query per ShowArtist/ShowVenue row. With 10 shows × 3 artists each, this was 30+ queries instead of 2.

**Originally listed locations (already fixed before this task):**
- [x] `services/venue.go` — `GetShowsForVenue()` — already used batch loading
- [x] `services/favorite_venue.go` — `getOrderedArtistsForShow()` — already used batch loading
- [x] `services/saved_show.go` — artist loading — already used batch loading
- [x] `services/show.go` — `buildShowResponse()` — already used batch loading

**Newly discovered and fixed N+1 locations:**
- [x] `services/show.go` — `ExportShowToMarkdown()` — loaded each artist individually in a loop; fixed with batch `WHERE id IN ?`
- [x] `services/artist.go` — `GetShowsForArtist()` — loaded each venue AND each artist individually per show; fixed with batch loading for both

**Also fixed:** `services/saved_show.go` — `buildShowResponse()` was missing `Slug`, `BandcampEmbedURL` in artist responses, `Slug` in venue responses, and `Slug`, `IsSoldOut`, `IsCancelled`, `Source`, `SourceVenue`, `ScrapedAt`, `DuplicateOfShowID` in show responses

---

### P0-1d: Replace log.Fatal in Config Validation ✅ ALREADY FIXED

**Priority**: P1 (High) | **Effort**: XS | **Risk**: Low

- [x] `internal/config/config.go` — `LoadConfig()` returns `(nil, error)` and `Validate()` returns `error`. No `log.Fatal` in the config package. `main.go` handles the fatal exit via `log.Fatalf`.

---

### P0-1e: Remove or Implement AI Handler Stub

**Priority**: P3 (Nice-to-have) | **Effort**: XS | **Risk**: None

- [ ] `handlers/show.go:1200` — `TODO: Implement AI processing logic` returns `"not_implemented"`. Either implement or remove the dead endpoint.

---

### P0-2: Thread `*gorm.DB` from main.go Through Routes & Handlers ✅ COMPLETED

**Priority**: P0 (Critical) | **Effort**: M | **Risk**: Low | **Completed**: February 2026

All 18 DB-using services already accepted `*gorm.DB` with a nil fallback to `db.GetDB()`. However, every handler passed `nil` — so all services silently relied on the global singleton. This made unit testing handlers impossible without global DB state.

- [x] Add `database := db.GetDB()` in `main.go` after `db.Connect()`
- [x] Pass `database` to `routes.SetupRoutes()` and `services.NewCleanupService()`
- [x] Add `database *gorm.DB` parameter to `SetupRoutes` + 9 helper functions in `routes.go`
- [x] Add `database *gorm.DB` as first param to 10 handler constructors, pass to service constructors
- [x] Update `routes_test.go` to match new signatures (pass `nil` for test compat)

**Files Modified (13):**
- `cmd/server/main.go`
- `internal/api/routes/routes.go`, `routes_test.go`
- `internal/api/handlers/`: `show.go`, `venue.go`, `artist.go`, `admin.go`, `saved_show.go`, `favorite_venue.go`, `show_report.go`, `audit_log.go`, `apple_auth.go`, `oauth_account.go`

**Verification:**
- [x] `go build ./internal/...` passes
- [x] `go build ./cmd/server` passes
- [x] No behavioral changes — same DB instance flows through explicitly instead of via global singleton
- [x] Nil-fallback in services retained for backward compatibility (removed later in P1-1)

---

## Phase 2: Dependency Management (Medium Impact, Medium Risk)

### P1-1: Create Service Container for Centralized Lifecycle ✅ COMPLETED

**Priority**: P1 (High) | **Effort**: M | **Risk**: Medium | **Completed**: February 2026

- [x] Create `internal/services/container.go`
- [x] Move service instantiation from routes.go to container
- [x] Update routes to use container

**Files Affected:**
- `internal/services/container.go` (new file)
- `internal/api/routes/routes.go`
- `cmd/server/main.go`

**Implementation:**
```go
// container.go
package services

import (
    "gorm.io/gorm"
    "psychic-homily/backend/internal/config"
)

type Container struct {
    DB     *gorm.DB
    Config *config.Config

    // Services - lazily initialized
    userService    *UserService
    showService    *ShowService
    venueService   *VenueService
    artistService  *ArtistService
    authService    *AuthService
    jwtService     *JWTService
}

func NewContainer(db *gorm.DB, cfg *config.Config) *Container {
    return &Container{DB: db, Config: cfg}
}

func (c *Container) UserService() *UserService {
    if c.userService == nil {
        c.userService = NewUserService(c.DB)
    }
    return c.userService
}

func (c *Container) ShowService() *ShowService {
    if c.showService == nil {
        c.showService = NewShowService(c.DB)
    }
    return c.showService
}

// ... other service getters
```

**Benefits:**
- Single source of truth for service instances
- Easy to swap implementations for testing
- Clear lifecycle management
- No external dependencies (no Wire, no Fx)

**Verification:**
- [x] All services initialized through container
- [x] Routes file is cleaner
- [x] Application starts and functions correctly

---

### P1-2: Update Handlers to Accept Dependencies via Constructor ✅ COMPLETED

**Priority**: P1 (High) | **Effort**: M | **Risk**: Low | **Completed**: February 2026

- [x] Update handler constructors to accept service dependencies
- [x] Remove internal service instantiation from handlers
- [x] Pass services from container in routes

**Files Affected:**
- `internal/api/handlers/show.go`
- `internal/api/handlers/auth.go`
- `internal/api/handlers/venue.go`
- `internal/api/handlers/artist.go`
- `internal/api/handlers/admin.go`
- `internal/api/handlers/saved_show.go`
- `internal/api/handlers/favorite_venue.go`
- `internal/api/routes/routes.go`

**Before:**
```go
func NewShowHandler(cfg *config.Config) *ShowHandler {
    return &ShowHandler{
        showService:           services.NewShowService(),  // Creates new instance
        savedShowService:      services.NewSavedShowService(),
        discordService:        services.NewDiscordService(cfg),
        musicDiscoveryService: services.NewMusicDiscoveryService(cfg),
    }
}
```

**After:**
```go
func NewShowHandler(
    showService *services.ShowService,
    savedShowService *services.SavedShowService,
    discordService *services.DiscordService,
    musicDiscoveryService *services.MusicDiscoveryService,
) *ShowHandler {
    return &ShowHandler{
        showService:           showService,
        savedShowService:      savedShowService,
        discordService:        discordService,
        musicDiscoveryService: musicDiscoveryService,
    }
}
```

**Verification:**
- [x] Handlers don't create their own services
- [x] All dependencies injected from routes/container
- [x] `go test ./...` passes

---

## Phase 3: Testing Infrastructure (Medium Impact, Low Risk)

### P1-3: Create Service Interfaces for Mockable Testing ✅ COMPLETED

**Priority**: P1 (High) | **Effort**: M | **Risk**: Low | **Completed**: February 2026

- [x] Create `internal/services/interfaces.go` — 22 interfaces for all handler-consumed services
- [x] Define interfaces for services used by handlers
- [x] Add compile-time interface satisfaction checks (22 `var _ Interface = (*Concrete)(nil)`)
- [x] Update all 14 handler structs + constructors to depend on interfaces

**What was done:**
- Created `internal/services/interfaces.go` with 22 interface definitions
- Updated all 14 handler files: struct fields + constructor params changed from `*services.XService` to `services.XServiceInterface`
- Container (`container.go`) unchanged — concrete types passed to handlers satisfy interfaces implicitly
- All existing tests pass unchanged (243 handler, route, service, middleware tests)

**Verification:**
- [x] Interfaces compile correctly
- [x] Handlers can accept mock implementations
- [x] Existing tests pass (all 243 handler + route + middleware tests)

---

### P2-1: Add Handler Unit Tests with Mocks ✅ PARTIALLY COMPLETED

**Priority**: P2 (Medium) | **Effort**: L | **Risk**: Low | **Started**: February 2026

**Approach**: Hand-written struct-with-func-fields mocks (no mockery/gomock). Nil func fields panic on call — intentional, catches unexpected service interactions immediately.

- [x] Create mock structs for 7 service interfaces (`handler_unit_mock_helpers_test.go`)
- [x] SavedShowHandler tests (14 new — success, error, pagination, batch check)
- [x] FavoriteVenueHandler tests (12 new — success, error, pagination, default timezone)
- [x] AuditLogHandler tests (7 new — auth guards, success, error, limit clamping, filters)
- [x] ShowReportHandler tests (11 new — success, error, audit log, flag handling)
- [x] ArtistReportHandler tests (24 new — auth guards, success, error, audit log)
- [ ] AdminHandler mock tests — deferred (77 unit tests already exist, complex multi-dep)
- [ ] AuthHandler mock tests — deferred (34 unit tests already exist, complex auth flows)
- [ ] ShowHandler mock tests — deferred (18 unit tests already exist, complex multi-dep)

**Files Created/Modified:**
- `internal/api/handlers/handler_unit_mock_helpers_test.go` (new — 7 mock structs)
- `internal/api/handlers/saved_show_test.go` (8 → 23 tests)
- `internal/api/handlers/favorite_venue_test.go` (9 → 22 tests)
- `internal/api/handlers/audit_log_test.go` (new — 7 tests)
- `internal/api/handlers/show_report_test.go` (13 → 25 tests)
- `internal/api/handlers/artist_report_test.go` (new — 24 tests)

**Result**: 68 new mock-based tests, 311 total handler tests (162 unit + 149 integration), all passing.

**Verification:**
- [x] Hand-written mocks satisfy all interfaces (compile-time checks)
- [x] Handler tests don't require database
- [x] `go test ./internal/api/handlers/...` passes (311 tests)

---

## Phase 4: Code Quality (Low Impact, Low Risk)

### P2-2: Add Middleware Tests

**Priority**: P2 (Medium) | **Effort**: S | **Risk**: Low

- [ ] Create JWT middleware tests
- [ ] Create rate limit middleware tests
- [ ] Create security headers middleware tests

**Files to Create:**
- `internal/api/middleware/jwt_test.go`
- `internal/api/middleware/ratelimit_test.go`
- `internal/api/middleware/security_test.go`

**Verification:**
- [ ] Middleware tests cover happy path and error cases
- [ ] `go test ./internal/api/middleware/...` passes

---

### P3-1: Add Structured Context Logging to Services

**Priority**: P3 (Nice-to-have) | **Effort**: S | **Risk**: Low

- [ ] Update service methods to accept context parameter
- [ ] Use `logger.FromContext(ctx)` in services
- [ ] Ensure request ID correlation works end-to-end

**Before:**
```go
// Handler logs with context
logger.FromContext(ctx).Info("show_created", "show_id", show.ID)

// Service can't log with request correlation
func (s *ShowService) CreateShow(req *CreateShowRequest) (*ShowResponse, error)
```

**After:**
```go
func (s *ShowService) CreateShow(ctx context.Context, req *CreateShowRequest) (*ShowResponse, error) {
    logger.FromContext(ctx).Debug("creating_show", "venue_count", len(req.Venues))
    // ...
}
```

**Verification:**
- [ ] Service logs include request IDs
- [ ] Log correlation works across handler → service boundaries

---

## Implementation Order Summary

| Order | Phase | Item | Priority | Effort | Status |
|-------|-------|------|----------|--------|--------|
| 1 | 1 | Standardize Typed Errors (P0-1) | P0 | S | ✅ Done |
| 2 | 1 | Extend Typed Errors to Auth/JWT (P0-1b) | P0 | S | ✅ Done |
| 3 | 1 | Fix N+1 Artist Loading (P0-1c) | P0 | S | ✅ Done |
| 4 | 1 | Replace log.Fatal in Config (P0-1d) | P1 | XS | ✅ Done (already) |
| 5 | 1 | DB Injection in Services (P0-2) | P0 | M | ✅ Done |
| 6 | 2 | Service Container (P1-1) | P1 | M | ✅ Done |
| 7 | 2 | Handler DI (P1-2) | P1 | M | ✅ Done |
| 8 | 3 | Service Interfaces (P1-3) | P1 | M | ✅ Done |
| 9 | 3 | Handler Unit Tests (P2-1) | P2 | L | ✅ Round 1 done (5 handlers, 68 tests) |
| 10 | 4 | Middleware Tests (P2-2) | P2 | S | ✅ Done (92.7%) |
| 11 | 4 | Context in Services (P3-1) | P3 | S | |
| 12 | 4 | Remove/Implement AI Stub (P0-1e) | P3 | XS | |

---

## What NOT to Do (Avoiding Over-Engineering)

1. **Do NOT add Wire or Fx** - This is a medium-sized app. Manual DI with a simple container is cleaner and more readable.

2. **Do NOT create a repository layer** - Services already handle DB operations cleanly. Adding repositories would be unnecessary indirection for this codebase size.

3. **Do NOT add generic error handler middleware** - Huma already handles this well with typed responses.

4. **Do NOT migrate to a different router** - Chi + Huma is an excellent, well-established choice.

5. **Do NOT add event sourcing or CQRS** - Total overkill for this application.

6. **Do NOT add excessive abstractions** - Keep it simple. If you can understand the code flow by reading it linearly, don't abstract it.

7. **Do NOT over-mock** - Integration tests with testcontainers are valuable. Only add mocks where unit test speed matters.

---

## Critical Files Reference

| File | Purpose | Phase |
|------|---------|-------|
| `internal/services/show.go` | Largest service, best candidate to start DI pattern | 1, 2 |
| `internal/errors/show.go` | Excellent typed error pattern to extend | 1 |
| `internal/api/routes/routes.go` | Central routing, will need updates for container | 2 |
| `internal/api/handlers/show.go` | Handler with string-based error checking to fix | 1, 2 |
| `db/connection.go` | DB singleton to be injected instead of called globally | 1 |

---

## Test Coverage Goals

| Package | Current | Target |
|---------|---------|--------|
| `internal/api/handlers` | ~1% | 50% |
| `internal/api/middleware` | 0% | 80% |
| `internal/services` | ~20% | 40% |
| Overall | ~11.5% | 35% |

---

*Last updated: February 18, 2026*
