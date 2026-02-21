# Psychic Homily iOS App - V1 Implementation Plan

## Overview

A focused iOS companion app targeting two high-value mobile use cases:
1. **Browse & save shows** - View upcoming shows, filter by city, save to personal list
2. **Submit shows from screenshots** - Capture show flyers, AI extracts info, submit with minimal friction

The existing backend API covers ~90% of what the iOS client needs. Main backend additions: Sign in with Apple, token-in-response-body auth, and porting the AI extraction endpoint from Next.js to Go.

### V1 Scope
- **Auth**: Email/password + Sign in with Apple
- **Screenshot OCR**: Send image to backend (Go) for AI processing
- **Push notifications**: Deferred to V2
- **Offline mode**: Online-only for V1

---

## Implementation Progress

**Last updated**: 2026-02-08

### Completed
- **All backend changes** (Phases 1, 4, 5 backend):
  - Token returned in login/register/magic-link response bodies
  - Lenient JWT middleware for token refresh (7-day grace period)
  - Sign in with Apple endpoint (`POST /auth/apple/callback`) with JWK validation + caching
  - AI extraction service ported from Next.js to Go (Anthropic API + entity matching)
  - `go build ./internal/...` passes cleanly
- **All iOS Swift code** (39 files total):
  - Models (7 files): User, Show, Artist, Venue, SavedShow, Extraction, APIResponse
  - Networking (3 files): APIClient actor (with 401 refresh), APIEndpoints enum, KeychainManager
  - App shell (2 files): PsychicHomilyApp (TabView + URL handling), AppState (auth observable)
  - ViewModels (7 files): Auth, ShowList, ShowDetail, SavedShows, ArtistDetail, VenueDetail, SubmitShow
  - Extensions (2 files): Date+Formatting (Arizona timezone), Color+Theme (brand colors)
  - Shared Views (5 files): LoadingView, ErrorView, SaveButton, SocialLinksView, StatusBadge
  - Auth Views (2 files): LoginView (email/password + Sign in with Apple), RegisterView
  - Show Views (4 files): ShowListView (infinite scroll + city filter), ShowRowView, ShowDetailView, CityFilterView
  - Detail Views (2 files): ArtistDetailView (social links + shows), VenueDetailView (social links + shows)
  - Saved Shows (1 file): SavedShowsView (swipe-to-unsave)
  - Submit Views (3 files): SubmitShowView (camera + PhotosPicker), ExtractionResultView, ShowFormView
  - Profile (1 file): ProfileView (user info + sign out)
  - Share Extension (1 file): ShareViewController (App Group image sharing)

### Review Fixes Applied
- Added `Hashable` conformance to `Show`, `ShowArtist`, `ShowVenue` (required by `NavigationLink(value:)`)
- Fixed Share Extension to use `extensionContext?.open()` instead of broken responder chain
- Wired up App Group image loading in `SubmitShowView` (reads shared image, cleans up, auto-extracts)
- Unified submit flow: `ShowFormView` → `onSubmitted` callback → `SubmitShowView.reset()`
- Added swipe-to-save on `ShowListView` rows (authenticated only)
- Added session restore on app launch (`.task { await appState.restoreSession() }`)

### Backend Tests (completed)
- `jwt_lenient_test.go` — 12 tests for `ValidateTokenLenient`: grace period boundaries, invalid/malformed/empty tokens, wrong issuer/audience, missing exp claim
- `apple_auth_test.go` — 15 tests: `IsEmailVerified` (bool/string/nil/int), `ValidateIdentityToken` (valid, expired, wrong aud/kid/key, malformed), JWK fetch caching + refetch + error handling
- `extraction_test.go` — 22 tests: `parseExtractionResponse` (direct/markdown/bare/invalid JSON), `extractRawArtists` (multiple/missing/empty/bad types), `buildUserContent` (text/image/both), `ExtractShow` validation (missing key, invalid type, empty text, too long, invalid media types)
- Also fixed pre-existing test compilation issues in `jwt_test.go` and `auth_test.go` caused by `*gorm.DB` parameter additions to service constructors
- Fixed pre-existing call-site issues: `middleware/jwt.go`, `cmd/discovery-import/main.go`

