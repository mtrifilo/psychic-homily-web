'use client'

import { useState } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useTheme } from 'next-themes'
import {
  Bell, Calendar, ExternalLink, Home, LayoutGrid, Library, LogOut, Moon, Radio,
  Settings, Shield, Sun, User, UserCircle,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger,
} from '@/components/ui/sheet'
import { useAuthContext } from '@/lib/context/AuthContext'
import { isNavActive, mobileBrowseGroups, mobileBrowseHrefs } from './navData'

// The persistent mobile bottom tab bar (PSY-1020, Figma Navigation 540:8 —
// Option A, the user-approved pattern; the hamburger-sheet Option B 542:6 is the
// fallback record only). Five tabs: Home · Shows · Radio · Browse · Account.
// Home/Shows/Radio are plain links (Radio became a plain /radio link in
// PSY-1057, so the static mock and shipped reality agree here). Browse opens the
// long-tail bottom sheet (every desktop Browse/Contribute/Editorial destination,
// composed in navData's mobileBrowseGroups — one source of truth, no forked
// lists). Account is auth-aware: a /auth link for anonymous visitors, an
// account sheet mirroring the UserMenu entries when signed in.
//
// Rendered by AppShell below `lg` on every page; AppShell adds the matching
// bottom padding (var(--bottom-tab-bar-height) + safe-area inset) so fixed-bar
// content is never covered. The bar sits at z-40 — under sheets/dialogs and the
// z-50 top bar, and deliberately under the z-50 cookie banner (PSY-1029 owns
// that surface; see the PR note on stacking).

const primaryTabs: ReadonlyArray<{ href: string; label: string; icon: LucideIcon }> = [
  { href: '/', label: 'Home', icon: Home },
  { href: '/shows', label: 'Shows', icon: Calendar },
  { href: '/radio', label: 'Radio', icon: Radio },
]

// Routes that light the Account tab when signed in — the sheet's own
// destinations. /users/me is the self-profile entry (PSY-1045); after its
// redirect to /users/<username> the tab goes inactive, which is acceptable for
// a redirect alias.
const accountHrefs = ['/notifications', '/library', '/users/me', '/profile', '/admin']

function tabClassName(active: boolean): string {
  return cn(
    'flex h-full flex-col items-center justify-center gap-1 text-[11px] font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/50',
    active ? 'text-primary' : 'text-muted-foreground hover:text-foreground'
  )
}

// Row style for links inside the Browse/Account sheets — carried over from the
// retired hamburger sheet so the long-tail nav reads the same as before.
function sheetLinkClassName(active: boolean): string {
  return cn(
    'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
    active
      ? 'bg-accent text-accent-foreground'
      : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
  )
}

