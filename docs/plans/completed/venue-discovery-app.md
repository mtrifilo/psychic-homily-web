# Local Venue Discovery Web Application

A local web application for discovering venue events with a proper UI, replacing the CLI-based discovery workflow. The app runs on your laptop and imports shows directly to Stage or Production via the Go backend API.

## Overview

The discovery app allows admins to:
1. Select which venues to discover from a configured list
2. Quick preview events without loading detail pages
3. Select specific events for full discovery (with artist extraction)
4. Preview import results with dry run mode
5. Import directly to Stage or Production

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│          Local Discovery App (Bun + React + Vite)           │
│                    http://localhost:5173                     │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ 1. Select   │→ │ 2. Preview  │→ │ 3. Select & Import  │ │
│  │   Venues    │  │   Events    │  │      Shows          │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
│                                                             │
│  Backend: Bun server with Playwright (localhost:3001)       │
└───────────────────────────┬─────────────────────────────────┘
                            │ POST /admin/discovery/import
                            ▼
┌─────────────────────────────────────────────────────────────┐
│              Go Backend (Stage or Production)               │
│                                                             │
│  POST /admin/discovery/import - accepts discovered JSON     │
└─────────────────────────────────────────────────────────────┘
```

## Installation & Running

```bash
cd discovery

# Install dependencies
bun install

# Install Playwright browsers (first time only)
bunx playwright install chromium

# Run in development mode (starts both server and UI)
bun run dev
```

This starts:
- **Discovery server** on http://localhost:3001 (Playwright-powered discovery)
- **Web UI** on http://localhost:5173 (Vite + React)

Open http://localhost:5173 in your browser.

## Directory Structure

```
discovery/
├── package.json
├── tsconfig.json
├── vite.config.ts
├── src/
│   ├── main.tsx              # React entry
│   ├── App.tsx               # Main app with step navigation
│   ├── index.css             # Tailwind CSS
│   ├── components/
│   │   ├── VenueSelector.tsx # Step 1: Checkbox grid of venues
│   │   ├── EventPreview.tsx  # Step 2: Quick preview results
│   │   ├── EventSelector.tsx # Step 3: Select events for full discovery
│   │   ├── ImportPanel.tsx   # Step 4: Review & import
│   │   └── Settings.tsx      # Token & environment config
│   ├── lib/
│   │   ├── api.ts            # Backend API client
│   │   ├── config.ts         # Venue configurations
│   │   └── types.ts          # TypeScript interfaces
│   └── server/
│       ├── index.ts          # Bun HTTP server
│       └── providers/
│           ├── ticketweb.ts  # TicketWeb/Stateside Presents discovery
│           ├── types.ts      # Discovery interfaces
│           └── index.ts      # Discovery registry
```

## Authentication

### API Tokens (Long-lived)

The discovery app uses long-lived API tokens for authentication:

1. Go to **Profile** -> scroll to **API Tokens** section (admin only)
2. Click **Create Token**
3. Set a description (e.g., "Discovery on Mike's laptop")
4. Set expiration (default 90 days, max 365 days)
5. Copy the token (starts with `phk_`) - it won't be shown again
6. In the discovery app, go to **Settings** and paste the token

Tokens can be revoked from the web UI at any time.

### Token Format

API tokens use the prefix `phk_` (psychic homily key) followed by 64 hex characters:
```
phk_a1b2c3d4e5f6...
```

The backend middleware automatically detects tokens by this prefix and validates them separately from JWT tokens.

## User Workflow

### Step 1: Select Venues
- Grid of configured venues with discovery type badges
- Checkboxes to select which to discover
- Select All / Clear buttons

### Step 2: Preview Events
- Quick load (~3s per venue) - no detail pages visited
- Shows date and title for each event
- Can preview each venue independently or all at once

### Step 3: Select Events
- Checkboxes to select events for full discovery
- Future events only (past events filtered out)
- Select All / None per venue
- Full discovery fetches artist details from event pages

### Step 4: Import
- Review all discovered events
- Choose target: Stage or Production
- **Dry Run**: Preview what would be imported
- **Live Import**: Execute the import
- Shows results: imported, duplicates, rejected, errors

## Data Export Feature

The discovery app also includes a **Data Export** feature for syncing local development data to Stage or Production.

### How It Works

1. Click **Data Export** in the header
2. Load data from your local Go backend (must be running on localhost:8080)
3. Browse shows, artists, and venues in tabbed view
4. Select items to upload using checkboxes
5. Preview (dry run) before committing
6. Import to Stage or Production

### Use Cases

- **Testing locally**: Create test shows on your local database, then upload to Stage for integration testing
- **Seeding data**: Upload artists and venues from local to Production
- **Data migration**: Move specific shows between environments

### Requirements

- Local Go backend running on `localhost:8080`
- Same API token works for both local and remote (if same user account exists)
- Admin access required

### Data Export API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/export/shows` | GET | Export shows with artists/venues |
| `/admin/export/artists` | GET | Export artists |
| `/admin/export/venues` | GET | Export venues |
| `/admin/data/import` | POST | Import shows/artists/venues |