### Not Started
- Xcode project file (.xcodeproj) — must be created in Xcode, then add existing Swift files
- Share Extension Xcode target setup (code is written, needs target configuration)
- Polish, testing, App Store submission (Phases 8-9)

---

## Architecture

- **Language**: Swift 6 (strict concurrency)
- **UI**: SwiftUI (iOS 17+)
- **Architecture**: MVVM with `@Observable` macro
- **Networking**: URLSession async/await (no third-party HTTP libs)
- **Secure storage**: Keychain Services
- **Dependencies**: SPM only, zero third-party packages for V1
- **Photo capture**: `PhotosPicker` (SwiftUI) + `UIImagePickerController` (camera)

### Project Structure

```
PsychicHomily/
  PsychicHomily/
    App/
      PsychicHomilyApp.swift          # @main, tab bar, URL handling
      AppState.swift                   # Auth state, shared observable
    Models/
      Show.swift                       # Show, ShowResponse, UpcomingShowsResponse
      Artist.swift                     # Artist, ArtistSocials
      Venue.swift                      # Venue, VenueSocials
      User.swift                       # User profile
      SavedShow.swift                  # SavedShowResponse
      Extraction.swift                 # ExtractedShowData, ExtractedArtist, ExtractedVenue
      APIResponse.swift                # Generic error wrappers
    Networking/
      APIClient.swift                  # Actor-based HTTP client, token injection
      APIEndpoints.swift               # Endpoint enum (path, method, auth)
      AuthInterceptor.swift            # 401 handling, token refresh
      KeychainManager.swift            # Keychain CRUD for JWT
    ViewModels/
      ShowListViewModel.swift          # Upcoming shows, filtering, cursor pagination
      ShowDetailViewModel.swift        # Show detail, save/unsave
      SavedShowsViewModel.swift        # My List
      ArtistDetailViewModel.swift      # Artist profile + shows
      VenueDetailViewModel.swift       # Venue profile + shows
      AuthViewModel.swift              # Login, register, Sign in with Apple
      SubmitShowViewModel.swift        # Image capture, extraction, form, submit
    Views/
      Shows/                           # ShowListView, ShowRowView, ShowDetailView, CityFilterView
      SavedShows/                      # SavedShowsView
      Artists/                         # ArtistDetailView
      Venues/                          # VenueDetailView
      Submit/                          # SubmitShowView, ExtractionResultView, ShowFormView
      Auth/                            # LoginView, RegisterView
      Profile/                         # ProfileView
      Shared/                          # SocialLinksView, LoadingView, ErrorView, SaveButton, StatusBadge
    Extensions/
      Date+Formatting.swift            # Arizona timezone (America/Phoenix, no DST)
      Color+Theme.swift                # Brand colors
    ShareExtension/
      ShareViewController.swift        # Receives images from other apps
    Resources/
      Assets.xcassets
  PsychicHomilyTests/
  PsychicHomilyUITests/
```

---

## Tab Bar (4 tabs)

| Tab | SF Symbol | Screen | Auth Required |
|-----|-----------|--------|---------------|
| Shows | `music.note.list` | Upcoming shows + city filters | No |
| My List | `bookmark.fill` | Saved shows | Yes |
| Submit | `camera.fill` | Screenshot capture + submission | Yes |
| Profile | `person.circle` | User info, settings, login | No |

Tabs requiring auth show inline login prompts (not modals) when unauthenticated.

### Navigation Flow

```
Shows -> ShowDetail -> ArtistDetail -> ShowDetail (recursive)
                    -> VenueDetail  -> ShowDetail (recursive)

My List -> ShowDetail -> (same as above)

Submit -> ExtractionResult -> ShowForm -> Success

Profile -> LoginView -> RegisterView
```

---

## API Client

### Auth Strategy: Bearer Tokens

The backend middleware already checks `Authorization: Bearer <token>` before cookies. The iOS app uses this header approach exclusively.

**Token lifecycle:**
1. Login/register -> backend returns JWT in response body (backend change 7A)
2. Store JWT in Keychain (`kSecAttrAccessibleAfterFirstUnlock`)
3. Attach `Authorization: Bearer <token>` on every authenticated request
4. On 401 `token_expired` -> attempt refresh via `POST /auth/refresh` (backend change 7B)
5. On refresh failure -> clear Keychain, navigate to login

