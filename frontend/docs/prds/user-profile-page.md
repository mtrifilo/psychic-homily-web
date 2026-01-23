# User Profile Page

## Summary

Add a dedicated user profile page where logged-in users can manage their account settings, update their personal information, and change their password. The email address in the navigation becomes clickable, linking to this profile page. This also involves refactoring the Collections page to focus purely on content collections (saved shows, submissions) rather than settings.

**Status:** Planning
**Priority:** High
**Estimated Scope:** Medium (1-2 days)

---

## Problem Statement

### Current State

- Users see their email in the top-right navigation, but it's not interactive
- The Collections page (`/collection`) mixes content (saved shows, submissions) with account settings
- Settings currently only show email verification status with a placeholder for future settings
- User preferences exist in the backend (`UserPreferences` model) but aren't exposed to the frontend
- No way for users to update their profile (username, name, bio) or change their password

### Pain Points

- Users have no dedicated place to manage their account
- Mixing collections and settings creates confusion about what the Collections page is for
- Backend has UserPreferences ready (timezone, notifications, theme, language) but frontend can't access them
- No password change functionality exists

### Success Criteria

- [ ] Users can access their profile by clicking their email/username in navigation
- [ ] Users can update their profile information (username, first name, last name, bio)
- [ ] Users can change their password with email verification
- [ ] Users can manage preferences (timezone, notifications, theme)
- [ ] Collections page focuses on content only (no settings tab)
- [ ] Settings in profile page include everything from the old Collections settings tab

---

## User Stories

1. **As a logged-in user**, I want to click on my email in the navigation so that I can access my account settings
2. **As a user**, I want to set a username so that I can have a display name other than my email
3. **As a user**, I want to change my password so that I can keep my account secure
4. **As a user**, I want to set my timezone so that show times display correctly for my location
5. **As a user**, I want to control notification preferences so that I only receive alerts I want
6. **As a user**, I want to see my saved shows and submissions in Collections without settings clutter

---

## Requirements

### Functional Requirements

#### Must Have (MVP)

**Profile Page (`/profile`):**
- [ ] Display current user information (email, username, name, avatar)
- [ ] Allow editing username (with uniqueness validation)
- [ ] Allow editing first name and last name
- [ ] Allow editing bio
- [ ] Show email verification status (move from Collections settings)
- [ ] Send verification email button (if not verified)

**Password Change:**
- [ ] "Change Password" section in profile
- [ ] Require current password to change password
- [ ] New password + confirm password fields
- [ ] Password strength requirements (min 8 chars)
- [ ] Email notification sent when password is changed

**Navigation Update:**
- [ ] Make user email/username clickable in navigation
- [ ] Link to `/profile`
- [ ] Show username if set, otherwise show email

**Collections Page Refactor:**
- [ ] Remove "Settings" tab from Collections page
- [ ] Keep only "Saved Shows" and "My Submissions" tabs
- [ ] Add subtle link to Profile page for settings (e.g., "Manage account settings")

#### Should Have

**User Preferences:**
- [ ] Timezone selector (dropdown of common timezones)
- [ ] Email notification toggle
- [ ] Theme preference (light/dark/system) - integrate with existing mode-toggle

#### Could Have (Future)

- [ ] Avatar upload
- [ ] Push notification toggle (requires push notification infrastructure)
- [ ] Language preference
- [ ] Delete account functionality
- [ ] Connected accounts management (view/disconnect OAuth accounts)
- [ ] Activity log (recent logins, password changes)

### Non-Functional Requirements

- **Performance:** Profile page loads under 1 second
- **Accessibility:** All form fields have proper labels, keyboard navigation works
- **Security:** Password change requires current password verification, sends email notification
- **Validation:** Username uniqueness checked on blur (debounced), clear error messages

---

## Technical Context

### Relevant Codebase Areas

**Backend:**
- `backend/internal/models/user.go` - User and UserPreferences models (already has all needed fields)
- `backend/internal/services/user.go` - User service with UpdateUser, password methods
- `backend/internal/services/auth.go` - Auth service
- `backend/internal/api/handlers/auth.go` - Auth handlers (profile endpoint exists)
- `backend/internal/api/routes/routes.go` - Route definitions

**Frontend:**
- `frontend/app/nav.tsx` - Navigation component showing user email (line ~85-95)
- `frontend/app/collection/page.tsx` - Collections page with Settings tab to refactor
- `frontend/components/SettingsPanel.tsx` - Current settings panel to migrate/repurpose
- `frontend/lib/context/AuthContext.tsx` - Auth context with user state
- `frontend/lib/hooks/useAuth.ts` - Auth hooks (useProfile, etc.)
- `frontend/lib/types/user.ts` - User types (may need to add preferences)

**Database:**
- Table: `users` - Has all profile fields ready
- Table: `user_preferences` - Has timezone, theme, notification fields (not yet exposed via API)

### Existing Patterns to Follow

