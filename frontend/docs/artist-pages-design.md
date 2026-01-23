# Artist Pages Feature Design

## Overview

This document outlines the design for individual artist pages, allowing users to click on any artist name throughout the application and view detailed information about that artist, including their social links, upcoming and past shows, and an embedded Bandcamp player featuring their latest release.

**Status:** Phase 1 (MVP) and Phase 2 (Music Embeds) are complete. Phase 3 (Automatic Album Discovery) is planned for future enhancement.

## User Stories

1. **As a visitor**, I want to click on an artist name on a show listing to learn more about them
2. **As a visitor**, I want to see what other upcoming shows an artist has in my area
3. **As a visitor**, I want to listen to an artist's music directly on their page via Bandcamp
4. **As a visitor**, I want to find an artist's social media profiles to follow them

## Feature Requirements

### Artist Detail Page (`/artists/[id]`)

**Header Section:**
- Artist name
- Location (city, state) if available
- Social media links (Instagram, Bandcamp, Spotify, etc.)

**Music Section:**
- Bandcamp embedded player (if Bandcamp URL available)
- Fallback: Link to Bandcamp profile if embed unavailable

**Shows Section:**
- Tabbed interface (similar to venue pages):
  - **Upcoming Shows** - Shows where this artist is performing (sorted by date ascending)
  - **Past Shows** - Historical shows (sorted by date descending)
- Each show displays:
  - Date and time
  - Venue name (clickable link to venue page)
  - Other artists on the bill
  - Price and age requirement

---

## Technical Design

### Backend Changes

#### 1. New Endpoint: Get Artist Details

```
GET /artists/:artist_id
```

**Response:**
```json
{
  "id": 123,
  "name": "Glitterer",
  "city": "Philadelphia",
  "state": "PA",
  "social": {
    "instagram": "https://instagram.com/glitterer",
    "bandcamp": "https://glitterer.bandcamp.com",
    "spotify": "https://open.spotify.com/artist/...",
    "website": "https://glitterer.com"
  },
  "bandcamp_embed_url": "https://glitterer.bandcamp.com/album/rationale",
  "created_at": "2024-01-15T...",
  "updated_at": "2024-01-15T..."
}
```

#### 2. New Endpoint: Get Artist Shows

```
GET /artists/:artist_id/shows?time_filter=upcoming|past|all&limit=20&timezone=America/Phoenix
```

**Response:**
```json
{
  "artist_id": 123,
  "shows": [
    {
      "id": 456,
      "title": "Glitterer / Prince Daddy & The Hyena",
      "event_date": "2024-03-15T02:00:00Z",
      "venue": {
        "id": 13,
        "name": "Valley Bar",
        "city": "Phoenix",
        "state": "AZ"
      },
      "artists": [
        { "id": 123, "name": "Glitterer" },
        { "id": 124, "name": "Prince Daddy & The Hyena" }
      ],
      "price": 20.00,
      "age_requirement": "21+"
    }
  ],
  "total": 5
}
```

#### 3. Database Schema Addition

Add optional `bandcamp_embed_url` field to artists table:

```sql
ALTER TABLE artists ADD COLUMN bandcamp_embed_url TEXT;
```

This allows storing a specific album/track URL for embedding, separate from the profile URL in the `social` JSON field.

#### 4. Bandcamp Embed Discovery Service (Optional Enhancement)

A background service or on-demand endpoint that:
1. Takes a Bandcamp profile URL
2. Fetches and parses the artist's Bandcamp page
3. Extracts the latest/featured album URL
4. Stores it in `bandcamp_embed_url`

---

### Bandcamp Embed Integration

#### How Bandcamp Embeds Work

Bandcamp provides iframe embeds for albums/tracks:

```html
<iframe
  style="border: 0; width: 350px; height: 470px;"
  src="https://bandcamp.com/EmbeddedPlayer/album=ALBUM_ID/size=large/bgcol=181a1b/linkcol=056cc4/tracklist=false/transparent=true/"
  seamless>
  <a href="https://artist.bandcamp.com/album/album-name">Album by Artist</a>
</iframe>
```

The embed URL requires an **album or track ID**, not just the artist profile URL.

#### oEmbed API

Bandcamp supports oEmbed. Given an album URL, we can fetch embed info:

```
GET https://bandcamp.com/oembed?url=https://glitterer.bandcamp.com/album/rationale&format=json
```

**Response:**
```json
{
  "version": "1.0",
  "type": "rich",
  "provider_name": "Bandcamp",
  "provider_url": "https://bandcamp.com",
  "title": "Rationale by Glitterer",
  "html": "<iframe ...></iframe>",
  "width": 350,
  "height": 470
}
```

#### Album URL Discovery via Scraping

To automatically find an album URL from a profile URL:

**Step 1: Fetch Artist Page**
```
GET https://artistname.bandcamp.com
```

**Step 2: Parse HTML for Album Data**

Bandcamp pages contain album info in several places:
- `<script type="application/ld+json">` - Structured data with discography
- `data-tralbum` attributes on music player elements
- Album links in the discography grid (`/album/album-name`)

**Step 3: Extract Latest Release**
- Parse the structured data or HTML
- Find the most recent or featured album
- Return the full album URL

**Example Scraping Logic (Pseudocode):**
```go
func DiscoverBandcampAlbum(profileURL string) (string, error) {
    // Fetch the page
    resp, err := http.Get(profileURL)

    // Parse HTML
    doc, err := goquery.NewDocumentFromReader(resp.Body)

    // Try structured data first
    doc.Find("script[type='application/ld+json']").Each(func(i int, s *goquery.Selection) {
        // Parse JSON, look for MusicAlbum or MusicRecording types
    })

    // Fallback: Find first album link in discography
    albumLink := doc.Find("a[href*='/album/']").First().AttrOr("href", "")

    return albumLink, nil
}
```

#### Caching Strategy

- Cache discovered album URLs in the database (`bandcamp_embed_url` field)
- Set a TTL (e.g., 30 days) before re-checking for newer releases
- Allow manual override via admin interface
- Cache oEmbed responses in memory/Redis (1 hour TTL)

#### Error Handling

| Scenario | Behavior |
|----------|----------|
| No Bandcamp URL | Try Spotify embed (see below) |
| Profile URL only, scraping fails | Show link to Bandcamp profile |
| Album URL exists, oEmbed fails | Show link to album |
| Rate limited by Bandcamp | Use cached data, retry later |

---

### Spotify Embed Integration (Fallback)

When an artist has no Bandcamp URL but has a Spotify URL, we can embed their Spotify artist page which displays their top tracks.

#### How Spotify Embeds Work

Spotify provides iframe embeds and supports oEmbed:

```html
<iframe
  style="border-radius:12px"
  src="https://open.spotify.com/embed/artist/ARTIST_ID?utm_source=generator&theme=0"
  width="100%"
  height="352"
  frameBorder="0"
  allow="autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture"
  loading="lazy">
</iframe>
```

#### oEmbed API

```
GET https://open.spotify.com/oembed?url=https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb
```

**Response:**
```json
{
  "html": "<iframe ...></iframe>",
  "width": 456,
  "height": 152,
  "version": "1.0",
  "provider_name": "Spotify",
  "provider_url": "https://spotify.com",
  "type": "rich",
  "title": "Glitterer",
  "thumbnail_url": "https://i.scdn.co/image/...",
  "thumbnail_width": 640,
  "thumbnail_height": 640
}
```

#### Advantages Over Bandcamp

| Feature | Bandcamp | Spotify |
|---------|----------|---------|
| Embed from artist URL | ❌ Requires album URL | ✅ Shows top tracks |
| Album discovery needed | Yes (scraping) | No |
| Free listening | Full tracks | 30-second previews* |
| oEmbed support | ✅ | ✅ |

*Full playback requires Spotify login

#### Embed Priority Order

1. **Bandcamp album URL** - Best experience, full tracks, supports artists directly
2. **Spotify artist URL** - Good fallback, no discovery needed, shows top tracks
3. **Link to profile** - Fallback when embeds unavailable

#### URL Parsing

Extract Spotify artist ID from various URL formats:
```ts
function parseSpotifyArtistId(url: string): string | null {
  // Formats:
  // https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb
  // https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb?si=...
  // spotify:artist:4Z8W4fKeB5YxbusRsdQVPb

  const webMatch = url.match(/spotify\.com\/artist\/([a-zA-Z0-9]+)/)
  if (webMatch) return webMatch[1]

  const uriMatch = url.match(/spotify:artist:([a-zA-Z0-9]+)/)
  if (uriMatch) return uriMatch[1]

  return null
}
```

#### Theme Support

Spotify embeds support dark mode via the `theme` parameter:
- `theme=0` - Dark theme (matches our dark mode)
- `theme=1` - Light theme

---

### Frontend Changes

#### 1. New Page: `/artists/[id]/page.tsx`

```
frontend/
  app/
    artists/
      [id]/
        page.tsx        # Artist detail page
```

**Component Structure:**
```tsx
<ArtistDetail>
  <ArtistHeader>
    <ArtistName />
    <ArtistLocation />
    <SocialLinks />
  </ArtistHeader>

  <BandcampEmbed url={artist.bandcamp_embed_url} />

  <ArtistShowsList artistId={id}>
    <Tabs>
      <TabContent value="upcoming">
        <ShowsList timeFilter="upcoming" />
      </TabContent>
      <TabContent value="past">
        <ShowsList timeFilter="past" />
      </TabContent>
    </Tabs>
  </ArtistShowsList>
</ArtistDetail>
```

