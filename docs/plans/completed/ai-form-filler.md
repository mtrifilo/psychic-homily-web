# AI Form Filler-Outer Feature

**Date:** 2026-01-31
**Status:** Complete

## Overview

AI-powered form assistant that extracts show information from unstructured text or images (flyers/screenshots) and auto-populates the submission form. Artists and venues are matched against the database to use existing entity IDs when possible.

---

## Architecture

```
User Input (text or image)
        │
        ▼
┌─────────────────────────────────┐
│ AIFormFiller Component          │
│  - Text tab / Image drop zone   │
│  - Base64 encodes images        │
│  - Calls extraction API         │
└─────────────────────────────────┘
        │ POST /api/ai/extract-show
        ▼
┌─────────────────────────────────┐
│ API Route                       │
│  - Validates auth (any user)    │
│  - Calls Claude with vision     │
│  - Parses JSON response         │
│  - Searches artists/venues      │
│  - Returns matched data         │
└─────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────┐
│ ShowForm                        │
│  - Receives extracted data      │
│  - Populates fields via         │
│    form.setFieldValue()         │
│  - Shows match indicators       │
└─────────────────────────────────┘
```

---

## Files Created

### 1. Type Definitions
**File:** `/frontend/lib/types/extraction.ts`

```typescript
interface ExtractShowRequest {
  type: 'text' | 'image' | 'both'  // 'both' for image + additional context
  text?: string           // Required for 'text', optional context for 'both'
  image_data?: string     // Base64 for images (required for 'image' or 'both')
  media_type?: 'image/jpeg' | 'image/png' | 'image/gif' | 'image/webp'
}

interface ExtractedArtist {
  name: string
  is_headliner: boolean
  matched_id?: number      // Database ID if matched
  matched_name?: string    // Canonical name from DB
  matched_slug?: string
}

interface ExtractedVenue {
  name: string
  city?: string
  state?: string
  matched_id?: number
  matched_name?: string
  matched_slug?: string
}

interface ExtractedShowData {
  artists: ExtractedArtist[]
  venue?: ExtractedVenue
  date?: string    // YYYY-MM-DD
  time?: string    // HH:MM (24-hour)
  cost?: string    // e.g., "$20", "Free"
  ages?: string    // e.g., "21+", "All Ages"
  description?: string
}

interface ExtractShowResponse {
  success: boolean
  data?: ExtractedShowData
  error?: string
  warnings?: string[]
}
```

### 2. API Route
**File:** `/frontend/app/api/ai/extract-show/route.ts`

- **Auth:** Requires authenticated user (any user, not admin-only)
- **Model:** `claude-haiku-4-5-20251001` (vision-capable, cost-effective)
- **Input handling:**
  - Text: Pass directly to Claude
  - Image: Include as vision content block with base64 data
  - Both: Image + user context combined in single message
- **Entity matching:** Calls backend `/artists/search` and `/venues/search` APIs
- **Response:** Returns structured data with `matched_id` populated for known entities

### 3. React Hook
**File:** `/frontend/lib/hooks/useShowExtraction.ts`

TanStack Query mutation hook wrapping the API call:

```typescript
const { mutate, isPending, error, data } = useShowExtraction()

// Extract from text only
mutate({ type: 'text', text: 'The National at Valley Bar...' })

// Extract from image only
mutate({
  type: 'image',
  image_data: base64Data,
  media_type: 'image/jpeg'
})

// Extract from image with additional context
mutate({
  type: 'both',
  image_data: base64Data,
  media_type: 'image/jpeg',
  text: 'This is at Valley Bar in Phoenix, doors at 7pm'
})
```

### 4. UI Component
**File:** `/frontend/components/forms/AIFormFiller.tsx`

- Collapsible card (collapsed by default)
- **Unified view** with both image and text inputs visible:
  - Compact image drop zone at top (drag-and-drop + click to select)
  - Image preview with remove button when image is uploaded
  - Text area below for pasting show details or adding context to images
- Dynamic placeholder text (changes based on whether an image is uploaded)
- "Extract Show Info" button with loading spinner
- Success display with match indicators (badges show checkmark for existing entities)

---

## Files Modified

### 1. Submissions Page
**File:** `/frontend/app/submissions/page.tsx`

- Added `AIFormFiller` component above the form card
- Added state for extracted data
- Passes `onExtracted` callback to `AIFormFiller`
- Passes `initialExtraction` to `ShowForm`

### 2. Show Form
**File:** `/frontend/components/forms/ShowForm.tsx`

- Added `initialExtraction?: ExtractedShowData` prop
- Added `useEffect` to populate form when extraction data changes
- Uses `form.setFieldValue()` to set all fields
- Updates `selectedVenue` state for verified venue display

### 3. Forms Index
**File:** `/frontend/components/forms/index.ts`

- Added export for `AIFormFiller`

---

## Claude System Prompt