### APIClient (actor for thread-safe token access)

```swift
actor APIClient {
    static let shared = APIClient()
    func request<T: Decodable>(_ endpoint: APIEndpoint) async throws -> T
    func upload<T: Decodable>(_ endpoint: APIEndpoint, imageData: Data, mimeType: String) async throws -> T
}
```

### Cursor-based Pagination

`GET /shows/upcoming` uses cursor pagination. `ShowListViewModel` maintains `nextCursor` and appends pages on scroll via `.onAppear` sentinel row.

---

## Backend Changes Required

### 7A. Token in Login/Register Response Bodies

**File**: `backend/internal/api/handlers/auth.go`

Add `Token string` field to `LoginResponse.Body` and `RegisterResponse.Body`. Set after JWT generation. Web frontend ignores it (uses cookies); iOS reads it.

### 7B. Lenient JWT Middleware for /auth/refresh

**File**: `backend/internal/api/middleware/jwt.go`

Create `LenientHumaJWTMiddleware` for `/auth/refresh` that accepts tokens expired within a 7-day grace period. Parse with time validation disabled, then manually check `exp` within grace window.

### 7C. Sign in with Apple Endpoint

**New files**:
- `backend/internal/services/apple_auth.go` - Apple JWT validation (JWK set from Apple)
- `backend/internal/api/handlers/apple_auth.go` - Handler for `POST /auth/apple/callback`

**Endpoint**: `POST /auth/apple/callback`
**Request**: `{ identity_token, first_name?, last_name? }`
**Logic**: Validate Apple JWT -> find/create user via oauth_accounts -> return JWT

**Config**: Add `APPLE_BUNDLE_ID` env var

### 7D. Port AI Extraction to Go

**Modified files**:
- `backend/internal/api/handlers/show.go` - Replace stubbed `AIProcessShowHandler`
- `backend/internal/services/extraction.go` - New service

Port logic from `frontend/app/api/ai/extract-show/route.ts`:
1. Accept `{ type, text?, image_data?, media_type? }`
2. Call Anthropic API (Claude Haiku 4.5 with vision)
3. Parse JSON response
4. Match artists/venues against search services
5. Return `ExtractShowResponse` with match data

**Config**: Add `ANTHROPIC_API_KEY`

### 7E. No Database Migrations Needed

`oauth_accounts` already supports arbitrary providers. Apple accounts use `provider = 'apple'`.

---

## Implementation Phases & Checklists

### Phase 1: Foundation & Auth (Week 1-2)

**iOS:**
- [ ] Create Xcode project with all targets and capabilities
- [x] Implement `APIClient` actor, `KeychainManager`, `APIEndpoints`
- [x] Define all Codable models matching backend API responses
- [x] Implement `AuthViewModel` with email/password login + registration
- [x] Build `LoginView` and `RegisterView`
- [x] Build tab bar shell (`PsychicHomilyApp.swift` with TabView)
- [x] Implement `AppState` observable (auth state, session restore, tab selection)
- [x] Implement all ViewModels (ShowList, ShowDetail, SavedShows, ArtistDetail, VenueDetail, SubmitShow)
- [x] Implement `Date+Formatting.swift` (Arizona timezone) and `Color+Theme.swift` (brand colors)

**Backend:**
- [x] Add `Token` field to `LoginResponse.Body`, `RegisterResponse.Body`, and `VerifyMagicLinkResponse.Body`
- [x] Set token value in `LoginHandler`, `RegisterHandler`, and `VerifyMagicLinkHandler` after JWT generation
- [x] Create `ValidateTokenLenient` method in `jwt.go`
- [x] Create `LenientHumaJWTMiddleware` in `middleware/jwt.go`
- [x] Register `/auth/refresh` under lenient middleware (7-day grace period)
- [x] Test: `jwt_lenient_test.go` — 12 tests for `ValidateTokenLenient` (grace period, boundaries, rejection cases)

### Phase 2: Show Browsing (Week 2-3)

