'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useTheme } from 'next-themes'
import {
  Bell, Calendar, ExternalLink, Home, LayoutGrid, Library, LogOut, Moon, Radio,
  Palette, Settings, Shield, Sun, User, UserCircle,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { Button } from '@/components/ui/button'
import {
  Sheet, SheetClose, SheetContent, SheetHeader, SheetTitle, SheetTrigger,
} from '@/components/ui/sheet'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  isNavActive, mobileBrowseGroups, mobileBrowseHrefs, sheetLinkClassName,
  navGroupLabelClassName,
} from './navData'
import type { NavLink } from './navData'

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
// Rendered by AppShell below `xl` on every page — matching PrimaryNav's
// xl:flex, so the lg–xl band (tablets) keeps a primary nav; AppShell adds the
// matching bottom padding (var(--bottom-tab-bar-height) + safe-area inset) so
// fixed-bar content is never covered. The bar sits at z-40 — under
// sheets/dialogs and the z-50 top bar, and deliberately under the z-50 cookie
// banner (PSY-1029 owns that surface; see the PR note on stacking).

// Exported for the mobile-reachability guard test against PrimaryNav's
// primaryLinks: every desktop primary destination must appear here or in
// mobileBrowseHrefs.
export const primaryTabs: ReadonlyArray<{ href: string; label: string; icon: LucideIcon }> = [
  { href: '/', label: 'Home', icon: Home },
  { href: '/shows', label: 'Shows', icon: Calendar },
  { href: '/radio', label: 'Radio', icon: Radio },
]

// The account sheet's destinations — hrefs for the tab's active state are
// DERIVED below, so adding an entry can't silently stop the tab lighting up.
// Mirrors the desktop UserMenu (+ Settings, which the retired hamburger sheet
// carried). The Profile entry's '/users/me' is a placeholder: the component
// substitutes the username-aware href (same rule as UserMenu/Sidebar, PSY-1045).
const accountItems: ReadonlyArray<NavLink & { icon: LucideIcon; adminOnly?: boolean }> = [
  { href: '/notifications', label: 'Notifications', icon: Bell },
  { href: '/library', label: 'My Library', icon: Library },
  { href: '/users/me', label: 'Profile', icon: UserCircle },
  { href: '/profile', label: 'Settings', icon: Settings },
  { href: '/settings/appearance', label: 'Appearance', icon: Palette },
  { href: '/admin', label: 'Admin', icon: Shield, adminOnly: true },
]
const accountHrefs = accountItems.map(i => i.href)

function tabClassName(active: boolean): string {
  return cn(
    'flex h-full flex-col items-center justify-center gap-1 text-[11px] font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/50',
    active ? 'text-primary' : 'text-muted-foreground hover:text-foreground'
  )
}

// A row inside a bottom sheet. SheetClose makes the sheets uncontrolled — the
// sheet closes on navigation without the component tracking open state.
function SheetNavLink({ item, active }: { item: NavLink; active: boolean }) {
  const Icon = item.icon
  return (
    <SheetClose asChild>
      <Link
        href={item.href}
        target={item.external ? '_blank' : undefined}
        rel={item.external ? 'noopener noreferrer' : undefined}
        className={sheetLinkClassName(active)}
      >
        {Icon && <Icon className="size-4" aria-hidden />}
        <span>{item.label}</span>
        {item.external && (
          <ExternalLink className="ml-auto size-3 opacity-50" aria-hidden />
        )}
      </Link>
    </SheetClose>
  )
}

// One tab-slot bottom sheet: trigger chrome + content shell. The body is passed
// as a COMPONENT'S children so a closed sheet never renders (Radix keeps closed
// content unmounted) — the bar re-renders on every navigation, and desktop
// (xl+) can never open these at all.
function SheetTab({
  label,
  icon: Icon,
  active,
  subtitle,
  children,
}: {
  label: string
  icon: LucideIcon
  active: boolean
  subtitle?: string
  children: React.ReactNode
}) {
  return (
    <Sheet>
      <SheetTrigger
        {...replayOnHydrate}
        className={tabClassName(active)}
        aria-current={active ? 'page' : undefined}
      >
        <Icon className="size-5" aria-hidden />
        {label}
      </SheetTrigger>
      <SheetContent
        side="bottom"
        className="max-h-[80dvh] gap-0 pb-[env(safe-area-inset-bottom)]"
      >
        <SheetHeader className="border-b border-border/50 px-4 py-3">
          <SheetTitle className="text-left text-base">{label}</SheetTitle>
          {subtitle && (
            <p className="truncate text-sm text-muted-foreground">{subtitle}</p>
          )}
        </SheetHeader>
        <nav aria-label={label} className="overflow-y-auto px-2 py-4">
          {children}
        </nav>
      </SheetContent>
    </Sheet>
  )
}

