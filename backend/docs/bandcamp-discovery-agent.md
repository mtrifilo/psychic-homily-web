# Bandcamp Album Discovery Agent

This document describes the AI-powered Bandcamp album discovery feature for Psychic Homily, enabling admins to automatically find and add Bandcamp album URLs for artists with one click.

## Implementation Status

**Status: Implemented** (January 2026)

### Completed Components

| Component | File | Status |
|-----------|------|--------|
| Backend endpoint | `backend/internal/api/handlers/artist.go` | ✅ Implemented |
| Route registration | `backend/internal/api/routes/routes.go` | ✅ Implemented |
| Discovery API route | `frontend/app/api/admin/artists/[id]/discover-bandcamp/route.ts` | ✅ Implemented |
| Manual update API route | `frontend/app/api/admin/artists/[id]/bandcamp/route.ts` | ✅ Implemented |
| Frontend hooks | `frontend/lib/hooks/useAdminArtists.ts` | ✅ Implemented |
| Admin UI controls | `frontend/components/ArtistDetail.tsx` | ✅ Implemented |

### Implementation Notes

- **Claude Model**: Uses `claude-haiku-4-5-20251001` with web search tool (`web_search_20250305`)
- **Anthropic SDK**: Added `@anthropic-ai/sdk` to frontend dependencies
- **Admin Validation**: Validates admin access by calling backend `/auth/profile` endpoint
- **URL Validation**: Uses existing `/api/bandcamp/album-id` route to verify URLs are embeddable
- **Rate Limiting**: Not yet implemented (marked as future enhancement)

### How to Test

1. Log in as an admin user
2. Navigate to any artist page (e.g., `/artists/123`)
3. If no Bandcamp embed exists, you'll see:
   - "Discover Bandcamp Album" button (AI-powered)
   - "Enter URL Manually" button
4. If an embed exists, you'll see an "Edit Bandcamp URL" button

### Environment Variables Required

```bash
ANTHROPIC_API_KEY=sk-ant-...  # Required for AI discovery
```

---

## Overview

The Bandcamp Discovery Agent allows admins to automatically discover Bandcamp album URLs for artists. When viewing an artist page without a music embed, admins can click "Discover Bandcamp Album" to trigger an AI-powered search that finds and saves the artist's Bandcamp album URL.

### Key Features

- **One-click discovery** - AI finds Bandcamp albums automatically
- **Manual fallback** - Text input for manual URL entry
- **URL validation** - Validates URLs can be embedded before saving
- **Admin-only** - Protected by JWT + is_admin check

## Architecture

### Design Principles

- **Next.js API Route + Claude**: AI logic in Next.js for easier streaming support later
- **Existing SDK**: Reuses `@anthropic-ai/sdk` already in the project
- **URL Validation**: Leverages existing `/api/bandcamp/album-id` route
- **Minimal Backend Changes**: Single new admin endpoint to update artist

### Flow Diagram

```
┌─────────────────────┐
│  ArtistDetail.tsx   │
│  (Admin Button)     │
└──────────┬──────────┘
           │ POST
           ▼
┌─────────────────────────────────────┐
│  /api/admin/artists/[id]/           │
│  discover-bandcamp/route.ts         │
│                                     │
│  1. Validate admin session          │
│  2. Call Claude with web search     │
│  3. Validate URL with /api/bandcamp │
│  4. Update artist via Go backend    │
└──────────┬──────────────────────────┘
           │ PATCH
           ▼
┌─────────────────────────────────────┐
│  Go Backend                         │
│  PATCH /admin/artists/{id}/bandcamp │
│                                     │
│  Updates bandcamp_embed_url field   │
└─────────────────────────────────────┘
```

### Manual URL Flow

```
┌─────────────────────┐
│  ArtistDetail.tsx   │
│  (Manual Input)     │
└──────────┬──────────┘
           │ POST
           ▼
┌─────────────────────────────────────┐
│  /api/admin/artists/[id]/           │
│  bandcamp/route.ts                  │
│                                     │
│  1. Validate admin session          │
│  2. Validate URL with /api/bandcamp │
│  3. Update artist via Go backend    │
└──────────┬──────────────────────────┘
           │ PATCH
           ▼
┌─────────────────────────────────────┐
│  Go Backend                         │
│  PATCH /admin/artists/{id}/bandcamp │
└─────────────────────────────────────┘
```

## API Contracts