- For protected pages, see: `frontend/app/collection/page.tsx` (Suspense boundary, auth redirect)
- For form handling, see: `frontend/app/auth/page.tsx` (login/register forms with validation)
- For API mutations, see: `frontend/lib/hooks/useAuth.ts` (useMutation pattern)
- For settings UI, see: `frontend/components/SettingsPanel.tsx` (card-based sections)
- For tabs, see: `frontend/app/collection/page.tsx` (shadcn Tabs with URL state)

### Dependencies

- **shadcn/ui:** Form components (Input, Button, Label, Card, Tabs, Select)
- **TanStack Query:** Mutations for profile updates
- **bcrypt (backend):** Password hashing

---

## Proposed Solution

### Overview

Create a new `/profile` route that serves as the central hub for account management. The navigation will link the user's email/username to this page. The Collections page will be simplified to focus on content collections only, with a small link pointing users to the profile page for account settings.

### Key Components

1. **ProfilePage (`/profile`):** Main profile page with sections for user info, password, and preferences
2. **ProfileForm:** Editable form for user details (username, name, bio)
3. **PasswordChangeForm:** Secure password change with current password verification
4. **PreferencesForm:** Timezone, notifications, theme toggles
5. **Updated Navigation:** Clickable user identifier linking to profile

### Data Flow

```
User clicks email in nav → /profile page loads
                              ↓
                         GET /auth/profile (existing)
                              ↓
                         Display user info + preferences
                              ↓
User edits field → PUT /users/me → Update user in DB
                              ↓
                         Invalidate profile query → UI updates
```

---

## API Design

### New Endpoints

#### `PUT /users/me`

**Description:** Update the authenticated user's profile information

**Request:**
```json
{
  "username": "newusername",
  "first_name": "John",
  "last_name": "Doe",
  "bio": "Music lover from Phoenix"
}
```

**Response:**
```json
{
  "id": 123,
  "email": "user@example.com",
  "username": "newusername",
  "first_name": "John",
  "last_name": "Doe",
  "bio": "Music lover from Phoenix",
  "avatar_url": null,
  "email_verified": true,
  "created_at": "2024-01-15T...",
  "updated_at": "2024-01-20T..."
}
```

**Error Responses:**
| Status | Condition |
|--------|-----------|
| 400 | Invalid input (username too short, invalid characters) |
| 409 | Username already taken |
| 401 | Not authenticated |

---

#### `PUT /users/me/password`

**Description:** Change the authenticated user's password

**Request:**
```json
{
  "current_password": "oldpassword123",
  "new_password": "newpassword456"
}
```

**Response:**
```json
{
  "message": "Password updated successfully"
}
```

**Error Responses:**
| Status | Condition |
|--------|-----------|
| 400 | New password doesn't meet requirements |
| 401 | Current password is incorrect |
| 403 | OAuth-only account (no password set) |

**Side Effects:**
- Sends email notification: "Your password was changed"
- Optionally: Invalidate other sessions (future enhancement)

---

#### `GET /users/me/preferences`

**Description:** Get the authenticated user's preferences

**Response:**
```json
{
  "notification_email": true,
  "notification_push": false,
  "theme": "system",
  "timezone": "America/Phoenix",
  "language": "en"
}
```

---

#### `PUT /users/me/preferences`

**Description:** Update the authenticated user's preferences

**Request:**
```json
{
  "notification_email": true,
  "timezone": "America/Phoenix",
  "theme": "dark"
}
```

**Response:** Same as GET response with updated values

---

#### `GET /users/check-username/:username`

**Description:** Check if a username is available (for real-time validation)

**Response:**
```json
{
  "available": true
}
```
or
```json
{
  "available": false,
  "message": "Username is already taken"
}
```

---

## UI/UX Design

### Profile Page Wireframe

```
┌─────────────────────────────────────────────────────────────┐
│  ← Back                                     [Navigation]    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Profile Settings                                           │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  ACCOUNT INFORMATION                                │    │
│  │                                                     │    │
│  │  Email          user@example.com    ✓ Verified      │    │
│  │                 [Resend verification] (if needed)   │    │
│  │                                                     │    │
│  │  Username       [________________]  ✓ Available     │    │
│  │                                                     │    │
│  │  First Name     [________________]                  │    │
│  │                                                     │    │
│  │  Last Name      [________________]                  │    │
│  │                                                     │    │
│  │  Bio            [________________]                  │    │
│  │                 [________________]                  │    │
│  │                                                     │    │
│  │                              [Save Changes]         │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  CHANGE PASSWORD                                    │    │
│  │                                                     │    │
│  │  Current Password    [________________]             │    │
│  │                                                     │    │
│  │  New Password        [________________]             │    │
│  │                                                     │    │
│  │  Confirm Password    [________________]             │    │
│  │                                                     │    │
│  │                           [Update Password]         │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  PREFERENCES                                        │    │
│  │                                                     │    │
│  │  Timezone        [America/Phoenix        ▼]         │    │
│  │                                                     │    │
│  │  Email Notifications    [Toggle: ON]                │    │
│  │  Receive emails about saved shows and updates       │    │
│  │                                                     │    │
│  │  Theme                  ○ Light ○ Dark ● System     │    │
│  │                                                     │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Navigation Update

```
Before:
┌──────────────────────────────────────────────────────────────┐
│  [Logo]  Shows  Venues  Submit     user@example.com [Logout] │
└──────────────────────────────────────────────────────────────┘

