import { cookies } from 'next/headers'
import { TopBar } from './TopBar'
import { CommandPalette } from './CommandPalette'
import { SideNavShell } from './SideNavShell'
import { NAV_MODE_COOKIE, parseNavMode } from '@/lib/nav-mode'
import { getAuthenticatedNavMode } from '@/lib/auth-hydration'
import { BottomTabBar } from './nav/BottomTabBar'

// The global application shell (PSY-1013 top-bar nav; PSY-1116 nav-mode toggle).
// Resolves the user's nav-style preference at SSR and renders one of two
// compositions:
//   • 'top'  (default) — the top bar owns global nav; content renders full-width.
//   • 'side' — a SLIM top bar (no PrimaryNav) above the revived left Sidebar.
//
// Precedence (PSY-1117): the authenticated account preference wins, then the
// `nav_mode` cookie, then the top default. For an authenticated viewer the
// account read normally resolves to a concrete value (column default 'top'),
// so the cookie is only the resolution path for anonymous/logged-out viewers —
// and the fallback when the account read fails (backend outage → undefined →
// cookie). The
// account read is what makes the preference cross-device with no flash — a
// logged-in viewer on a brand-new browser (no cookie yet) still gets their
// saved nav on first paint, and the settings toggle's post-save router.refresh()
// flips the chrome by re-reading the account, not the cookie.
// getAuthenticatedNavMode() shares the AuthHydrator prefetch's React.cache(), so
// this adds no extra backend fetch.
//
// Reading these here makes only the per-request shell dynamic; pages keep their
// own cache modes (same pattern as the auth-hydration + geo shell reads — see
// lib/geo-default.ts). The shell already renders inside the root layout's
// <Suspense> boundary alongside the cookie-reading AuthHydrator.
//
// Below `xl` the BottomTabBar is the primary nav (PSY-1020) in BOTH nav modes —
// the nav-mode preference is desktop chrome, and the bar is xl:hidden (paired
// with PrimaryNav's xl:flex). The shell's bottom padding (bar height + iOS
// safe-area inset) keeps page content and the footer clear of the fixed bar,
// and collapses at `xl` where the bar disappears.
//
// Order matters: the skip-to-content link is the first focusable element (jumps
// keyboard users past the banner/nav straight to <main id="main-content">, set
// in app/layout.tsx). The CommandPalette is mounted once here so the global ⌘K
// shortcut works on every route.
export async function AppShell({ children }: { children: React.ReactNode }) {
  const [accountNavMode, cookieStore] = await Promise.all([
    getAuthenticatedNavMode(),
    cookies(),
  ])
  const navMode = parseNavMode(
    accountNavMode ?? cookieStore.get(NAV_MODE_COOKIE)?.value
  )

  return (
    // LANDSCAPE SAFE AREA (PSY-1820). viewport-fit=cover hands the page the
    // full display, including the notch / rounded-corner band iOS used to
    // letterbox away. In landscape that band is ~44-59px, and the app's widest
    // gutter anywhere is px-4/md:px-8 — so without an inset the leading and
    // trailing edge of every page would render under the notch. Insetting the
    // shell covers all in-flow content at once: page containers, the sticky
    // TopBar, and the Footer. In portrait both insets are 0, so this is a
    // no-op on the common case.
    //
    // Deliberately NOT on `body`: Radix mounts react-remove-scroll for every
    // dialog, sheet, popover, and dropdown menu, which injects an UNLAYERED
    // `body[data-scroll-locked] { padding-left/right }` rule. Unlayered
    // declarations beat anything in @layer base, so a body-level inset would
    // silently collapse to 0 the moment any overlay opened — including this
    // shell's own Browse sheet — and the page behind it would jump sideways.
    //
    // It also does not reach position:fixed descendants, which resolve against
    // the viewport rather than this box; BottomTabBar, the cookie banner, and
    // the skip link below each carry their own env() padding. The remaining
    // fixed and full-bleed surfaces (side sheets, portaled Radix popovers, the
    // /atlas map) are tracked in PSY-1824.
    //
    // Trade-off accepted: in-flow chrome that paints edge-to-edge is inset too,
    // so TopBar's border-b and Footer's border-t stop at the safe area rather
    // than the physical screen edge in landscape. Content under the notch is
    // the worse failure.
    <div className="flex min-h-screen flex-col pb-[calc(var(--bottom-tab-bar-height)+env(safe-area-inset-bottom))] pl-[env(safe-area-inset-left)] pr-[env(safe-area-inset-right)] xl:pb-0">
      {/* The left offset carries the landscape safe-area inset itself: this is
          position:fixed, so the body-level inset in globals.css (PSY-1820)
          does not apply to it, and as the document's first focusable element
          it must not land under the notch for keyboard/switch-control users. */}
      <a
        href="#main-content"
        className="fixed left-[calc(1rem+env(safe-area-inset-left))] top-3 z-[100] -translate-y-20 rounded-md border border-border bg-background px-4 py-2 text-sm font-medium text-foreground opacity-0 shadow-md transition-transform focus:translate-y-0 focus:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring"
      >
        Skip to content
      </a>
      <TopBar variant={navMode === 'side' ? 'slim' : 'full'} />
      {navMode === 'side' ? <SideNavShell>{children}</SideNavShell> : children}
      <BottomTabBar />
      <CommandPalette />
    </div>
  )
}