### 1. Backend: Admin Artist Bandcamp Update

Updates an artist's `bandcamp_embed_url` field.

**Endpoint**: `PATCH /admin/artists/{artist_id}/bandcamp`

**Authentication**: JWT with `is_admin: true`

**Request**:
```json
{
  "bandcamp_embed_url": "https://artistname.bandcamp.com/album/album-name"
}
```

**Response (200 OK)**:
```json
{
  "id": 123,
  "name": "Artist Name",
  "state": "AZ",
  "city": "Phoenix",
  "bandcamp_embed_url": "https://artistname.bandcamp.com/album/album-name",
  "social": {
    "instagram": "https://instagram.com/artist",
    "bandcamp": "https://artist.bandcamp.com",
    ...
  },
  "created_at": "2025-01-15T10:30:00Z",
  "updated_at": "2025-01-20T14:22:00Z"
}
```

**Error Responses**:

| Status | Condition | Response |
|--------|-----------|----------|
| 400 | Invalid artist ID | `{ "detail": "Invalid artist ID" }` |
| 400 | Invalid URL format | `{ "detail": "Invalid Bandcamp URL format" }` |
| 403 | Not admin | `{ "detail": "Admin access required" }` |
| 404 | Artist not found | `{ "detail": "Artist not found" }` |
| 500 | Database error | `{ "detail": "Failed to update artist (request_id: xxx)" }` |

### 2. Next.js: Discovery Route

AI-powered discovery of Bandcamp albums.

**Endpoint**: `POST /api/admin/artists/[id]/discover-bandcamp`

**Authentication**: Session cookie with admin user

**Request**: No body required (artist ID from URL)

**Response (200 OK)**:
```json
{
  "success": true,
  "bandcamp_url": "https://artistname.bandcamp.com/album/album-name",
  "artist": {
    "id": 123,
    "name": "Artist Name",
    "bandcamp_embed_url": "https://artistname.bandcamp.com/album/album-name",
    ...
  }
}
```

**Error Responses**:

| Status | Condition | Response |
|--------|-----------|----------|
| 401 | Not authenticated | `{ "error": "Authentication required" }` |
| 403 | Not admin | `{ "error": "Admin access required" }` |
| 404 | Artist not found | `{ "error": "Artist not found" }` |
| 404 | No Bandcamp found | `{ "error": "NOT_FOUND", "message": "Could not find Bandcamp album for this artist" }` |
| 422 | Invalid URL from AI | `{ "error": "INVALID_URL", "message": "AI returned invalid Bandcamp URL" }` |
| 500 | API error | `{ "error": "Discovery failed", "message": "..." }` |
| 503 | Claude unavailable | `{ "error": "AI service unavailable" }` |

### 3. Next.js: Manual Update Route

Manual Bandcamp URL update with validation.

**Endpoint**: `POST /api/admin/artists/[id]/bandcamp`

**Authentication**: Session cookie with admin user

**Request**:
```json
{
  "bandcamp_url": "https://artistname.bandcamp.com/album/album-name"
}
```

**Response (200 OK)**:
```json
{
  "success": true,
  "artist": {
    "id": 123,
    "name": "Artist Name",
    "bandcamp_embed_url": "https://artistname.bandcamp.com/album/album-name",
    ...
  }
}
```

**Error Responses**: Same as discovery route, plus:

| Status | Condition | Response |
|--------|-----------|----------|
| 400 | Missing URL | `{ "error": "bandcamp_url is required" }` |
| 400 | Not a Bandcamp URL | `{ "error": "URL must be a Bandcamp album or track URL" }` |
| 422 | URL has no album ID | `{ "error": "Could not extract album ID from URL" }` |

## TypeScript Interfaces

### Frontend Types

```typescript
// frontend/lib/types/artist.ts (additions)

/**
 * Request to update an artist's Bandcamp embed URL
 */
export interface UpdateArtistBandcampRequest {
  bandcamp_url: string
}

/**
 * Response from Bandcamp discovery endpoint
 */
export interface DiscoverBandcampResponse {
  success: boolean
  bandcamp_url?: string
  artist?: Artist
  error?: string
  message?: string
}

/**
 * Response from manual Bandcamp update endpoint
 */
export interface UpdateBandcampResponse {
  success: boolean
  artist?: Artist
  error?: string
}
```

### API Endpoint Additions

