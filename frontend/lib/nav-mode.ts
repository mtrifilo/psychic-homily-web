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
