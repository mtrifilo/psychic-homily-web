# Music Discovery Agent

This document describes the AI-powered music discovery feature for Psychic Homily, enabling automatic and manual discovery of Bandcamp and Spotify URLs for artists.

## Implementation Status

**Status: Implemented** (January 2026)

### Components

| Component | File | Description |
|-----------|------|-------------|
| Backend Music Discovery Service | `backend/internal/services/music_discovery.go` | Triggers automatic discovery for new artists |
| Backend Bandcamp Update Handler | `backend/internal/api/handlers/artist.go` | `PATCH /admin/artists/{id}/bandcamp` |
| Backend Spotify Update Handler | `backend/internal/api/handlers/artist.go` | `PATCH /admin/artists/{id}/spotify` |
| Frontend Combined Discovery | `frontend/app/api/admin/artists/[id]/discover-music/route.ts` | AI-powered Bandcamp + Spotify discovery |
| Frontend Bandcamp-only Discovery | `frontend/app/api/admin/artists/[id]/discover-bandcamp/route.ts` | Manual Bandcamp discovery |
| Frontend Manual Bandcamp Update | `frontend/app/api/admin/artists/[id]/bandcamp/route.ts` | Manual URL entry |
| Admin UI Controls | `frontend/components/ArtistDetail.tsx` | Discover buttons and manual input |

---

## Overview

The Music Discovery system has two modes:

### 1. Automatic Discovery (on artist creation)
When a new artist is created during show creation or import, the backend automatically triggers AI-powered discovery to find the artist's Bandcamp or Spotify URL.

### 2. Manual Discovery (admin button click)
Admins can manually trigger discovery for any artist by clicking the "Discover Bandcamp Album" button on the artist detail page.

### Key Features

- **Bandcamp + Spotify support** - Tries Bandcamp first, falls back to Spotify
- **Automatic triggering** - Runs when new artists are created
- **Manual fallback** - Admin can enter URLs manually
- **URL validation** - Validates URLs can be embedded before saving
- **Dual authentication** - Supports both admin session and service-to-service auth

---

## Architecture

### Automatic Discovery Flow

When a new artist is created during show creation/import:

```
┌──────────────────────────────────────────────────────────────────────┐
│                    SHOW CREATION/IMPORT                              │
│              (User or admin creates show with new artist)            │
└────────────────────────────┬─────────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────────┐
│             BACKEND: CreateShow() or ConfirmShowImport()             │
│                                                                      │
│  for each newly created artist:                                      │
│    musicDiscoveryService.DiscoverMusicForArtist(artistID, name)      │
└────────────────────────────┬─────────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────────┐
│          BACKEND: MusicDiscoveryService.triggerDiscovery()           │
│              (async goroutine - fire-and-forget)                     │
│                                                                      │
│  1. Build URL: {FRONTEND_URL}/api/admin/artists/{id}/discover-music  │
│  2. Create HTTP POST request                                         │
│  3. Add header: X-Internal-Secret: {INTERNAL_API_SECRET}             │
│  4. Send request                                                     │
└────────────────────────────┬─────────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────────┐
│       FRONTEND: /api/admin/artists/[id]/discover-music/route.ts      │
│                         Next.js POST Handler                         │
│                                                                      │
│  1. Check X-Internal-Secret header OR admin session                  │
│  2. Get artist details from backend                                  │
│  3. Initialize Anthropic client                                      │
│  4. Call Claude Haiku with web_search tool                           │
│  5. Try Bandcamp first, fall back to Spotify                         │
└────────────────────────────┬─────────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────────┐
│           FRONTEND: updateArtistBandcamp() or                        │
│                     updateArtistSpotify()                            │
│                                                                      │
│  PATCH to backend:                                                   │
│    /admin/artists/{artistId}/bandcamp or /spotify                    │
│  Header: X-Internal-Secret: {INTERNAL_API_SECRET}                    │
└────────────────────────────┬─────────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────────┐
│           BACKEND: UpdateArtistBandcamp/SpotifyHandler()             │
│                                                                      │
│  1. Check X-Internal-Secret header OR admin JWT                      │
│  2. Validate URL format                                              │
│  3. Update artist in database                                        │
└──────────────────────────────────────────────────────────────────────┘
```

### Manual Discovery Flow

Admin clicks "Discover Bandcamp Album" button:

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
└─────────────────────────────────────┘
```

---

## Service-to-Service Authentication

The `INTERNAL_API_SECRET` enables secure communication between the backend and frontend without requiring a user session.

### How It Works

1. Backend and frontend share a secret via `INTERNAL_API_SECRET` env var
2. Backend includes the secret in the `X-Internal-Secret` header
3. Frontend validates the header matches the env var
4. If valid, the request is authorized without session auth

### Dual Authentication Pattern

All discovery endpoints support two authentication methods:

```typescript
// Frontend API route example
function isAuthorized(request: NextRequest): boolean {
  // Method 1: Internal service request
  const internalSecret = process.env.INTERNAL_API_SECRET
  if (internalSecret && request.headers.get('X-Internal-Secret') === internalSecret) {
    return true
  }

  // Method 2: Admin session
  const session = await getServerSession()
  return session?.user?.isAdmin === true
}
```

```go
// Backend handler example
func (h *ArtistHandler) UpdateArtistBandcampHandler(ctx context.Context, req *Request) {
    // Method 1: Internal service request
    internalSecret := os.Getenv("INTERNAL_API_SECRET")
    if internalSecret != "" && req.InternalSecret == internalSecret {
        isInternalRequest = true
    }

    // Method 2: Admin JWT
    user := middleware.GetUserFromContext(ctx)
    if user != nil && user.IsAdmin {
        isAdmin = true
    }

    if !isInternalRequest && !isAdmin {
        return huma.Error403Forbidden("Admin access required")
    }
}
```

---

## Environment Variables

### Frontend (Vercel)

| Variable | Required | Description |
|----------|----------|-------------|
| `ANTHROPIC_API_KEY` | Yes | Claude API key for AI-powered discovery |
| `INTERNAL_API_SECRET` | Yes | Shared secret for service-to-service auth (must match backend) |
| `BACKEND_URL` | Yes | Backend API URL (e.g., `https://api.psychichomily.com`) |