#### 2. Component: `MusicEmbed.tsx` (IMPLEMENTED)

A unified component that handles both Bandcamp and Spotify embeds with automatic fallback.

**Location:** `frontend/components/MusicEmbed.tsx`

```tsx
interface MusicEmbedProps {
  bandcampAlbumUrl?: string | null
  bandcampProfileUrl?: string | null
  spotifyUrl?: string | null
  artistName: string
}

export function MusicEmbed({
  bandcampAlbumUrl,
  bandcampProfileUrl,
  spotifyUrl,
  artistName,
}: MusicEmbedProps) {
  // Priority:
  // 1. Bandcamp album embed (if albumUrl exists)
  // 2. Spotify artist embed (if spotifyUrl exists)
  // 3. Link to Bandcamp profile (if profileUrl exists)
  // 4. Nothing (no music links available)
}
```

**Implementation details:**
- Uses Next.js API route `/api/oembed` to proxy oEmbed requests (avoids CORS)
- Responsive sizing via `.music-embed-container` CSS class
- Loading state with spinner while fetching oEmbed
- Graceful fallback chain with states: `loading`, `success`, `fallback`, `none`
- Uses `dangerouslySetInnerHTML` to render oEmbed HTML response

#### 3. API Route: `/api/oembed` (IMPLEMENTED)

**Location:** `frontend/app/api/oembed/route.ts`

Unified proxy endpoint to fetch oEmbed data from multiple providers:

```ts
// app/api/oembed/route.ts
export async function GET(request: Request) {
  const { searchParams } = new URL(request.url)
  const url = searchParams.get('url')

  if (!url) {
    return Response.json({ error: 'URL required' }, { status: 400 })
  }

  // Determine provider and fetch oEmbed
  let oembedUrl: string

  if (url.includes('bandcamp.com')) {
    oembedUrl = `https://bandcamp.com/oembed?url=${encodeURIComponent(url)}&format=json`
  } else if (url.includes('spotify.com') || url.includes('spotify:')) {
    oembedUrl = `https://open.spotify.com/oembed?url=${encodeURIComponent(url)}`
  } else {
    return Response.json({ error: 'Unsupported provider' }, { status: 400 })
  }

  const response = await fetch(oembedUrl)

  if (!response.ok) {
    return Response.json(
      { error: 'Failed to fetch oEmbed' },
      { status: response.status }
    )
  }

  return Response.json(await response.json())
}
```

#### 4. Make Artist Names Clickable

Update components to link artist names to their pages:

**Files to modify:**
- `components/show-list.tsx` - Main show listing
- `components/VenueShowsList.tsx` - Shows on venue pages
- `components/VenueCard.tsx` - Expanded venue card shows

**Example change:**
```tsx
// Before
<span>{artist.name}</span>

// After
<Link href={`/artists/${artist.id}`} className="hover:text-primary hover:underline">
  {artist.name}
</Link>
```

#### 5. New Hooks

```ts
// lib/hooks/useArtists.ts

export function useArtist(artistId: number) {
  return useQuery({
    queryKey: ['artists', artistId],
    queryFn: () => apiRequest<ArtistDetail>(`/artists/${artistId}`),
  })
}

export function useArtistShows(options: {
  artistId: number
  timeFilter: 'upcoming' | 'past' | 'all'
  timezone: string
  limit?: number
  enabled?: boolean
}) {
  return useQuery({
    queryKey: ['artists', options.artistId, 'shows', options.timeFilter],
    queryFn: () => apiRequest<ArtistShowsResponse>(
      `/artists/${options.artistId}/shows?time_filter=${options.timeFilter}&timezone=${options.timezone}&limit=${options.limit}`
    ),
    enabled: options.enabled,
  })
}
```

#### 6. Types

```ts
// lib/types/artist.ts

export interface ArtistDetail {
  id: number
  name: string
  city?: string | null
  state?: string | null
  social: {
    instagram?: string | null
    bandcamp?: string | null
    spotify?: string | null
    soundcloud?: string | null
    youtube?: string | null
    twitter?: string | null
    facebook?: string | null
    website?: string | null
  }
  bandcamp_embed_url?: string | null
  created_at: string
  updated_at: string
}

export interface ArtistShow {
  id: number
  title: string
  event_date: string
  venue: {
    id: number
    name: string
    city: string
    state: string
  }
  artists: Array<{
    id: number
    name: string
  }>
  price?: number | null
  age_requirement?: string | null
}

