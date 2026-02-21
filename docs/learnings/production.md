# Production Learnings

Reference for agents working on security, deployment, or production issues.

## Security Fixes Applied (2026-02-12)

- **GetShows status filter**: `GetShows()` in `show.go` now filters `status = 'approved'` — was leaking pending/rejected shows on public `/shows` endpoint and sitemap
- **Inactive user JWT check**: `ValidateToken()` and `ValidateTokenLenient()` in `jwt.go` now check `user.IsActive` after DB lookup — deactivated users with valid JWTs (24h window) were able to continue using the app
- **Ownership verification**: Confirmed solid — all user-scoped endpoints use `WHERE user_id = ?` from JWT context
- **Error response sanitization**: 27 response-facing `err.Error()` calls replaced with generic messages across 8 handler files — details logged server-side only, clients get `request_id` for correlation

## Performance Fixes

- **N+1 fix**: `UpdateShowWithRelations` venue/artist fetches batched with `WHERE id IN` + map lookup (was N+1 per venue/artist)

## Frontend Fixes

- **Error boundaries**: `error.tsx` added to `shows/`, `venues/`, `artists/`, `admin/`, `collection/` — Sentry + retry button
- **Console.log cleanup**: 13 `console.log` calls removed from `discover-music/route.ts` (incl. API key debug log)
- **Accessibility**: `role="alert"` added to 22 validation/error elements across 9 files; `aria-invalid` added to FormField textarea

## Chrome Dark Background Rendering Fix

- Near-black `background-color` on html/body causes visible rectangle artifacts in Chrome due to GPU compositor tile boundaries
- **Fix**: Use `html::before { content: ''; position: fixed; inset: 0; z-index: -1; background: var(--background); }` — creates a single full-viewport compositor layer that bypasses tile boundaries
- Discovery app colors were also converted from oklch to hex during debugging (keep hex)
- `darkMode: 'class'` added to discovery `tailwind.config.js`
