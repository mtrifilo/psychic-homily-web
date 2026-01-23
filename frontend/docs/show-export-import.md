# Show Export/Import Feature

Export shows as markdown files and import them via the admin panel. This feature enables easy show data backup, transfer between environments, and bulk show creation.

## Overview

- **Export**: Download any show as a markdown file (development only)
- **Import**: Upload markdown files via admin panel drag-and-drop to create new shows

## Markdown Format

Exported shows use YAML frontmatter with a markdown body:

```markdown
---
version: "1.0"
exported_at: "2024-01-15T14:30:00Z"

show:
  title: "Night of Electronic Chaos"
  event_date: "2024-03-15T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  price: 15.00
  age_requirement: "21+"
  status: "approved"

venues:
  - name: "Valley Bar"
    city: "Phoenix"
    state: "AZ"
    address: "130 N Central Ave"
    zipcode: "85004"
    social:
      instagram: "valleybarphx"
      website: "https://valleybarphx.com"

artists:
  - name: "Noise Collective"
    position: 0
    set_type: "headliner"
    city: "Phoenix"
    state: "AZ"
    social:
      bandcamp: "noisecollective"
      instagram: "noisecollective"
  - name: "Ambient Waves"
    position: 1
    set_type: "opener"
---

## Description

An evening of experimental electronic music featuring local and touring artists.
```

### Field Reference

#### Show Fields

| Field | Required | Description |
|-------|----------|-------------|
| `title` | No | Show title (can be empty) |
| `event_date` | Yes | ISO 8601 datetime with timezone |
| `city` | No | City where show takes place |
| `state` | No | State abbreviation (e.g., "AZ") |
| `price` | No | Ticket price as decimal |
| `age_requirement` | No | Age restriction (e.g., "21+", "All Ages") |
| `status` | No | Ignored on import; shows created as approved |

#### Venue Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Venue name |
| `city` | Yes | Venue city |
| `state` | Yes | State abbreviation |
| `address` | No | Street address |
| `zipcode` | No | ZIP code |
| `social` | No | Social media links (see below) |

#### Artist Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Artist name |
| `position` | Yes | Order in lineup (0 = first) |
| `set_type` | Yes | "headliner" or "opener" |
| `city` | No | Artist's home city |
| `state` | No | Artist's home state |
| `social` | No | Social media links (see below) |

#### Social Fields (for venues and artists)

All social fields are optional:
- `instagram` - Instagram username
- `facebook` - Facebook username/page
- `twitter` - Twitter/X username
- `youtube` - YouTube channel
- `spotify` - Spotify artist ID
- `soundcloud` - SoundCloud username
- `bandcamp` - Bandcamp username
- `website` - Full URL

## Exporting Shows

### Requirements

- Development environment only (`ENVIRONMENT=development`)
- Export endpoint returns 404 in production

### How to Export

1. Navigate to a show detail page or show list
2. Click the **Export** button (download icon)
3. A markdown file downloads with format: `show-YYYY-MM-DD-title-slug.md`

### Using the ExportShowButton Component

```tsx
import { ExportShowButton } from '@/components/ExportShowButton'

// In your component:
<ExportShowButton showId={123} showTitle="My Show" />

// With different variants:
<ExportShowButton showId={123} variant="outline" size="icon" />
```

The button only renders in development mode (`process.env.NODE_ENV === 'development'`).

## Importing Shows

### Requirements

- Admin access required
- Valid markdown file with YAML frontmatter

### How to Import

1. Go to **Admin Console** → **Import Show** tab
2. Drag and drop a `.md` file (or click to browse)
3. Review the **Preview**:
   - Show details (title, date, location)
   - Venues: shows if existing or will be created
   - Artists: shows if existing or will be created
   - Warnings: duplicate detection, missing fields
4. Click **Confirm Import** to create the show

### Preview Information

The preview shows match results for venues and artists:

- **Exists** (green): Found existing record, will link to it
- **Will create** (gray): No match found, will create new record

### Matching Logic

**Venues** are matched by:
```sql
LOWER(name) = ? AND LOWER(city) = ?
```
Venues are unique within a city (e.g., "The Rebel Lounge" in Phoenix ≠ "The Rebel Lounge" in Tucson).

**Artists** are matched by:
```sql
LOWER(name) = ?
```
Artist names are globally unique.

### Duplicate Detection

The preview warns if:
- A headliner already has a show at the same venue on the same date
- Required fields are missing (event_date, venues, artists)

Warnings don't prevent import but alert you to potential issues.

### Auto-Verification

When admins import shows:
- New venues are automatically marked as **verified**
- Shows are created with **approved** status

## API Endpoints

### Export Endpoint (dev only)

```
GET /shows/{show_id}/export
```

**Response:**
- Content-Type: `text/markdown; charset=utf-8`
- Content-Disposition: `attachment; filename="show-2024-03-15-title.md"`

Returns 404 in production.

### Import Preview

```
POST /admin/shows/import/preview
Authorization: Bearer <token>
Content-Type: application/json

{
  "content": "<base64-encoded markdown>"
}
```

**Response:**
```json
{
  "show": {
    "title": "Night of Electronic Chaos",
    "event_date": "2024-03-15T20:00:00Z",
    "city": "Phoenix",
    "state": "AZ"
  },
  "venues": [
    {
      "name": "Valley Bar",
      "city": "Phoenix",
      "state": "AZ",
      "existing_id": 42,
      "will_create": false
    }
  ],
  "artists": [
    {
      "name": "Noise Collective",
      "position": 0,
      "set_type": "headliner",
      "existing_id": null,
      "will_create": true
    }
  ],
  "warnings": [],
  "can_import": true
}
```

### Import Confirm

```
POST /admin/shows/import/confirm
Authorization: Bearer <token>
Content-Type: application/json

{
  "content": "<base64-encoded markdown>"
}
```

**Response:** Standard ShowResponse object with the created show.

## Use Cases

### Backup Shows

Export important shows to keep local backups of show data.

### Transfer Between Environments

1. Export shows from production (if export is temporarily enabled)
2. Import to staging/development for testing

### Bulk Show Creation

1. Create a markdown template
2. Duplicate and modify for each show
3. Import via admin panel

### Data Migration

Export shows from one database, import to another after schema changes.

## Troubleshooting

### "Invalid base64 content" Error

The markdown file content must be valid UTF-8. Check for encoding issues if the file was edited in an external tool.

### "Missing event date" Warning

The `event_date` field in the frontmatter is required and must be valid ISO 8601 format.

### Venues Not Matching

Venue matching is case-insensitive but requires exact name match. Check for:
- Extra spaces
- Different punctuation ("The Rebel Lounge" vs "Rebel Lounge")
- City spelling differences

### Artists Not Matching

Artist matching is case-insensitive but exact. Verify the artist name matches exactly what's in the database.
