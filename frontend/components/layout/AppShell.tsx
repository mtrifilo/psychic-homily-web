import { cookies } from 'next/headers'
import { TopBar } from './TopBar'
import { CommandPalette } from './CommandPalette'
import { SideNavShell } from './SideNavShell'
import { NAV_MODE_COOKIE, parseNavMode } from '@/lib/nav-mode'

// The global application shell (PSY-1013 top-bar nav; PSY-1116 nav-mode toggle).
// Resolves the user's nav-style preference from the `nav_mode` cookie at SSR and
// renders one of two compositions:
//   • 'top'  (default) — the top bar owns global nav; content renders full-width.
//   • 'side' — a SLIM top bar (no PrimaryNav) above the revived left Sidebar.
// Reading the cookie here makes only the per-request shell dynamic; pages keep
// their own cache modes (same pattern as the auth-hydration + geo shell reads —
// see lib/geo-default.ts). The shell already renders inside the root layout's
// <Suspense> boundary alongside the cookie-reading AuthHydrator.
//
// Order matters: the skip-to-content link is the first focusable element (jumps
// keyboard users past the banner/nav straight to <main id="main-content">, set
// in app/layout.tsx). The CommandPalette is mounted once here so the global ⌘K
// shortcut works on every route.
export async function AppShell({ children }: { children: React.ReactNode }) {
  const navMode = parseNavMode((await cookies()).get(NAV_MODE_COOKIE)?.value)

  return (
    <div className="flex min-h-screen flex-col">
      <a
        href="#main-content"
        className="fixed left-4 top-3 z-[100] -translate-y-20 rounded-md border border-border bg-background px-4 py-2 text-sm font-medium text-foreground opacity-0 shadow-md transition-transform focus:translate-y-0 focus:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring"
      >
        Skip to content
      </a>
      <TopBar variant={navMode === 'side' ? 'slim' : 'full'} />
      {navMode === 'side' ? <SideNavShell>{children}</SideNavShell> : children}
      <CommandPalette />
    </div>
  )
}
