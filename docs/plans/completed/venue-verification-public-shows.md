# Venue Verification & Public Shows

**Date:** 2026-01-30
**Updated:** 2026-01-31
**Status:** Implemented

## Overview

This document describes the updated venue verification system that allows shows from unverified venues to be publicly visible while protecting venue privacy by displaying city-only locations.

## Problem Statement

Previously, shows submitted with new/unverified venues were hidden from the public (status: `pending`) until an admin approved them. This created friction for users submitting legitimate shows and bottlenecked show visibility on admin availability.

## Solution

Shows from unverified venues now go directly public with a **city-only address display**. Full address and map are revealed only after admin verification.

### Key Behavior Changes

| Before | After |
|--------|-------|
| Unverified venue → Show status `pending` → Hidden from public | Unverified venue → Show status `approved` → Public with city-only |
| Required admin approval before visibility | No approval needed; address revealed after verification |
| Users saw "Pending Review" warning | Users see "New Venue" info explaining city-only display |

## Implementation Details

### Backend Changes

**File:** `/backend/internal/services/show.go`

The `determineShowStatus()` function was simplified:

```go
func (s *ShowService) determineShowStatus(tx *gorm.DB, venues []CreateShowVenue, isAdmin bool, isPrivate bool) models.ShowStatus {
    // Private shows stay private regardless of venue status
    if isPrivate {
        return models.ShowStatusPrivate
    }
    // All other shows are approved - unverified venues will show city-only on frontend
    return models.ShowStatusApproved
}
```

The `PublishShow()` function was also updated to always set `approved` status (no longer sets `pending` for unverified venues).

**New Endpoint:** `GET /admin/venues/unverified`

Added endpoint for admins to list and verify unverified venues independently of show approval.

### Frontend Changes

#### 1. Show Submission Form (`ShowForm.tsx`)
- Blue info box for new venues
- Text: "Your show will be published with city-only location until the venue is verified"
- Removed dead code for pending status redirect

#### 2. Venue Location Card (`VenueLocationCard.tsx`)
- `verified` prop controls display mode
- **Verified:** Full address, embedded map, directions button
- **Unverified:** City/state only with info tooltip

#### 3. Success Dialog (`SubmissionSuccessDialog.tsx`)
- Simplified to only handle private show submissions
- Removed dead pending status code

#### 4. Admin Console (`/admin`)
- **New Tab:** "Unverified Venues" - lists venues awaiting verification
- Each venue shows name, city/state, show count, and creation date
- "Verify Venue" button with confirmation dialog
- Badge count shows number of unverified venues

#### 5. Collection Page (`/collection`)
- Simplified dialog handling (private shows only)
- Updated comments to reflect new behavior
- Kept pending status badge for legacy data compatibility

### Display Rules

| Component | Verified Venue | Unverified Venue |
|-----------|---------------|------------------|
| VenueLocationCard | Full address + map + directions | City, State + tooltip |
| Show list cards | City, State (unchanged) | City, State (unchanged) |
| VenueCard | City, State (unchanged) | City, State (unchanged) |
| Admin views | Full details | Full details |

## Files Modified

### Backend
- `/backend/internal/services/show.go` - Status determination logic, PublishShow simplified
- `/backend/internal/services/venue.go` - Added GetUnverifiedVenues method
- `/backend/internal/api/handlers/admin.go` - Added GetUnverifiedVenuesHandler
- `/backend/internal/api/handlers/show.go` - Updated PublishShowHandler comment
- `/backend/internal/api/routes/routes.go` - Added unverified venues route

### Frontend
- `/frontend/components/forms/ShowForm.tsx` - Simplified submission flow
- `/frontend/components/SubmissionSuccessDialog.tsx` - Simplified to private shows only
- `/frontend/components/VenueLocationCard.tsx` - Conditional address display
- `/frontend/components/VenueDetail.tsx` - Pass verified prop
- `/frontend/app/admin/page.tsx` - Added Unverified Venues tab
- `/frontend/app/admin/unverified-venues/page.tsx` - New admin page for venue verification
- `/frontend/app/collection/page.tsx` - Simplified dialog handling
- `/frontend/lib/api.ts` - Added UNVERIFIED endpoint
- `/frontend/lib/queryClient.ts` - Added unverifiedVenues query key
- `/frontend/lib/types/venue.ts` - Added UnverifiedVenue type
- `/frontend/lib/hooks/useAdminVenues.ts` - Added useUnverifiedVenues hook
- `/frontend/lib/hooks/useShowPublish.ts` - Updated comment

## Admin Workflow

### Verifying Venues

1. Go to Admin Console
2. Click "Unverified Venues" tab
3. Review venue details (name, address, city/state, show count)
4. Click "Verify Venue" button
5. Confirm in dialog
6. Venue is now verified; full address displays on all associated shows

### Finding Unverified Venues

Admins can find unverified venues via:
- **Unverified Venues tab** - Dedicated list sorted by creation date
- **Pending Shows tab** - Shows with unverified venues display "Unverified Venue" badge (for legacy pending shows)

## Testing Checklist

- [x] Submit show with new venue → Verify show appears publicly immediately
- [x] View show publicly → Verify only city/state displayed for venue
- [x] View venue detail page → Verify city-only with tooltip (no map)
- [x] Admin verifies venue → Verify full address and map now display
- [x] Submit show with verified venue → Verify full address displays
- [x] Submit private show → Verify it stays private (not public)
- [x] Admin submissions → Verify unchanged behavior (always approved)
- [x] Admin Unverified Venues tab → Shows unverified venues with verify button

## Rollback Plan

If issues arise, revert the `determineShowStatus()` and `PublishShow()` functions to check venue verification:

```go
// Revert to this if needed
if hasUnverifiedVenue {
    if isPrivate {
        return models.ShowStatusPrivate
    }
    return models.ShowStatusPending
}
```

Frontend changes are cosmetic and can remain even if backend is reverted, though the admin Unverified Venues tab would become the primary way to verify venues.

## Future Considerations

1. **Venue verification request flow**
   - Allow users to request verification for venues they submitted
   - Add a "Request Verification" button on unverified venue pages
   - Notification to admins when verification requested

2. **Abuse prevention**
   - Monitor for spam submissions now that shows go public immediately
   - Consider rate limiting show submissions per user
   - Add reporting mechanism for fake/spam venues

3. **Analytics**
   - Track verified vs unverified venue show submissions
   - Monitor time-to-verification metrics
