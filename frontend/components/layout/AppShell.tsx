import { TopBar } from './TopBar'
import { CommandPalette } from './CommandPalette'
import { BottomTabBar } from './nav/BottomTabBar'

// The global application shell (PSY-1013). Renamed from SidebarLayout now that
// the left sidebar is retired as the primary navigation — the top bar owns
// global nav, and a deep page that genuinely needs contextual sub-nav adds its
// own page-level linkbox rather than a reinstated global sidebar. Below `lg`
// the BottomTabBar is the primary nav (PSY-1020); the shell's bottom padding
// (bar height + iOS safe-area inset) keeps page content and the footer clear
// of the fixed bar, and collapses at `lg` where the bar disappears.
//
// Order matters: the skip-to-content link is the first focusable element (jumps
// keyboard users past the banner/nav straight to <main id="main-content">, set
// in app/layout.tsx). The CommandPalette is mounted once here so the global ⌘K
// shortcut works on every route.
export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col pb-[calc(var(--bottom-tab-bar-height)+env(safe-area-inset-bottom))] lg:pb-0">
      <a
        href="#main-content"
        className="fixed left-4 top-3 z-[100] -translate-y-20 rounded-md border border-border bg-background px-4 py-2 text-sm font-medium text-foreground opacity-0 shadow-md transition-transform focus:translate-y-0 focus:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring"
      >
        Skip to content
      </a>
      <TopBar />
      {children}
      <BottomTabBar />
      <CommandPalette />
    </div>
  )
}