```
You are a show information extractor. Given text or an image of a show flyer, extract structured information.

Output ONLY valid JSON with no additional text or markdown formatting:
{
  "artists": [{"name": "Artist Name", "is_headliner": true}],
  "venue": {"name": "Venue Name", "city": "City", "state": "AZ"},
  "date": "YYYY-MM-DD",
  "time": "HH:MM",
  "cost": "$20",
  "ages": "21+"
}

Rules:
- First artist listed is usually the headliner (is_headliner: true), others are is_headliner: false
- Convert dates to YYYY-MM-DD format (assume current year if not specified)
- Convert time to 24-hour format (default to 20:00 if "doors" time is given but show time is ambiguous)
- State should be 2-letter abbreviation (default to AZ for Arizona venues)
- Omit fields if not found (don't include null or empty values)
- For cost, include the dollar sign if it's a paid show, or use "Free" if explicitly stated as free
- For ages, common formats are "21+", "18+", "All Ages"
- If multiple dates are shown, extract only the first/primary date
- Return ONLY the JSON object, no explanation or markdown code blocks
```

---

## Entity Matching Logic

For each extracted entity, the API searches the backend:

```typescript
// Search for artist match
const searchResult = await fetch(`${BACKEND_URL}/artists/search?q=${encodeURIComponent(name)}`)
const { artists } = await searchResult.json()

// Find exact match (case-insensitive)
const match = artists.find(a => a.name.toLowerCase() === name.toLowerCase())
if (match) {
  artist.matched_id = match.id
  artist.matched_name = match.name  // canonical casing
  artist.matched_slug = match.slug
}
```

Same logic applies for venues.

---

## UI Design

### Unified View (Image + Text)
```
┌─────────────────────────────────────────────────────────────┐
│ ✨ AI Form Filler-Outer                              [▼]       │
├─────────────────────────────────────────────────────────────┤
│  Upload a flyer image and/or paste show details...          │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ [🖼] Drop a flyer image here, or click to select      │  │
│  │     JPEG, PNG, GIF, WebP (max 10MB)                   │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ Paste show details, flyer text, or event info...     │  │
│  │                                                       │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  [Extract Show Info]                                        │
└─────────────────────────────────────────────────────────────┘
```

### With Image Uploaded
```
┌─────────────────────────────────────────────────────────────┐
│ ✨ AI Form Filler-Outer                              [▼]       │
├─────────────────────────────────────────────────────────────┤
│  ┌───────────────────────────────────────────────────────┐  │
│  │              [Image Preview]                    [X]   │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ Add any details the image might be missing...        │  │
│  │ (venue, time, price, etc.)                           │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  [Extract Show Info]                                        │
└─────────────────────────────────────────────────────────────┘
```

### Success State
```
┌─────────────────────────────────────────────────────────────┐
│  ┌───────────────────────────────────────────────────────┐  │
│  │ ✓ Extraction Complete                                │  │
│  │ Artists: [The National ✓] [Bartees Strange]          │  │
│  │ Venue: [Valley Bar ✓]                                │  │
│  │ Date: Feb 15, 2026 at 8:00 PM                        │  │
│  │ $35 / 21+                                            │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

Badges with checkmarks indicate entities matched to existing database records.

---

## Error Handling

| Scenario | Status | Response |
|----------|--------|----------|
| No API key configured | 503 | "AI service not configured" |
| User not authenticated | 401 | "Authentication required" |
| Invalid image type | 400 | "Invalid image type. Supported formats: ..." |
| Text too long (>10k chars) | 400 | "Text content exceeds maximum length" |
| Claude API error | 503 | "AI service error. Please try again." |
| JSON parse failure | 500 | "Failed to parse AI response" |
| API credits exhausted | 503 | "AI service temporarily unavailable" |

---

## Security & Limits

- **Auth:** Session cookie required (any authenticated user)
- **Image size:** 10MB client-side validation
- **Text length:** 10,000 characters max
- **Supported formats:** JPEG, PNG, GIF, WebP

---

## Environment Variables

Requires `ANTHROPIC_API_KEY` to be set in the frontend environment.

---

## Testing

1. Go to `/submissions`
2. Click "AI Form Filler-Outer" to expand

3. **Text-only test:**
   - Paste sample text in the textarea like:
     ```
     The National with Bartees Strange
     Valley Bar, Phoenix AZ
     Feb 15 8pm $35 21+
     ```
   - Click "Extract Show Info"
   - Verify form populates correctly
   - Check that Valley Bar shows checkmark if it exists in DB

4. **Image-only test:**
   - Drag a show flyer image into the drop zone (or click to select)
   - Click "Extract Show Info"
   - Verify extracted data appears and form populates

5. **Image + text test:**
   - Upload a flyer image
   - In the text area below, add context like:
     ```
     This is at Valley Bar in Phoenix, doors at 7pm, $25
     ```
   - Click "Extract Show Info"
   - Verify the text context is combined with image analysis

---

## Future Improvements

- [ ] Add rate limiting if abuse detected
- [ ] Cache recent extractions to avoid duplicate API calls
- [ ] Support extracting multiple shows from a single flyer
- [ ] Add confidence scores to extracted fields
- [ ] Allow user to correct/reject matches before applying