```typescript
// frontend/lib/api.ts (additions to API_ENDPOINTS)

ADMIN: {
  // ... existing endpoints
  ARTISTS: {
    DISCOVER_BANDCAMP: (artistId: number) =>
      `/api/admin/artists/${artistId}/discover-bandcamp`,
    UPDATE_BANDCAMP: (artistId: number) =>
      `/api/admin/artists/${artistId}/bandcamp`,
  },
}
```

## Go Handler Implementation

### Handler Code

```go
// backend/internal/api/handlers/artist.go (additions)

// UpdateArtistBandcampRequest represents the request for updating bandcamp URL
type UpdateArtistBandcampRequest struct {
	ArtistID string `path:"artist_id" validate:"required" doc:"Artist ID"`
	Body     struct {
		BandcampEmbedURL *string `json:"bandcamp_embed_url" doc:"Bandcamp album or track URL for embedding"`
	}
}

// UpdateArtistBandcampResponse represents the response for updating bandcamp URL
type UpdateArtistBandcampResponse struct {
	Body *services.ArtistDetailResponse
}

// UpdateArtistBandcampHandler handles PATCH /admin/artists/{artist_id}/bandcamp
func (h *ArtistHandler) UpdateArtistBandcampHandler(ctx context.Context, req *UpdateArtistBandcampRequest) (*UpdateArtistBandcampResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		logger.FromContext(ctx).Warn("admin_access_denied",
			"user_id", getUserID(user),
			"request_id", requestID,
		)
		return nil, huma.Error403Forbidden("Admin access required")
	}

	// Parse artist ID
	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	// Validate URL format if provided
	if req.Body.BandcampEmbedURL != nil && *req.Body.BandcampEmbedURL != "" {
		if !isValidBandcampURL(*req.Body.BandcampEmbedURL) {
			return nil, huma.Error400BadRequest("Invalid Bandcamp URL format")
		}
	}

	logger.FromContext(ctx).Debug("admin_update_artist_bandcamp_attempt",
		"artist_id", artistID,
		"admin_id", user.ID,
	)

	// Update the artist
	updates := map[string]interface{}{
		"bandcamp_embed_url": req.Body.BandcampEmbedURL,
	}

	artist, err := h.artistService.UpdateArtist(uint(artistID), updates)
	if err != nil {
		if err.Error() == "artist not found" {
			return nil, huma.Error404NotFound("Artist not found")
		}
		logger.FromContext(ctx).Error("admin_update_artist_bandcamp_failed",
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update artist (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_update_artist_bandcamp_success",
		"artist_id", artistID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &UpdateArtistBandcampResponse{Body: artist}, nil
}

// isValidBandcampURL validates that the URL is a proper Bandcamp album/track URL
func isValidBandcampURL(url string) bool {
	// Must contain bandcamp.com
	if !strings.Contains(url, "bandcamp.com") {
		return false
	}
	// Must be an album or track URL
	if !strings.Contains(url, "/album/") && !strings.Contains(url, "/track/") {
		return false
	}
	return true
}
```

### Route Registration

```go
// backend/internal/api/routes.go (additions)

// Admin artist routes
huma.Register(api, huma.Operation{
	OperationID: "updateArtistBandcamp",
	Method:      http.MethodPatch,
	Path:        "/admin/artists/{artist_id}/bandcamp",
	Summary:     "Update artist Bandcamp embed URL",
	Description: "Admin-only endpoint to update an artist's Bandcamp embed URL",
	Tags:        []string{"Admin", "Artists"},
	Security: []map[string][]string{
		{"bearerAuth": {}},
	},
}, artistHandler.UpdateArtistBandcampHandler)
```

## Claude Prompt Engineering

### System Prompt

```
You are a music research assistant helping to find Bandcamp album pages for artists.

Your task is to find the official Bandcamp album page for the given artist name.

Rules:
1. Search for the artist's official Bandcamp page
2. Return an album or track URL in the format: https://[artist].bandcamp.com/album/[name] or https://[artist].bandcamp.com/track/[name]
3. Do NOT return just the profile URL (e.g., https://artist.bandcamp.com)
4. Prefer full albums over single tracks when available
5. Prefer the most recent album, or the artist's most popular/representative work
6. If the artist has multiple Bandcamp pages, prefer the official/verified one
7. If you cannot find a Bandcamp page for this artist, return exactly: NOT_FOUND

Important:
- Only return Bandcamp URLs, not Spotify, SoundCloud, or other platforms
- The URL must be embeddable (album or track page, not profile page)
- Return ONLY the URL on a single line, or NOT_FOUND - no other text
```