After:
┌──────────────────────────────────────────────────────────────┐
│  [Logo]  Shows  Venues  Submit     [user@example.com →] [⎋]  │
└──────────────────────────────────────────────────────────────┘
                                      ↑ clickable, links to /profile
```

Or with username set:
```
┌──────────────────────────────────────────────────────────────┐
│  [Logo]  Shows  Venues  Submit          [@username →] [⎋]    │
└──────────────────────────────────────────────────────────────┘
```

### Refactored Collections Page

```
┌─────────────────────────────────────────────────────────────┐
│  My Collection                                              │
│                                                             │
│  [Saved Shows] [My Submissions]                             │
│   ^^^^^^^^^^^                                               │
│  ┌─────────────────────────────────────────────────────┐    │
│  │                                                     │    │
│  │  [Show cards...]                                    │    │
│  │                                                     │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  ─────────────────────────────────────────────────────────  │
│  Looking for account settings? [Go to Profile →]            │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### User Flow

1. User logs in and sees their email in the navigation
2. User clicks on their email → navigates to `/profile`
3. User sees their current profile information pre-filled
4. User updates their username → real-time availability check
5. User clicks "Save Changes" → success toast, data persisted
6. User changes password → enters current + new password → success message + email sent
7. User updates timezone → preference saved, show times now display in their timezone

### States

- **Loading:** Skeleton loaders for profile sections while data loads
- **Empty:** N/A (user always has basic profile data)
- **Error:** Inline error messages below fields, toast for server errors
- **Success:** Toast notification "Profile updated" / "Password changed"
- **Validating:** Spinner next to username field while checking availability

---

## Database Changes

No schema changes needed - the existing `users` and `user_preferences` tables have all required fields:

**Existing `users` fields to expose:**
- `username` (already exists, nullable, unique)
- `first_name` (already exists)
- `last_name` (already exists)
- `bio` (already exists)
- `avatar_url` (already exists, future use)

**Existing `user_preferences` fields to expose:**
- `notification_email` (boolean, default true)
- `notification_push` (boolean, default false)
- `theme` (string, default 'light')
- `timezone` (string, default 'UTC')
- `language` (string, default 'en')

---

## Implementation Notes

### Approach Preferences

- Use shadcn/ui Card components for each settings section (matches existing SettingsPanel style)
- Follow the mutation pattern in `useAuth.ts` for profile updates
- Username validation should be debounced (300ms) and show inline status
- Password change form should be a separate component with its own submit handler
- Theme preference should integrate with existing `mode-toggle.tsx` (don't duplicate logic)
- Timezone selector should include common US timezones at the top, then alphabetical list

### Known Challenges

- **Username uniqueness:** Need real-time validation without hammering the API (debounce)
- **OAuth users and password:** OAuth-only users don't have a password - hide the password change section or show "Set a password" option
- **Theme sync:** The mode-toggle already handles theme; preferences should sync with it, not replace it
- **Email change:** Intentionally out of scope (complex security implications) - email shown as read-only

### Testing Strategy

- [ ] Unit tests for username validation logic
- [ ] Unit tests for password strength validation
- [ ] Integration tests for profile update endpoints
- [ ] Manual testing for OAuth user edge case (no password section)
- [ ] Manual testing for email verification flow from profile page

---

## Out of Scope

- Email address changes (security complexity, verification flow)
- Avatar/profile picture upload (requires file storage infrastructure)
- Account deletion (needs careful data handling, confirmation flow)
- Two-factor authentication (significant security feature)
- Connected accounts management (viewing/disconnecting OAuth accounts)
- Session management (viewing active sessions, logging out other devices)
- Push notifications (infrastructure not in place)

---

## Open Questions

1. **Should username have format restrictions?**
   - Suggested: alphanumeric + underscores, 3-30 chars, no spaces
   - Alternative: Allow more characters but sanitize for display

2. **What happens when an OAuth user wants to set a password?**
   - Option A: Show "Set a password" instead of "Change password" (requires new endpoint)
   - Option B: Hide password section entirely for OAuth-only users
   - Recommendation: Option B for MVP, Option A as future enhancement

3. **Should timezone preference automatically update show display times?**
   - Currently shows use America/Phoenix hardcoded
   - Need to evaluate if this should be user-preference driven
   - Recommendation: Yes, use user's timezone preference when displaying show times

4. **Should we show a "Danger Zone" section for future destructive actions?**
   - Delete account, disconnect OAuth, etc.
   - Recommendation: Add the UI section with disabled/coming-soon state for future

---

## References

- [Artist Pages Design](../artist-pages-design.md) - Example of detail page structure
- [shadcn/ui Form Components](https://ui.shadcn.com/docs/components/form)
- [TanStack Query Mutations](https://tanstack.com/query/latest/docs/react/guides/mutations)
- [OWASP Password Guidelines](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)

---

## Changelog

| Date | Author | Changes |
|------|--------|---------|
| 2026-01-22 | Claude | Initial draft |