function BrowseSheetBody({
  isAuthenticated,
  pathname,
}: {
  isAuthenticated: boolean
  pathname: string
}) {
  const { resolvedTheme, setTheme } = useTheme()
  return (
    <>
      {mobileBrowseGroups.map(group => (
        <div key={group.label} className="mb-4">
          <p className={cn('mb-2 px-3', navGroupLabelClassName)}>{group.label}</p>
          {group.items
            .filter(item => !item.authOnly || isAuthenticated)
            .map(item => (
              <SheetNavLink
                key={item.href}
                item={item}
                active={!item.external && isNavActive(pathname, item.href)}
              />
            ))}
        </div>
      ))}

      {/* Theme toggle — migrated from the retired hamburger sheet; the
          top bar's toggle is hidden below `sm`, so this keeps it reachable
          for everyone (incl. anonymous) on phones. Deliberately NOT a
          SheetClose: flipping the theme should show the result in place.
          resolvedTheme (not theme) so the first click always flips the
          VISIBLE theme under theme="system" — matches the canonical
          ModeToggle. */}
      <div className="mx-3 my-2 border-t border-border/30" />
      <button
        onClick={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}
        className="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm font-medium text-foreground/70 transition-colors hover:bg-accent/50 hover:text-accent-foreground"
      >
        <Sun className="size-4 dark:hidden" aria-hidden />
        <Moon className="hidden size-4 dark:block" aria-hidden />
        {resolvedTheme === 'dark' ? 'Light mode' : 'Dark mode'}
      </button>
    </>
  )
}

function AccountSheetBody({
  isAdmin,
  profileHref,
  pathname,
  logout,
}: {
  isAdmin: boolean
  profileHref: string
  pathname: string
  logout: () => void
}) {
  return (
    <>
      {accountItems
        .filter(item => !item.adminOnly || isAdmin)
        .map(item => {
          const href = item.href === '/users/me' ? profileHref : item.href
          return (
            <SheetNavLink
              key={item.href}
              item={{ ...item, href }}
              active={isNavActive(pathname, href)}
            />
          )
        })}
      <div className="mx-3 my-2 border-t border-border/30" />
      <div className="px-3 py-2">
        <SheetClose asChild>
          <Button variant="outline" className="w-full justify-start" onClick={logout}>
            <LogOut className="mr-2 size-4" aria-hidden />
            Sign out
          </Button>
        </SheetClose>
      </div>
    </>
  )
}

export function BottomTabBar() {
  const pathname = usePathname()
  const { user, isAuthenticated, isLoading, logout } = useAuthContext()

  const isActive = (href: string) => isNavActive(pathname, href)

  // Same username-or-claim routing rule as UserMenu/Sidebar (PSY-1045): with a
  // username the Profile entry deep-links (no redirect hop, and the Account tab
  // lights on the landing route); without one it routes to the claim view.
  const profileHref = user?.username ? `/users/${user.username}` : '/users/me'

  // Exactly one tab lights up. Primary tabs win on shared prefixes (e.g.
  // /shows/submit is both a Shows descendant and a Browse-sheet destination —
  // Shows takes it); Account owns its own routes; Browse takes the rest of its
  // sheet's destinations.
  const primaryActive = primaryTabs.some(t => isActive(t.href))
  const accountActive = (
    isAuthenticated ? [...accountHrefs, profileHref] : ['/auth']
  ).some(isActive)
  const browseActive =
    !primaryActive && !accountActive && mobileBrowseHrefs.some(isActive)

  return (
    <nav
      aria-label="Mobile navigation"
      className="fixed inset-x-0 bottom-0 z-40 border-t border-border/50 bg-background/95 pb-[env(safe-area-inset-bottom)] backdrop-blur-sm supports-[backdrop-filter]:bg-background/80 xl:hidden"
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
        <SheetTab label="Browse" icon={LayoutGrid} active={browseActive}>
          <BrowseSheetBody isAuthenticated={isAuthenticated} pathname={pathname} />
        </SheetTab>

        {/* Account — auth-aware */}
        {isLoading ? (
          // Inert placeholder during auth hydration so the 5-tab grid doesn't
          // jump; mirrors the top bar hiding its account cluster while loading.
          <div aria-hidden className={tabClassName(false)}>
            <User className="size-5" />
            Account
          </div>
        ) : isAuthenticated && user ? (
          <SheetTab
            label="Account"
            icon={User}
            active={accountActive}
            subtitle={user.email}
          >
            <AccountSheetBody
              isAdmin={!!user.is_admin}
              profileHref={profileHref}
              pathname={pathname}
              logout={logout}
            />
          </SheetTab>
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
