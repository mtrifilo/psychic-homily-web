# User Profile Page Implementation

## Summary
This document describes the implementation of a new User Profile page that consolidates user account management, moving settings out of the My Collection page.

## Motivation
- Separate concerns: My Collection is for content (saved shows, submissions), not account settings
- Better UX: Users can access their profile by clicking their email in the navigation
- Extensible: Profile tab serves as a placeholder for future profile editing features

## Architecture

### New Route: `/profile`
A new page with two tabs:
- **Profile** (default): Displays user information (email, name), placeholder for future editing
- **Settings**: Email verification, password change (reuses `SettingsPanel` component)

### Navigation Change
User email in the header is now clickable, linking to `/profile`:
- Desktop: Email displayed in the authenticated section (right side of nav)
- Mobile: "Signed in as {email}" text in mobile menu

### URL-based Tab Navigation
Follows the same pattern as `/collection`:
- `/profile` → Profile tab (default)
- `/profile?tab=settings` → Settings tab

## Key Files

### Created
- `/frontend/app/profile/page.tsx` - New profile page component

### Modified
- `/frontend/app/nav.tsx` - Email now links to `/profile`
- `/frontend/app/collection/page.tsx` - Removed Settings tab
- `/frontend/app/verify-email/page.tsx` - Updated links to `/profile?tab=settings`
- `/frontend/app/submissions/page.tsx` - Updated link to `/profile?tab=settings`

### Reused (no changes)
- `/frontend/components/SettingsPanel.tsx` - Settings UI
- `/frontend/components/settings/change-password.tsx` - Password change form
- `/frontend/components/settings/passkey-management.tsx` - Passkey management

## Component Structure

```
/profile/page.tsx
├── ProfilePageContent (client component)
│   ├── Auth guard (redirect to /auth if not authenticated)
│   ├── Header (icon + "My Profile" title + email)
│   └── Tabs
│       ├── Profile tab → ProfileTab component (placeholder)
│       └── Settings tab → SettingsPanel component (existing)
└── Suspense wrapper with loading fallback
```

## Authentication
Same pattern as collection page:
- Uses `useAuthContext()` hook
- Shows loading spinner while checking auth
- Redirects to `/auth` if not authenticated

## Future Enhancements
The Profile tab is a placeholder ready for:
- Profile editing (name, avatar)
- Account deletion
- Notification preferences
- Connected accounts