## Backend API Endpoints

### POST /admin/discovery/import

Import discovered events to the database.

**Headers:**
```
Authorization: Bearer phk_your_token_here
Content-Type: application/json
```

**Request:**
```json
{
  "events": [
    {
      "id": "event-123",
      "title": "The National with Phoebe Bridgers",
      "date": "2026-03-20",
      "venue": "Valley Bar",
      "venueSlug": "valley-bar",
      "imageUrl": "https://...",
      "doorsTime": "7:00 pm",
      "showTime": "8:00 pm",
      "ticketUrl": "https://...",
      "artists": ["The National", "Phoebe Bridgers"],
      "scrapedAt": "2026-02-03T10:30:00Z"
    }
  ],
  "dryRun": true
}
```

**Response:**
```json
{
  "total": 5,
  "imported": 3,
  "duplicates": 1,
  "rejected": 1,
  "errors": 0,
  "messages": [
    "IMPORTED: The National at Valley Bar on 2026-03-20 20:00",
    "DUPLICATE: Event already imported as show #123",
    "REJECTED: Matches previously rejected show #45"
  ]
}
```

### API Token Management Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/tokens` | POST | Create new API token |
| `/admin/tokens` | GET | List user's tokens |
| `/admin/tokens/{id}` | DELETE | Revoke a token |

## Discovery Server API

Local endpoints served by the Bun server:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/discovery/venues` | GET | List configured venues (includes city/state) |
| `/discovery/preview/:slug` | GET | Quick preview of events for one venue |
| `/discovery/preview-batch` | POST | Parallel preview of multiple venues (max 5 concurrent) |
| `/discovery/discover/:slug` | POST | Full discovery with details |
| `/discovery/health` | GET | Health check |

### Batch Preview Request

```json
POST /discovery/preview-batch
{
  "venueSlugs": ["valley-bar", "crescent-ballroom", "the-van-buren"]
}
```

### Batch Preview Response

```json
[
  { "venueSlug": "valley-bar", "events": [...] },
  { "venueSlug": "crescent-ballroom", "events": [...] },
  { "venueSlug": "the-van-buren", "error": "Timeout loading page" }
]
```

## Adding New Venues

### 1. Add to Frontend Config

Edit `discovery/src/lib/config.ts`:

```typescript
export const VENUES: VenueConfig[] = [
  // ... existing venues
  {
    slug: 'new-venue',
    name: 'New Venue Name',
    providerType: 'ticketweb', // or new discovery type
    url: 'https://venue-website.com/calendar/',
    city: 'Phoenix',
    state: 'AZ',
  },
]
```

### 2. Add to Backend Config

Edit `backend/internal/services/discovery.go`:

```go
var VenueConfig = map[string]struct {
    Name    string
    City    string
    State   string
    Address string
}{
    // ... existing venues
    "new-venue": {
        Name:    "New Venue Name",
        City:    "Phoenix",
        State:   "AZ",
        Address: "123 Main St",
    },
}
```

### 3. Add to Server Config

Edit `discovery/src/server/index.ts`:

```typescript
const VENUES: Record<string, { name: string; providerType: string; city: string; state: string }> = {
  // ... existing venues
  'new-venue': { name: 'New Venue Name', providerType: 'ticketweb', city: 'Phoenix', state: 'AZ' },
}
```

### 4. Implement New Discovery Module (if needed)

If the venue uses a different ticketing system, create a new discovery module in `discovery/src/server/providers/`:

```typescript
// discovery/src/server/providers/seetickets.ts
import type { DiscoveryProvider } from './types'

export const seeticketsProvider: DiscoveryProvider = {
  async preview(venueSlug: string) {
    // Quick scan implementation
  },
  async discover(venueSlug: string, eventIds: string[]) {
    // Full discovery implementation
  },
}
```

Register in `discovery/src/server/providers/index.ts`:

```typescript
import { seeticketsProvider } from './seetickets'

