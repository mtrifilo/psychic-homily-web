# Auth Hardening Checklist

Production readiness audit of frontend authentication flows. These issues were identified by auditing every auth-related hook, context, component, and page for race conditions, silent failures, 401 spam, and stuck UI states.

## Context: Infinite Spinner Bug (Fixed)

We already fixed an infinite spinner on logout when the session was expired. Root cause: `queryClient.clear()` in global error handlers caused a cascade — every active query refetched, got 401, triggered another clear, ad infinitum.

**Fix applied (2 files):**
- `lib/queryClient.ts` — Replaced `queryClient.clear()` in `QueryCache.onError` / `MutationCache.onError` with targeted `invalidateQueries` on the profile query only, guarded by `profileState?.status !== 'error'`
- `lib/hooks/useAuth.ts` — Logout `mutationFn` now catches 401/TOKEN_MISSING and returns `{ success: true }` instead of throwing (session already gone = successful logout)

---

## HIGH Priority — Causes visible user-facing problems

### 1. `useIsShowSaved` fires for logged-out users (401 spam)
- **File:** `lib/hooks/useSavedShows.ts:56`
- **Problem:** `enabled: Boolean(showId)` only checks showId, not auth status. `SaveButton` calls `useSaveShowToggle(showId)` which calls this hook *before* the `if (!isAuthenticated) return null` guard at line 27 of `SaveButton.tsx`. React hooks run unconditionally, so every show card on the public shows page fires `/saved-shows/{id}/check` for logged-out visitors. This is the source of mass 401 errors visible in the browser console.
- **Fix:** Accept an `isAuthenticated` param (or get it from context) and add `enabled: Boolean(showId) && isAuthenticated` to `useIsShowSaved`. Thread the auth state through `useSaveShowToggle` and `SaveButton`.
- [x] Done

### 2. `useSavedShows` and `useMySubmissions` lack `enabled` guards
- **Files:** `lib/hooks/useSavedShows.ts:30-38`, `lib/hooks/useMySubmissions.ts:27-35`
- **Problem:** Neither hook has an `enabled` parameter. They fire immediately when mounted. These are on auth-gated pages (collection), but the queries execute before the page's auth redirect kicks in, causing a flash of 401 errors on page load.
- **Fix:** Add `enabled` parameter driven by auth state. Could accept `enabled` as an option, or check auth internally.
- [x] Done

### 3. `useFavoriteVenues` and `useFavoriteVenueShows` lack `enabled` guards
- **Files:** `lib/hooks/useFavoriteVenues.ts`
- **Problem:** Same pattern as #2. `useFavoriteVenues()` and `useFavoriteVenueShows()` fire unconditionally. If used on public pages (e.g., venue detail), they produce 401 spam for logged-out users.
- **Fix:** Add `enabled` parameter driven by auth state.
- [x] Done

### 4. Default `mutations.retry: 1` retries all mutations including auth
- **File:** `lib/queryClient.ts:34-37`
- **Problem:** Every mutation auto-retries once on failure. This includes login, register, password change, and account deletion. If login returns 500 briefly during a rolling deploy, the user sees one loading state but two backend requests fire. For idempotent reads this is fine, but auth mutations shouldn't silently retry.
- **Fix:** Change default `mutations.retry` to 0 (or use a retry function that skips 4xx like the query retry does). Individual mutations can opt into retry if needed.
- [x] Done

### 5. `isSessionExpiredError` doesn't detect `TOKEN_MISSING` in raw errors
- **File:** `lib/queryClient.ts:47-52`
- **Problem:** The `AuthError` path correctly uses `shouldRedirectToLogin` (which includes TOKEN_MISSING), but the raw-error fallback path only checks `TOKEN_EXPIRED` and `TOKEN_INVALID`. If a non-AuthError object with `code: 'TOKEN_MISSING'` reaches this function, the session expiry won't be detected and the user stays in a stale "logged in" state.
- **Fix:** Add `AuthErrorCode.TOKEN_MISSING` to the raw error checks.
- [x] Done

### 6. Passkey login and magic link don't set `is_admin` on user
- **Files:** `components/auth/passkey-login.tsx:72-79`, `app/auth/magic-link/page.tsx:26-32`
- **Problem:** Both set user data from the response but omit `is_admin`. After passkey or magic-link login, the Admin nav link won't appear until the profile query refetches (up to 5 minutes with current staleTime). The user thinks they're not an admin.
- **Fix:** Add `is_admin: data.user.is_admin` to both `setUser` calls.
- [x] Done

---

## MEDIUM Priority — Edge cases that could confuse users

### 7. Admin hooks fire without `enabled` guards
- **Files:** `lib/hooks/useAdminShows.ts` (`usePendingShows`, `useRejectedShows`), `lib/hooks/useAdminVenues.ts` (`useUnverifiedVenues`)
- **Problem:** No `enabled` guards. The admin layout redirects non-admins, but queries fire first, producing wasted 403 requests. Minor since only admin users reach these pages, but still wasteful during the brief redirect window.
- **Fix:** Accept an `enabled` option or check admin status internally.
- [x] Done

### 8. `AuthContext.setUser` doesn't clear error state
- **File:** `lib/context/AuthContext.tsx:87-89`
- **Problem:** `setUser` only sets `userOverride` but doesn't clear `errorOverride`. If the previous auth attempt showed an error and then the user successfully logs in via passkey (which calls `setUser`), the error could persist alongside the logged-in state until the profile query clears it.
- **Fix:** Add `setErrorOverride(null)` inside `setUser`.
- [x] Done

---

## Verification

After implementing all fixes:
1. `cd frontend && bun run build` — no build errors
2. Browse `/shows` while logged out — no 401 errors in console
3. Log in via passkey as admin — Admin nav link appears immediately
4. Log in via magic link as admin — Admin nav link appears immediately
5. Let session expire while on `/shows` — profile invalidates cleanly, no cascade
6. Click logout with expired session — no infinite spinner, clean redirect
7. Visit `/collection` while logged out — clean redirect, no 401 flash
8. Visit `/admin` as non-admin — clean redirect, no 403 requests