### User Message Template

```
Find the official Bandcamp album page for: {artist_name}
```

### Example Interactions

**Input**: "Find the official Bandcamp album page for: Radio Moscow"
**Expected Output**: `https://radiomoscow.bandcamp.com/album/brain-cycles`

**Input**: "Find the official Bandcamp album page for: The Beatles"
**Expected Output**: `NOT_FOUND`

### Claude API Configuration

```typescript
const response = await anthropic.messages.create({
  model: "claude-haiku-4-5-20251001", // Fast and cost-effective for simple searches
  max_tokens: 256,
  tools: [{
    type: "web_search",
    name: "web_search",
  }],
  messages: [{
    role: "user",
    content: `Find the official Bandcamp album page for: ${artistName}`
  }],
  system: SYSTEM_PROMPT,
})
```

## Frontend Component Behavior

### ArtistDetail.tsx Admin Controls

```
┌────────────────────────────────────────────────────────────────┐
│                        Artist Name                             │
│                        Phoenix, AZ                             │
│                                                                │
│  [Instagram] [Bandcamp] [Spotify]                              │
│                                                                │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  No music embed configured                               │  │
│  │                                                          │  │
│  │  [🔍 Discover Bandcamp Album]  (primary button)          │  │
│  │                                                          │  │
│  │  ─────────── or ───────────                              │  │
│  │                                                          │  │
│  │  [____________________________________] [Save]           │  │
│  │   Enter Bandcamp album URL manually                      │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                │
├────────────────────────────────────────────────────────────────┤
│                     Upcoming Shows                             │
└────────────────────────────────────────────────────────────────┘
```

### State Transitions

```
                    ┌─────────────┐
                    │   Initial   │
                    │  (no embed) │
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
           ▼               ▼               ▼
    ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
    │  Discover   │ │   Manual    │ │   Cancel    │
    │   Click     │ │   Input     │ │             │
    └──────┬──────┘ └──────┬──────┘ └─────────────┘
           │               │
           ▼               ▼
    ┌─────────────┐ ┌─────────────┐
    │  Loading    │ │  Validate   │
    │  (spinner)  │ │  & Save     │
    └──────┬──────┘ └──────┬──────┘
           │               │
     ┌─────┴─────┐   ┌─────┴─────┐
     ▼           ▼   ▼           ▼
┌─────────┐ ┌───────────┐ ┌─────────┐ ┌───────────┐
│ Success │ │   Error   │ │ Success │ │   Error   │
│ (toast) │ │  (toast)  │ │ (toast) │ │  (toast)  │
└─────────┘ └───────────┘ └─────────┘ └───────────┘
```

### When Embed Exists

```
┌────────────────────────────────────────────────────────────────┐
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                                                          │  │
│  │              [Bandcamp Embed Player]                     │  │
│  │                                                          │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                │
│  [✏️ Edit Embed URL]  (secondary button, admin only)           │
│                                                                │
│  (Clicking reveals the input field pre-filled with current URL)│
└────────────────────────────────────────────────────────────────┘
```

## React Hooks

### useDiscoverBandcamp

```typescript
// frontend/lib/hooks/useAdminArtists.ts

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { queryKeys } from '../queryClient'
import type { DiscoverBandcampResponse, Artist } from '../types/artist'

/**
 * Hook for AI-powered Bandcamp album discovery (admin only)
 */
export function useDiscoverBandcamp() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (artistId: number): Promise<DiscoverBandcampResponse> => {
      const response = await fetch(`/api/admin/artists/${artistId}/discover-bandcamp`, {
        method: 'POST',
        credentials: 'include',
      })

      if (!response.ok) {
        const error = await response.json()
        throw new Error(error.message || error.error || 'Discovery failed')
      }

      return response.json()
    },
    onSuccess: (data, artistId) => {
      // Invalidate artist query to refresh the embed
      queryClient.invalidateQueries({ queryKey: ['artist', artistId] })
    },
  })
}

/**
 * Hook for manually updating artist Bandcamp URL (admin only)
 */
export function useUpdateArtistBandcamp() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      artistId,
      bandcampUrl,
    }: {
      artistId: number
      bandcampUrl: string
    }): Promise<{ success: boolean; artist?: Artist }> => {
      const response = await fetch(`/api/admin/artists/${artistId}/bandcamp`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ bandcamp_url: bandcampUrl }),
      })

      if (!response.ok) {
        const error = await response.json()
        throw new Error(error.message || error.error || 'Update failed')
      }

      return response.json()
    },
    onSuccess: (data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['artist', variables.artistId] })
    },
  })
}
```

