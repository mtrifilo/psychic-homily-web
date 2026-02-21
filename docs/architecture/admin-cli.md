# Admin CLI for Show Export/Import

A terminal-based admin tool for exporting shows from one environment and importing them to another (e.g., local → stage → production).

## Overview

The CLI allows admins to:
1. Browse shows from a source environment (local, stage, or production)
2. Select multiple shows for export
3. Preview the import on a target environment (shows artist/venue matching)
4. Confirm import, automatically creating new artists/venues as needed

## Installation & Running

```bash
cd cli

# Install dependencies
bun install

# Run in development mode
bun run dev

# Build for distribution
bun run build

# Run built version
bun run start
```

## Architecture

```
cli/
├── package.json
├── tsconfig.json
├── src/
│   ├── index.tsx              # Entry point, screen orchestration
│   ├── config/
│   │   ├── environments.ts    # API URLs for local/stage/prod
│   │   └── auth.ts            # Token storage (~/.config/psychic-homily/)
│   ├── api/
│   │   └── client.ts          # HTTP client with auth headers
│   └── screens/
│       ├── environment-select.tsx  # Environment picker
│       ├── login.tsx               # Email/password login
│       ├── show-list.tsx           # Multi-select show browser
│       ├── export-preview.tsx      # Conflict detection display
│       └── import-result.tsx       # Import results summary
```

## User Flow

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│ Select Source   │ --> │ Login to Source │ --> │ Select Target   │
│ Environment     │     │ (if needed)     │     │ Environment     │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        v
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│ Import Result   │ <-- │ Export Preview  │ <-- │ Show List       │
│ Summary         │     │ (conflicts)     │     │ (multi-select)  │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

## Screen Controls

### Environment Selection
- `↑/↓` - Navigate options
- `Enter` - Select environment
- `q` - Quit

### Login
- `Tab/↑/↓` - Switch between email/password fields
- `Enter` - Submit (in password field) or move to next field
- `Esc` - Go back
- `q` - Quit (when fields are empty)

### Show List
- `↑/↓` - Navigate shows
- `Space` - Toggle selection
- `s` - Select/deselect all on current page
- `Enter` - Export selected shows
- `t` - Change target environment
- `n/p` - Next/previous page
- `a/d/r/v/c` - Filter by status (approved/pending/rejected/private/clear)
- `Esc` - Go back
- `q` - Quit

### Export Preview
- `↑/↓` - Scroll through shows
- `Enter` - Confirm import
- `Esc` - Cancel and go back
- `q` - Quit

### Import Result
- `Enter` - Continue to show list
- `q` - Quit

## Authentication

The CLI supports multiple authentication methods:

### 1. Token from Web UI (Recommended)
The easiest and most reliable method:

1. Log into the web UI (any auth method works: Google OAuth, email/password, magic link)
2. Click your avatar in the top-right → **Settings**
3. Scroll to **CLI Authentication** section (admin-only)
4. Click **Generate CLI Token**
5. Copy the token (it won't be shown again)
6. In the CLI, press `t` at the login screen and paste the token

This token is valid for 24 hours. Generate a new one when it expires.

**Frontend implementation**: `frontend/components/settings/SettingsPanel.tsx`
**Backend endpoint**: `POST /auth/cli-token` (admin-only)

### 2. Environment Variables
Set credentials to skip the login prompt entirely:

```bash
# Add to ~/.zshrc or ~/.bashrc
export PH_LOCAL_EMAIL="your@email.com"
export PH_LOCAL_PASSWORD="yourpassword"
export PH_STAGE_EMAIL="your@email.com"
export PH_STAGE_PASSWORD="yourpassword"
export PH_PRODUCTION_EMAIL="your@email.com"
export PH_PRODUCTION_PASSWORD="yourpassword"
```

### 3. Manual Email/Password
Press `m` at the login screen to enter email and password manually.

### Token Storage

Tokens are stored in `~/.config/psychic-homily/auth.json` with per-environment storage:

```json
{
  "local": {
    "token": "eyJ...",
    "expiresAt": 1706912345000
  },
  "production": {
    "token": "eyJ...",
    "expiresAt": 1706912345000
  }
}
```

- Tokens are checked for expiry with a 5-minute buffer
- Expired tokens are automatically cleared
- Each environment maintains its own token
- OAuth tokens persist until they expire (24 hours)

## Backend API Endpoints

The CLI uses these admin-only endpoints (all require JWT auth + admin role):

### GET /admin/shows
List all shows with optional filters.

**Query Parameters:**
- `limit` (int, default 50, max 100)
- `offset` (int, default 0)
- `status` (string: pending, approved, rejected, private)
- `from_date` (RFC3339 format)
- `to_date` (RFC3339 format)
- `city` (string)

**Response:**
```json
{
  "shows": [...],
  "total": 150
}
```

### POST /admin/shows/export/bulk
Export multiple shows as base64-encoded markdown.

**Request:**
```json
{
  "show_ids": [1, 2, 3]
}
```

**Response:**
```json
{
  "exports": ["base64-encoded-markdown...", ...]
}
```

### POST /admin/shows/import/bulk/preview
Preview import with artist/venue matching.

**Request:**
```json
{
  "shows": ["base64-encoded-markdown...", ...]
}
```

**Response:**
```json
{
  "previews": [
    {
      "show": { "title": "...", "event_date": "..." },
      "venues": [
        { "name": "Valley Bar", "city": "Phoenix", "existing_id": 5, "will_create": false }
      ],
      "artists": [
        { "name": "The National", "existing_id": 12, "will_create": false },
        { "name": "New Artist", "existing_id": null, "will_create": true }
      ],
      "warnings": ["Similar show exists on same date"],
      "can_import": true
    }
  ],
  "summary": {
    "total_shows": 3,
    "new_artists": 1,
    "new_venues": 0,
    "existing_artists": 5,
    "existing_venues": 3,
    "warning_count": 1,
    "can_import_all": true
  }
}
```

### POST /admin/shows/import/bulk/confirm
Execute the import.

**Request:**
```json
{
  "shows": ["base64-encoded-markdown...", ...]
}
```

**Response:**
```json
{
  "results": [
    { "success": true, "show": { "id": 45, "title": "..." } },
    { "success": false, "error": "Duplicate headliner conflict" }
  ],
  "success_count": 2,
  "error_count": 1
}
```

### POST /auth/cli-token (Protected, Admin Only)
Generate a CLI authentication token. This endpoint is called from the web UI Settings page.

**Response:**
```json
{
  "success": true,
  "token": "eyJ...",
  "expires_in": 86400,
  "message": "CLI token generated successfully. This token expires in 24 hours."
}
```

## Artist/Venue Matching

When importing shows:

- **Artists**: Matched case-insensitively by name. New artists are created if no match.
- **Venues**: Matched case-insensitively by name + city. New venues are created if no match.
- **Admin imports**: Automatically verify new venues (no pending state).

## Export Format

Shows are exported as markdown with YAML frontmatter:

```markdown
---
version: "1.0"
exported_at: "2024-03-15T10:30:00Z"
show:
  title: "The National with Support Act"
  event_date: "2024-03-20T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  status: "approved"
venues:
  - name: "Valley Bar"
    city: "Phoenix"
    state: "AZ"
    address: "130 N Central Ave"
artists:
  - name: "The National"
    position: 0
    set_type: "headliner"
  - name: "Support Act"
    position: 1
    set_type: "opener"
---

## Description

Show description text here...
```

## Environment Configuration

Defined in `src/config/environments.ts`:

```typescript
export const environments = [
  { name: 'Local', key: 'local', apiUrl: 'http://localhost:8080' },
  { name: 'Stage', key: 'stage', apiUrl: 'https://stage.api.psychichomily.com' },
  { name: 'Production', key: 'production', apiUrl: 'https://api.psychichomily.com' },
];
```

## Troubleshooting

### "Connection failed. Is the server running?"
- Ensure the backend is running for the selected environment
- Check API URL in environments.ts

### "Account is not an admin"
- The logged-in user must have `is_admin: true` in the database

### "Invalid token"
- Token may be expired; the CLI will prompt for re-login

### Import fails with duplicate error
- A headliner is already performing at the same venue on the same date
- This is a safeguard against duplicate show entries

### Token expired
- CLI tokens from the web UI expire after 24 hours
- Generate a new token from **Settings** → **CLI Authentication**
- The CLI automatically detects expired tokens and prompts for re-authentication