export interface ArtistShowsResponse {
  artist_id: number
  shows: ArtistShow[]
  total: number
}
```

---

## Implementation Phases

### Phase 1: Basic Artist Pages (MVP) - COMPLETED

1. **Backend:**
   - [x] Add `GET /artists/:id` endpoint
   - [x] Add `GET /artists/:id/shows` endpoint with time filtering

2. **Frontend:**
   - [x] Create `/artists/[id]/page.tsx`
   - [x] Create `ArtistDetail` component
   - [x] Create `ArtistShowsList` component
   - [x] Add hooks: `useArtist`, `useArtistShows`
   - [x] Make artist names clickable in show listings

**Deliverable:** Users can click artist names and see artist details + their shows

### Phase 2: Music Embeds (Bandcamp + Spotify) - COMPLETED

1. **Backend:**
   - [x] Add `bandcamp_embed_url` column to artists table (migration 000009)
   - [x] Update Artist model with `BandcampEmbedURL` field
   - [x] Update artist endpoints to include embed URL in response

2. **Frontend:**
   - [x] Create `MusicEmbed` component with priority-based loading:
     1. Bandcamp album URL (from `bandcamp_embed_url` field)
     2. Spotify artist URL (from `social.spotify`)
     3. Bandcamp profile link fallback (from `social.bandcamp`)
   - [x] Create `/api/oembed` proxy route for Bandcamp and Spotify
   - [x] Integrate `MusicEmbed` into `ArtistDetail` component
   - [x] Add CSS styles for responsive embed container

3. **Admin:**
   - [ ] Allow admins to manually set embed URL for artists (TODO)

**Deliverable:** Music embeds work via oEmbed when URLs are available

**Files Created:**
- `backend/db/migrations/000009_add_bandcamp_embed_url.up.sql`
- `backend/db/migrations/000009_add_bandcamp_embed_url.down.sql`
- `frontend/app/api/oembed/route.ts`
- `frontend/components/MusicEmbed.tsx`

**Files Modified:**
- `backend/internal/models/artist.go` - Added `BandcampEmbedURL` field
- `backend/internal/services/artist.go` - Added field to response struct/builder
- `frontend/lib/types/artist.ts` - Added `bandcamp_embed_url` to interface
- `frontend/components/ArtistDetail.tsx` - Integrated MusicEmbed component
- `frontend/app/globals.css` - Added `.music-embed-container` styles

### Phase 3: Automatic Album Discovery (Future Enhancement)

1. **Backend:**
   - [ ] Create Bandcamp scraping service
   - [ ] Add endpoint or background job to discover album URLs
   - [ ] Implement caching and refresh logic

2. **Frontend:**
   - [ ] Show loading/discovery state
   - [ ] Graceful fallback when discovery fails

**Deliverable:** Album URLs are automatically discovered from profile URLs

---

## Open Questions

1. **Should artist pages be public or require the artist to exist in our system?**
   - Currently artists are created when shows are submitted
   - No artist "profile management" exists yet

2. **How do we handle artist name variations?**
   - e.g., "DIIV" vs "Diiv" vs "diiv"
   - May need artist merging/aliasing feature later

3. **Should we show all shows or only approved/public shows?**
   - Recommend: Only show approved shows on public artist pages

4. **Rate limiting for Bandcamp scraping?**
   - Need to be respectful of Bandcamp's servers
   - Suggest: Max 1 request per artist per 24 hours, with caching

5. **What if an artist has no Bandcamp but has Spotify/SoundCloud?**
   - ✅ Addressed: Spotify embeds now supported as fallback (see Spotify Embed Integration section)
   - Future consideration: SoundCloud embeds (also supports oEmbed)

---

## UI Mockup (Text-based)

```
┌─────────────────────────────────────────────────────────────┐
│  ← Back to Shows                                            │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Glitterer                                                  │
│  Philadelphia, PA                                           │
│                                                             │
│  [🌐] [📷] [🎵] [Bandcamp]                                   │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────────────────────┐                        │
│  │                                 │                        │
│  │   [Bandcamp Embedded Player]    │                        │
│  │                                 │                        │
│  │   ▶ Rationale                   │                        │
│  │   by Glitterer                  │                        │
│  │                                 │                        │
│  └─────────────────────────────────┘                        │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  [Upcoming] [Past Shows]                                    │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ Sat, Mar 15                              8:00 PM    │    │
│  │ Valley Bar · Phoenix, AZ                   $20.00   │    │
│  │ w/ Prince Daddy & The Hyena, Dogleg                 │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ Sun, Mar 16                              7:00 PM    │    │
│  │ Club Congress · Tucson, AZ                 $18.00   │    │
│  │ w/ Prince Daddy & The Hyena                         │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Related Documents

- [Discord Notifications](../../backend/docs/discord-notifications.md) - May want to notify when new artist pages are viewed frequently
- [Time Utils](../lib/utils/timeUtils.ts) - Timezone handling for show dates