## Security Considerations

### 1. Authentication & Authorization

- **JWT validation**: Backend verifies JWT token on every request
- **Admin check**: `user.IsAdmin` must be `true` for all admin endpoints
- **Session validation**: Next.js routes validate session cookie

### 2. API Key Protection

```typescript
// API key only accessed server-side in Next.js API route
const anthropic = new Anthropic({
  apiKey: process.env.ANTHROPIC_API_KEY, // Never exposed to client
})
```

### 3. Rate Limiting

Implement rate limiting at the Next.js API route level:

```typescript
// Recommended: 10 discoveries per minute per user
import { Ratelimit } from '@upstash/ratelimit'
import { Redis } from '@upstash/redis'

const ratelimit = new Ratelimit({
  redis: Redis.fromEnv(),
  limiter: Ratelimit.slidingWindow(10, '1 m'),
  analytics: true,
  prefix: 'bandcamp-discovery',
})

export async function POST(request: Request, { params }: { params: { id: string } }) {
  const session = await getServerSession()
  if (!session?.user?.id) {
    return NextResponse.json({ error: 'Authentication required' }, { status: 401 })
  }

  const { success, limit, reset, remaining } = await ratelimit.limit(session.user.id)

  if (!success) {
    return NextResponse.json(
      { error: 'Rate limit exceeded', reset_at: reset },
      { status: 429 }
    )
  }

  // ... proceed with discovery
}
```

### 4. URL Validation

- Validate URL format before saving to database
- Verify URL is embeddable using existing `/api/bandcamp/album-id` route
- Reject non-Bandcamp URLs

### 5. Input Sanitization

- Artist names passed to Claude are sanitized
- URLs are validated against injection attacks
- Database queries use parameterized statements (via GORM)

## Error Handling Matrix

| Scenario | Detection | User Feedback | Logging |
|----------|-----------|---------------|---------|
| Not authenticated | Session check | "Please log in" redirect | Warn: unauthorized access attempt |
| Not admin | is_admin check | "Admin access required" toast | Warn: non-admin access attempt |
| Artist not found | DB query | "Artist not found" error | Info: 404 for artist ID |
| Bandcamp not found | Claude returns NOT_FOUND | "No Bandcamp found for this artist" toast | Info: no result for artist |
| Invalid URL from AI | URL validation fails | "Could not validate URL, try manual entry" toast | Warn: AI returned invalid URL |
| Invalid manual URL | Format/embed validation | "Invalid Bandcamp URL" error | Info: validation failure |
| Rate limited | Rate limiter | "Too many requests, try again in X seconds" toast | Info: rate limit hit |
| Claude API error | API exception | "Discovery service unavailable" toast | Error: full error details |
| Backend API error | HTTP 5xx | "Something went wrong" toast | Error: full error details |
| Network error | Fetch exception | "Network error, please retry" toast | Error: network failure |

## Database Changes

**None required** - the `bandcamp_embed_url` field already exists on the `artists` table.

Verify with:
```sql
SELECT column_name, data_type
FROM information_schema.columns
WHERE table_name = 'artists' AND column_name = 'bandcamp_embed_url';
```

## Testing Strategy

### Unit Tests

**Go Handler Tests** (`backend/internal/api/handlers/artist_test.go`):
```go
func TestUpdateArtistBandcampHandler(t *testing.T) {
    tests := []struct {
        name           string
        artistID       string
        isAdmin        bool
        url            string
        expectedStatus int
    }{
        {"valid update", "1", true, "https://artist.bandcamp.com/album/test", 200},
        {"not admin", "1", false, "https://artist.bandcamp.com/album/test", 403},
        {"invalid artist ID", "abc", true, "https://artist.bandcamp.com/album/test", 400},
        {"invalid URL", "1", true, "https://spotify.com/artist", 400},
        {"profile URL rejected", "1", true, "https://artist.bandcamp.com", 400},
        {"clear URL", "1", true, "", 200},
    }
    // ... test implementation
}
```

