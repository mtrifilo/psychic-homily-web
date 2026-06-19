// Global navigation-style preference (PSY-1116). The user chooses between the
// default top-bar nav and a left side-nav; the choice is stored in a cookie so
// the shell (AppShell, a Server Component) can resolve it at SSR with no layout
// flash. The cookie name + parser are the single source of truth shared by the
// server shell here and the settings toggle (PSY-1117).
//
// Server-and-client safe: no `next/headers` import, so both the server shell
// (reads via cookies()) and client code (reads/writes document.cookie) use it.

export const NAV_MODE_COOKIE = 'nav_mode'

export type NavMode = 'top' | 'side'

export const DEFAULT_NAV_MODE: NavMode = 'top'

/**
 * Coerce an arbitrary cookie value to a valid NavMode. Anything other than the
 * explicit 'side' opt-in (missing, malformed, or a stale value) resolves to the
 * top-bar default — the column-level contract mirrors the backend's
 * users.nav_mode CHECK (PSY-1115).
 */
export function parseNavMode(value: string | undefined | null): NavMode {
  return value === 'side' ? 'side' : DEFAULT_NAV_MODE
}

// One year. The cookie is a write-through cache of the account preference, not
// a session value, so it should outlive the browser session — long enough that
// a returning visitor (even logged out) keeps their last-chosen nav.
export const NAV_MODE_MAX_AGE_SECONDS = 60 * 60 * 24 * 365

/**
 * Persist the nav-mode choice to the `nav_mode` cookie from client code. For an
 * authenticated viewer the account preference wins at SSR (see AppShell), so
 * this cookie is the continuity value for the logged-out/anonymous view on this
 * browser and the fallback the shell uses when the account read is unavailable.
 * Client-only — it touches `document`, so only call it from an event handler /
 * effect, never at module load (this module is also imported by the server shell).
 *
 * Attributes: `path=/` so every route's shell sees it; `SameSite=Lax` (a
 * first-party UI preference; Lax still rides top-level navigations); `Secure`
 * over HTTPS only — production is HTTPS, but a Secure cookie set via
 * `document.cookie` over plain-HTTP dev (a LAN IP, not localhost) would be
 * silently dropped, so gate it on the actual protocol. Not `HttpOnly` by
 * design: the client writes it and the server reads it.
 */
export function setNavModeCookie(mode: NavMode): void {
  const secure =
    typeof location !== 'undefined' && location.protocol === 'https:'
      ? '; Secure'
      : ''
  document.cookie = `${NAV_MODE_COOKIE}=${mode}; path=/; max-age=${NAV_MODE_MAX_AGE_SECONDS}; SameSite=Lax${secure}`
}