### Backend (Railway)

| Variable | Required | Description |
|----------|----------|-------------|
| `INTERNAL_API_SECRET` | Yes | Shared secret for service-to-service auth (must match frontend) |
| `MUSIC_DISCOVERY_ENABLED` | No | Feature flag, default `false`. Set to `true` to enable automatic discovery |
| `FRONTEND_URL` | Yes | Frontend URL for calling discovery endpoints |

### Generating the Shared Secret

```bash
openssl rand -hex 32
```

**Important:** Use the same value in both Vercel and Railway.

---

## API Contracts

### 1. Combined Discovery (Automatic + Manual)

**Endpoint**: `POST /api/admin/artists/[id]/discover-music`

**Authentication**: `X-Internal-Secret` header OR admin session

**Response (200 OK)**:
```json
{
  "success": true,
  "bandcamp_url": "https://artist.bandcamp.com/album/name",
  "spotify_url": null,
  "source": "bandcamp",
  "artist": { ... }
}
```

### 2. Backend Bandcamp Update

**Endpoint**: `PATCH /admin/artists/{artist_id}/bandcamp`

**Authentication**: `X-Internal-Secret` header OR admin JWT

**Headers**:
```
X-Internal-Secret: <secret>  (for service-to-service)
Authorization: Bearer <jwt>   (for admin users)
```

**Request**:
```json
{
  "bandcamp_embed_url": "https://artist.bandcamp.com/album/name"
}
```

### 3. Backend Spotify Update

**Endpoint**: `PATCH /admin/artists/{artist_id}/spotify`

**Authentication**: `X-Internal-Secret` header OR admin JWT

**Request**:
```json
{
  "spotify_url": "https://open.spotify.com/artist/..."
}
```

---

## Trigger Points

Automatic discovery is triggered from these locations:

### Show Creation
`backend/internal/api/handlers/show.go` (lines 351-356):
```go
for _, artist := range show.Artists {
    if artist.IsNewArtist != nil && *artist.IsNewArtist {
        h.musicDiscoveryService.DiscoverMusicForArtist(artist.ID, artist.Name)
    }
}
```

### Show Import (Admin)
`backend/internal/api/handlers/admin.go` (lines 771-776):
```go
for _, artist := range show.Artists {
    if artist.IsNewArtist != nil && *artist.IsNewArtist {
        h.musicDiscoveryService.DiscoverMusicForArtist(artist.ID, artist.Name)
    }
}
```

---

## Claude Configuration

### Model
`claude-haiku-4-5-20251001` - Fast and cost-effective for web searches

### Tools
`web_search_20250305` - Enables real-time web search

### System Prompt (Bandcamp)
```
You are a music research assistant helping to find Bandcamp album pages for artists.

Rules:
1. Search for the artist's official Bandcamp page
2. Return an album or track URL in the format: https://[artist].bandcamp.com/album/[name]
3. Do NOT return just the profile URL (e.g., https://artist.bandcamp.com)
4. Prefer full albums over single tracks
5. Prefer the most recent or most popular album
6. If you cannot find a Bandcamp page, return exactly: NOT_FOUND
```

---

## Security Considerations

1. **API Key Protection** - `ANTHROPIC_API_KEY` only accessed server-side
2. **Secret Validation** - `INTERNAL_API_SECRET` validated on every request
3. **URL Validation** - All URLs validated before saving to database
4. **Dual Auth** - Supports both internal service and admin user auth
5. **Fire-and-Forget** - Automatic discovery doesn't block show creation

---

## Testing

### Enable Automatic Discovery in Stage

1. Generate a secret: `openssl rand -hex 32`
2. Set in Railway (backend): `INTERNAL_API_SECRET=<secret>`, `MUSIC_DISCOVERY_ENABLED=true`
3. Set in Vercel (frontend): `INTERNAL_API_SECRET=<secret>`, `ANTHROPIC_API_KEY=<key>`
4. Create a show with a new artist
5. Check artist detail page - Bandcamp/Spotify should populate automatically

### Test Manual Discovery

1. Log in as admin
2. Navigate to an artist without a music embed
3. Click "Discover Bandcamp Album"
4. Verify embed appears after discovery

---

## Monitoring

### Key Metrics
- Discovery success rate (found vs NOT_FOUND)
- Average discovery latency
- Error rate by type
- Manual vs automatic discovery ratio

### Logs
Backend logs include:
- `internal_service_request` - when X-Internal-Secret is used
- `admin_update_artist_bandcamp_success` - successful updates
- `admin_update_artist_bandcamp_failed` - failed updates

---

## Future Enhancements

- **Rate Limiting** - Prevent abuse of discovery endpoints
- **Batch Discovery** - Discover music for multiple artists at once
- **Retry Logic** - Retry failed discoveries
- **Analytics** - Track discovery success rates over time