- [x] Implement `ShowListViewModel` with cursor pagination + city filtering
- [x] Build `ShowListView` with pull-to-refresh and infinite scroll
- [x] Build `ShowRowView` card component
- [x] Implement `CityFilterView` with `GET /shows/cities`
- [x] Build `ShowDetailView` (full show info, artists, venue)
- [x] Build `ArtistDetailView` with social links + upcoming shows
- [x] Build `VenueDetailView` with social links + upcoming shows
- [x] Implement Arizona timezone date formatting (`America/Phoenix`, no DST)

### Phase 3: Saved Shows (Week 3-4)

- [x] Implement `SavedShowsViewModel` with save/unsave + list
- [x] Build `SavedShowsView` (My List tab)
- [x] Add save/unsave toggle to `ShowDetailView` and `ShowRowView`
- [x] Optimistic UI updates for save/unsave actions (in ViewModels)
- [x] Auth guard inline prompts for unauthenticated users

### Phase 4: Sign in with Apple (Week 4-5)

**Backend:**
- [x] Implement `AppleAuthService` - Apple JWT validation (JWK set fetch + verify + cache)
- [x] Create `AppleAuthHandler` with `POST /auth/apple/callback`
- [x] Add `APPLE_BUNDLE_ID` to config
- [x] Register route in rate-limited auth group
- [x] Tests: `apple_auth_test.go` — 15 tests (IsEmailVerified, ValidateIdentityToken, JWK caching)

**iOS:**
- [x] Implement Apple Sign In in `AuthViewModel` (ASAuthorizationController delegate)
- [x] Add Sign in with Apple button to `LoginView`
- [x] Capture first-sign-in name/email, send to backend
- [ ] Test: new user, returning user, email-linked accounts

### Phase 5: Screenshot Submission - Backend (Week 5-6)

- [x] Implement `services/extraction.go` with Anthropic API client (raw HTTP)
- [x] Port system prompt from Next.js route
- [x] Implement JSON response parsing (handle markdown blocks, bare JSON)
- [x] Implement artist matching via `ArtistService.SearchArtists()`
- [x] Implement venue matching via `VenueService.SearchVenues()`
- [x] Replace stubbed `AIProcessShowHandler` with real implementation
- [x] Update `AIProcessShowRequest` to match `ExtractShowRequest` contract
- [x] Add `ANTHROPIC_API_KEY` to config
- [x] Tests: `extraction_test.go` — 22 tests (parseExtractionResponse, extractRawArtists, buildUserContent, ExtractShow validation)
- [ ] Deploy to staging

### Phase 6: Screenshot Submission - iOS (Week 6-7)

- [x] Implement image compression (JPEG quality 0.8, max 2048px) + base64 encoding (in `SubmitShowViewModel`)
- [x] Build `SubmitShowView` with camera + photo library options
- [x] Build `ExtractionResultView` (match indicators, suggestions)
- [x] Build `ShowFormView` (pre-filled, editable)
- [x] Wire up `POST /shows` submission
- [x] Error states, loading states, success confirmation

### Phase 7: Share Extension (Week 7-8)

- [ ] Create Share Extension Xcode target + App Group (`group.com.psychichomily`)
- [x] Implement `ShareViewController` (receive image, write to shared container)
- [x] Register `psychichomily://` URL scheme in main app (code in PsychicHomilyApp.swift)
- [x] Handle URL in `PsychicHomilyApp.onOpenURL`
- [ ] Configure activation rule: `NSExtensionActivationSupportsImageWithMaxCount = 1` (Xcode)
- [ ] Test from Instagram, Safari, Photos, iMessage

### Phase 8: Polish & Testing (Week 8-9)

- [ ] Error states and retry for all screens
- [ ] Loading skeletons / spinners
- [ ] Haptic feedback on save/unsave
- [ ] Test on iPhone SE, 15, 16 Pro Max
- [ ] VoiceOver accessibility pass
- [ ] Dynamic Type font scaling
- [ ] Unit tests for ViewModels and APIClient
- [ ] UI tests for login -> browse -> save -> submit flows

### Phase 9: App Store Submission (Week 9-10)

- [ ] App Store Connect listing (description, keywords, category: Music)
- [ ] Screenshots for required device sizes
- [ ] Privacy policy URL
- [ ] Privacy nutrition labels (Email linked, Name optional linked, Photos not linked, User Content linked)
- [ ] TestFlight internal -> external beta
- [ ] Submit for review