const providers: Record<string, DiscoveryProvider> = {
  ticketweb: ticketwebProvider,
  seetickets: seeticketsProvider,
}
```

## Database Changes

### Migration: 000021_add_api_tokens

Creates `api_tokens` table for long-lived token storage:

```sql
CREATE TABLE api_tokens (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    description VARCHAR(255),
    scope VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_used_at TIMESTAMP WITH TIME ZONE,
    revoked_at TIMESTAMP WITH TIME ZONE
);
```

Run migration:
```bash
cd backend
go run ./cmd/migrate up
```

## Files Changed/Created

### Backend (Go)

| File | Change |
|------|--------|
| `internal/services/discovery.go` | Added `ImportEvents()` method |
| `internal/services/api_token.go` | **NEW** - Token CRUD service |
| `internal/models/api_token.go` | **NEW** - Token model |
| `internal/api/handlers/admin.go` | Added discovery import + token handlers |
| `internal/api/middleware/jwt.go` | Added API token auth support |
| `internal/api/routes/routes.go` | Registered new routes |
| `db/migrations/000021_add_api_tokens.up.sql` | **NEW** - Migration |

### Frontend (Next.js)

| File | Change |
|------|--------|
| `lib/api.ts` | Added token + discovery endpoints |
| `lib/hooks/useAuth.ts` | Added token hooks |
| `components/settings/api-token-management.tsx` | **NEW** - Token UI |
| `components/settings/SettingsPanel.tsx` | Added token management section |
| `components/settings/index.ts` | Export new component |

### Discovery App (New)

| Directory | Description |
|-----------|-------------|
| `discovery/` | Complete new Vite + React app |
| `discovery/src/components/` | UI components |
| `discovery/src/lib/` | API client and config |
| `discovery/src/server/` | Bun server with Playwright |

## Scalability Features

The discovery app is designed to handle 30+ venues across multiple states:

### Parallel Preview
- **Batch endpoint**: `POST /discovery/preview-batch` discovers up to 5 venues concurrently
- **Progress tracking**: UI shows real-time progress bar during batch preview
- **Error isolation**: Individual venue errors don't block other venues

### City/State Filtering
- **Filter buttons**: Quickly filter venues by city (e.g., "Phoenix, AZ", "Denver, CO")
- **Grouped display**: Venues are grouped by city in the selection grid
- **Select per city**: "Select All" respects current city filter

### Configuration
- Venues include `city` and `state` fields for proper grouping
- All three config locations must match:
  1. `discovery/src/lib/config.ts` (frontend)
  2. `discovery/src/server/index.ts` (discovery server)
  3. `backend/internal/services/discovery.go` (Go backend)

## Next Steps

### Phase 5: Polish & Extend

- [ ] **Add more discovery modules**: SeeTickets, Eventbrite, etc.
- [ ] **Error handling**: Retry logic for network failures
- [x] **Progress indicators**: Real-time discovery progress (implemented)
- [ ] **Discovery history**: Log of past discovery sessions
- [ ] **Scheduled discovery**: Cron job to auto-discover periodically
- [ ] **Event deduplication**: Smarter duplicate detection beyond source_event_id

### Potential Improvements

1. [x] **Batch discovery**: Discover from multiple venues in parallel (implemented)
2. [ ] **Image downloading**: Save event images to S3
3. [ ] **Artist matching**: Fuzzy matching for artist name variations
4. [ ] **Notifications**: Discord/email when new events found
5. [ ] **Mobile-friendly**: Responsive UI for tablet use
6. [ ] **Database-driven venues**: Move venue config to database for easier management

### Known Limitations

- Only TicketWeb discovery module implemented currently
- Requires manual token copy/paste (no OAuth flow in local app)
- No persistence of discovery state between sessions
- Playwright requires Chrome/Chromium installed
- New venues require code changes in 3 places

## Troubleshooting

### "API token not configured"
Go to Settings and paste your API token.

### "Failed to preview events"
- Check if the discovery server is running on port 3001
- Ensure Playwright browsers are installed: `bunx playwright install chromium`

### "403 Forbidden" on import
- Token may be expired or revoked
- User may no longer be an admin
- Generate a new token from the web UI

### Discovery hangs or times out
- Some venues have slow-loading calendars
- Try increasing timeout in `discovery/src/server/providers/ticketweb.ts`
- Check browser console for JavaScript errors on the venue site

### "Unknown venue: xyz"
- Venue slug must match in all three places:
  1. `discovery/src/lib/config.ts`
  2. `discovery/src/server/index.ts`
  3. `backend/internal/services/discovery.go`