**Next.js API Route Tests**:
```typescript
// frontend/app/api/admin/artists/[id]/discover-bandcamp/route.test.ts
describe('discover-bandcamp route', () => {
  it('returns 401 when not authenticated', async () => { /* ... */ })
  it('returns 403 when not admin', async () => { /* ... */ })
  it('returns valid Bandcamp URL on success', async () => { /* ... */ })
  it('returns NOT_FOUND when no Bandcamp exists', async () => { /* ... */ })
  it('validates URL before saving', async () => { /* ... */ })
  it('respects rate limits', async () => { /* ... */ })
})
```

### Integration Tests

```typescript
// Test full flow from button click to database update
describe('Bandcamp Discovery Integration', () => {
  it('discovers and saves Bandcamp URL for artist', async () => {
    // 1. Login as admin
    // 2. Create test artist without Bandcamp URL
    // 3. Trigger discovery
    // 4. Verify artist updated in database
    // 5. Verify embed renders on page
  })
})
```

### Manual Testing Checklist

- [ ] Non-logged-in user cannot see admin controls
- [ ] Non-admin user cannot see admin controls
- [ ] Admin can see "Discover Bandcamp Album" button when no embed exists
- [ ] Clicking discover shows loading spinner
- [ ] Successful discovery shows toast and embed appears
- [ ] "NOT_FOUND" shows appropriate message
- [ ] Manual URL input validates Bandcamp URLs
- [ ] Manual URL input rejects non-Bandcamp URLs
- [ ] Existing embed shows "Edit" button for admins
- [ ] Edit mode pre-fills current URL
- [ ] Rate limiting prevents abuse

## Implementation Order

1. **Phase 1: Backend Endpoint**
   - Add `UpdateArtistBandcampHandler` to `artist.go`
   - Register route in `routes.go`
   - Add URL validation helper
   - Write handler tests

2. **Phase 2: Next.js API Routes**
   - Create `/api/admin/artists/[id]/bandcamp/route.ts` (manual update)
   - Create `/api/admin/artists/[id]/discover-bandcamp/route.ts` (AI discovery)
   - Add rate limiting
   - Add session/admin validation

3. **Phase 3: Frontend Hooks**
   - Create `useDiscoverBandcamp` hook
   - Create `useUpdateArtistBandcamp` hook
   - Add to `frontend/lib/hooks/useAdminArtists.ts`

4. **Phase 4: UI Components**
   - Update `ArtistDetail.tsx` with admin controls
   - Add loading/error states
   - Add toast notifications
   - Test responsive layout

5. **Phase 5: Testing & Polish**
   - Write integration tests
   - Manual testing across browsers
   - Error message polish
   - Documentation updates

## Future Enhancements

### Batch Discovery

Add ability to discover Bandcamp for multiple artists at once:

```typescript
POST /api/admin/artists/batch-discover
{
  "artist_ids": [1, 2, 3, 4, 5]
}
```

### Auto-Refresh

Periodically re-check artists whose Bandcamp URLs return 404:

```go
// Cron job: Check weekly for broken embeds
func RefreshBrokenEmbeds() {
    artists := GetArtistsWithEmbedURL()
    for _, artist := range artists {
        if !IsURLValid(artist.BandcampEmbedURL) {
            // Mark for review or trigger re-discovery
        }
    }
}
```

### Discovery Queue

For large batch operations, use a job queue:

```
Admin triggers batch → Jobs added to queue → Workers process async → Results saved
```

### Analytics

Track discovery success rates:

```sql
CREATE TABLE bandcamp_discovery_logs (
    id SERIAL PRIMARY KEY,
    artist_id INT REFERENCES artists(id),
    admin_id INT REFERENCES users(id),
    success BOOLEAN,
    source VARCHAR(20), -- 'ai_discovery' or 'manual'
    url_found TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);
```

## Environment Variables

Add to `.env`:

```bash
# Already exists - used for AI discovery
ANTHROPIC_API_KEY=sk-ant-...

# Optional: Rate limiting (if using Upstash)
UPSTASH_REDIS_REST_URL=https://...
UPSTASH_REDIS_REST_TOKEN=...
```

## Monitoring

### Key Metrics

- Discovery success rate (found vs NOT_FOUND)
- Average discovery latency
- Rate limit hits
- Error rate by type
- Manual vs AI discovery ratio

### Alerts

- Alert if discovery error rate > 20%
- Alert if Claude API latency > 10s
- Alert if rate limit hits spike (potential abuse)