---

## Xcode Configuration

### Targets
| Target | Bundle ID |
|--------|-----------|
| PsychicHomily | `com.psychichomily.ios` |
| PsychicHomilyShareExtension | `com.psychichomily.ios.share` |
| PsychicHomilyTests | `com.psychichomily.ios.tests` |

### Capabilities
- Sign in with Apple (main app)
- App Groups `group.com.psychichomily` (main app + share extension)
- Keychain Sharing `com.psychichomily.ios` (main app)

### Build Configurations
| Config | API Base URL |
|--------|-------------|
| Debug | `http://localhost:8080` |
| Staging | `https://stage-api.psychichomily.com` |
| Release | `https://api.psychichomily.com` |

---

## Key Files Reference

### Backend (existing, modified)
| Purpose | File |
|---------|------|
| JWT middleware (Bearer + lenient) | `backend/internal/api/middleware/jwt.go` |
| JWT service (ValidateTokenLenient) | `backend/internal/services/jwt.go` |
| Auth handlers (token in response body) | `backend/internal/api/handlers/auth.go` |
| Route registration (lenient group, Apple route) | `backend/internal/api/routes/routes.go` |
| Show handler (real AI extraction) | `backend/internal/api/handlers/show.go` |
| Backend config (Apple + Anthropic) | `backend/internal/config/config.go` |

### Backend (new)
| Purpose | File |
|---------|------|
| Apple auth service (JWT validation, JWK caching) | `backend/internal/services/apple_auth.go` |
| Apple auth handler (POST /auth/apple/callback) | `backend/internal/api/handlers/apple_auth.go` |
| AI extraction service (Anthropic API, entity matching) | `backend/internal/services/extraction.go` |
| Apple auth tests (15 tests) | `backend/internal/services/apple_auth_test.go` |
| Extraction tests (22 tests) | `backend/internal/services/extraction_test.go` |
| Lenient JWT tests (12 tests) | `backend/internal/services/jwt_lenient_test.go` |

### iOS App
| Purpose | File |
|---------|------|
| App entry + tab bar | `ios/PsychicHomily/App/PsychicHomilyApp.swift` |
| Auth state (shared observable) | `ios/PsychicHomily/App/AppState.swift` |
| Actor-based HTTP client | `ios/PsychicHomily/Networking/APIClient.swift` |
| Endpoint definitions | `ios/PsychicHomily/Networking/APIEndpoints.swift` |
| Keychain token storage | `ios/PsychicHomily/Networking/KeychainManager.swift` |
| All Codable models | `ios/PsychicHomily/Models/*.swift` |
| All ViewModels | `ios/PsychicHomily/ViewModels/*.swift` |
| Arizona timezone formatting | `ios/PsychicHomily/Extensions/Date+Formatting.swift` |
| Brand colors | `ios/PsychicHomily/Extensions/Color+Theme.swift` |

### Reference (not modified)
| Purpose | File |
|---------|------|
| AI extraction logic (ported from) | `frontend/app/api/ai/extract-show/route.ts` |
| Extraction types (canonical contract) | `frontend/lib/types/extraction.ts` |
| OAuth account model | `backend/internal/models/user.go` |
| Artist/Venue search services | `backend/internal/services/artist.go`, `venue.go` |

---

## Verification Checklist

### Backend
- [x] `go test ./internal/services/` — all 49 new tests + pre-existing tests pass (except Docker-dependent `user_test.go`)
- [ ] `POST /auth/login` returns `token` in response body
- [ ] `POST /auth/register` returns `token` in response body
- [ ] `POST /auth/apple/callback` with Apple identity token works
- [ ] `POST /shows/ai-process` with base64 image returns structured extraction
- [ ] `POST /auth/refresh` with recently-expired token succeeds

### iOS
- [ ] Login, browse shows, save/unsave, view artist/venue detail on Simulator
- [ ] Camera capture and photo picker on physical device
- [ ] Share Extension: share from Photos -> opens submission flow
- [ ] Sign in with Apple on physical device
- [ ] Error states shown appropriately, no crashes on network failure