export function BottomTabBar() {
  const pathname = usePathname()
  const { user, isAuthenticated, isLoading, logout } = useAuthContext()
  const { theme, setTheme } = useTheme()
  const [browseOpen, setBrowseOpen] = useState(false)
  const [accountOpen, setAccountOpen] = useState(false)

  const isActive = (href: string) => isNavActive(pathname, href)

  // Exactly one tab lights up. Primary tabs win on shared prefixes (e.g.
  // /shows/submit is both a Shows descendant and a Browse-sheet destination —
  // Shows takes it); Account owns its own routes; Browse takes the rest of its
  // sheet's destinations.
  const primaryActive = primaryTabs.some(t => isActive(t.href))
  const accountActive = (isAuthenticated ? accountHrefs : ['/auth']).some(isActive)
  const browseActive =
    !primaryActive && !accountActive && mobileBrowseHrefs.some(isActive)

  return (
    <nav
      aria-label="Mobile navigation"
      className="fixed inset-x-0 bottom-0 z-40 border-t border-border/50 bg-background/95 pb-[env(safe-area-inset-bottom)] backdrop-blur-sm supports-[backdrop-filter]:bg-background/80 lg:hidden"
    >
      <div className="grid h-[var(--bottom-tab-bar-height)] grid-cols-5">
        {primaryTabs.map(tab => {
          const active = isActive(tab.href)
          const Icon = tab.icon
          return (
            <Link
              key={tab.href}
              href={tab.href}
              aria-current={active ? 'page' : undefined}
              className={tabClassName(active)}
            >
              <Icon className="size-5" aria-hidden />
              {tab.label}
            </Link>
          )
        })}

        {/* Browse — the long-tail sheet */}
        <Sheet open={browseOpen} onOpenChange={setBrowseOpen}>
          <SheetTrigger
            className={tabClassName(browseActive)}
            aria-current={browseActive ? 'page' : undefined}
          >
            <LayoutGrid className="size-5" aria-hidden />
            Browse
          </SheetTrigger>
          <SheetContent
            side="bottom"
            className="max-h-[80dvh] gap-0 pb-[env(safe-area-inset-bottom)]"
          >
            <SheetHeader className="border-b border-border/50 px-4 py-3">
              <SheetTitle className="text-left text-base">Browse</SheetTitle>
            </SheetHeader>
            <nav aria-label="Browse" className="overflow-y-auto px-2 py-4">
              {mobileBrowseGroups.map(group => (
                <div key={group.label} className="mb-4">
                  <p className="mb-2 px-3 font-mono text-[11px] font-semibold uppercase tracking-[0.12em] text-muted-foreground/60">
                    {group.label}
                  </p>
                  {group.items
                    .filter(item => !item.authOnly || isAuthenticated)
                    .map(item => {
                      const Icon = item.icon
                      const active = !item.external && isActive(item.href)
                      return (
                        <Link
                          key={item.href}
                          href={item.href}
                          target={item.external ? '_blank' : undefined}
                          rel={item.external ? 'noopener noreferrer' : undefined}
                          onClick={() => setBrowseOpen(false)}
                          className={sheetLinkClassName(active)}
                        >
                          {Icon && <Icon className="size-4" aria-hidden />}
                          <span>{item.label}</span>
                          {item.external && (
                            <ExternalLink className="ml-auto size-3 opacity-50" aria-hidden />
                          )}
                        </Link>
                      )
                    })}
                </div>
              ))}

              {/* Theme toggle — migrated from the retired hamburger sheet; the
                  top bar's toggle is hidden below `sm`, so this keeps it
                  reachable for everyone (incl. anonymous) on phones. */}
              <div className="mx-3 my-2 border-t border-border/30" />
              <button
                onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                className="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm font-medium text-foreground/70 transition-colors hover:bg-accent/50 hover:text-accent-foreground"
              >
                <Sun className="size-4 dark:hidden" aria-hidden />
                <Moon className="hidden size-4 dark:block" aria-hidden />
                {theme === 'dark' ? 'Light mode' : 'Dark mode'}
              </button>
            </nav>
          </SheetContent>
        </Sheet>

        {/* Account — auth-aware */}
        {isLoading ? (
          // Inert placeholder during auth hydration so the 5-tab grid doesn't
          // jump; mirrors the top bar hiding its account cluster while loading.
          <div aria-hidden className={tabClassName(false)}>
            <User className="size-5" />
            Account
          </div>
        ) : isAuthenticated && user ? (
          <Sheet open={accountOpen} onOpenChange={setAccountOpen}>
            <SheetTrigger
              className={tabClassName(accountActive)}
              aria-current={accountActive ? 'page' : undefined}
            >
              <User className="size-5" aria-hidden />
              Account
            </SheetTrigger>
            <SheetContent
              side="bottom"
              className="max-h-[80dvh] gap-0 pb-[env(safe-area-inset-bottom)]"
            >
              <SheetHeader className="border-b border-border/50 px-4 py-3">
                <SheetTitle className="text-left text-base">Account</SheetTitle>
                <p className="truncate text-sm text-muted-foreground">{user.email}</p>
              </SheetHeader>
              {/* Mirrors the desktop UserMenu entries (+ Settings, which the
                  retired hamburger sheet carried). /users/me handles the
                  username-or-claim routing server-side (PSY-1045). */}
              <nav aria-label="Account" className="overflow-y-auto px-2 py-4">
                {[
                  { href: '/notifications', label: 'Notifications', icon: Bell },
                  { href: '/library', label: 'My Library', icon: Library },
                  { href: '/users/me', label: 'Profile', icon: UserCircle },
                  { href: '/profile', label: 'Settings', icon: Settings },
                  ...(user.is_admin
                    ? [{ href: '/admin', label: 'Admin', icon: Shield }]
                    : []),
                ].map(item => {
                  const Icon = item.icon
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      onClick={() => setAccountOpen(false)}
                      className={sheetLinkClassName(isActive(item.href))}
                    >
                      <Icon className="size-4" aria-hidden />
                      {item.label}
                    </Link>
                  )
                })}
                <div className="mx-3 my-2 border-t border-border/30" />
                <div className="px-3 py-2">
                  <Button
                    variant="outline"
                    className="w-full justify-start"
                    onClick={() => {
                      logout()
                      setAccountOpen(false)
                    }}
                  >
                    <LogOut className="mr-2 size-4" aria-hidden />
                    Sign out
                  </Button>
                </div>
              </nav>
            </SheetContent>
          </Sheet>
        ) : (
          <Link
            href="/auth"
            aria-current={accountActive ? 'page' : undefined}
            className={tabClassName(accountActive)}
          >
            <User className="size-5" aria-hidden />
            Account
          </Link>
        )}
      </div>
    </nav>
  )
}
