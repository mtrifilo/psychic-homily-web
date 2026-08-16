'use client'

import { useState } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useTheme } from 'next-themes'
import { ExternalLink, LayoutGrid, LogOut, Moon, Sun, User } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { Button } from '@/components/ui/button'
import {
  Sheet, SheetClose, SheetContent, SheetDescription, SheetHeader, SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  PROFILE_CLAIM_HREF, accountNavItems, isNavActive, mobileBrowseGroups,
  mobileBrowseHrefs, primaryTabs, sheetLinkClassName, navGroupLabelClassName,
  visibleNavItems,
} from './navData'
import type { NavDestination, NavLink } from './navData'

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
// matching bottom padding so fixed-bar content is never covered. (The
// env(safe-area-inset-bottom) terms here are inert until viewport-fit=cover
// ships — see the --bottom-tab-bar-height comment in globals.css, which also
// lists every surface that must subtract the bar's height.) The bar sits at
// z-40 — under sheets/dialogs and the z-50 top bar. The z-50 cookie banner
// offsets itself ABOVE the bar below `xl` (CookieConsentBanner.tsx carries the
// other end of that contract): the two must never co-occupy the bottom band,
// or every pre-consent visitor loses the primary nav.

function tabClassName(active: boolean): string {
  return cn(
    'flex h-full flex-col items-center justify-center gap-1 text-[11px] font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/50',
    active ? 'text-primary' : 'text-muted-foreground hover:text-foreground'
  )
}

// A row inside a bottom sheet. SheetClose closes the (controlled) sheet on tap
// immediately, without waiting for the route change that SheetTab also reacts to.
function SheetNavLink({ item, active }: { item: NavLink; active: boolean }) {
  const Icon = item.icon
  return (
    <SheetClose asChild>
      <Link
        href={item.href}
        target={item.external ? '_blank' : undefined}
        rel={item.external ? 'noopener noreferrer' : undefined}
        className={cn(
          sheetLinkClassName(active),
          // The Contribute menu's "+ Submit a show" CTA keeps its primary-color
          // treatment in the sheet (Figma 460:3 — same rule the desktop menu
          // applies via navData's submitPrimary flag).
          item.submitPrimary && !active && 'text-primary hover:text-primary'
        )}
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
//
// CONTROLLED, closed on route change: Radix sheets don't push a history entry,
// so Android hardware-Back / iOS edge-swipe performs a client-side navigation
// with the sheet still open — the user "goes back" and the nav sheet is still
// covering the new page. Back IS the dismiss gesture on the platform this bar
// exists for. (Same pattern the retired hamburger sheet used; row taps still
// close instantly via SheetClose without waiting for the route.)
function SheetTab({
  label,
  icon: Icon,
  active,
  subtitle,
  description,
  children,
}: {
  label: string
  icon: LucideIcon
  active: boolean
  subtitle?: string
  description: string
  children: React.ReactNode
}) {
  const pathname = usePathname()
  const [open, setOpen] = useState(false)
  // previous-value-guard: adjust state DURING render on a pathname change, not
  // in an effect (react-hooks/set-state-in-effect; the PSY-1345 idiom).
  const [prevPathname, setPrevPathname] = useState(pathname)
  if (pathname !== prevPathname) {
    setPrevPathname(pathname)
    if (open) setOpen(false)
  }

  return (
    <Sheet open={open} onOpenChange={setOpen}>
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
          <SheetDescription className="sr-only">{description}</SheetDescription>
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
  user,
  pathname,
}: {
  user: { email: string; is_admin?: boolean } | null
  pathname: string
}) {
  const { resolvedTheme, setTheme } = useTheme()
  return (
    <>
      {mobileBrowseGroups.map(group => (
        <div key={group.label} className="mb-4">
          <p className={cn('mb-2 px-3', navGroupLabelClassName)}>{group.label}</p>
          {visibleNavItems(group.items, user).map(item => (
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
      {/* Icon tracks the ACTION like the label does (Sun + "Light mode" in
          dark), not the current state — an icon meaning "you are in dark"
          beside a label meaning "switch to light" reads as two controls. */}
      <button
        onClick={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}
        className="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm font-medium text-foreground/70 transition-colors hover:bg-accent/50 hover:text-accent-foreground"
      >
        <Sun className="hidden size-4 dark:block" aria-hidden />
        <Moon className="size-4 dark:hidden" aria-hidden />
        {resolvedTheme === 'dark' ? 'Light mode' : 'Dark mode'}
      </button>
    </>
  )
}

function AccountSheetBody({
  items,
  pathname,
  logout,
}: {
  items: NavDestination[]
  pathname: string
  logout: () => void
}) {
  return (
    <>
      {items.map(item => (
        <SheetNavLink
          key={item.href}
          item={item}
          active={isNavActive(pathname, item.href)}
        />
      ))}
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

  // The canonical account list (navData, PSY-1821) with the username-aware
  // Profile href already resolved. Admin stays in the list (adminOnly) so the
  // tab's active state covers every account route; rendering filters it.
  // Anonymous visitors never read it, so don't build it for them.
  const accountItems = isAuthenticated && user ? accountNavItems(user) : []

  // At most one tab lights up (off-map routes like /help light none). Primary
  // tabs win on shared prefixes (e.g.
  // /shows/submit is both a Shows descendant and a Browse-sheet destination —
  // Shows takes it); Account owns its own routes; Browse takes the rest of its
  // sheet's destinations. The claim-view alias stays in the active set even
  // when Profile deep-links to /users/<username>.
  const primaryActive = primaryTabs.some(t => isActive(t.href))
  const accountActive = isAuthenticated
    ? accountItems.some(i => isActive(i.href)) || isActive(PROFILE_CLAIM_HREF)
    : isActive('/auth')
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
        <SheetTab
          label="Browse"
          icon={LayoutGrid}
          active={browseActive}
          description="All destinations: catalog, curation, scenes, contribute, and editorial links."
        >
          <BrowseSheetBody user={user} pathname={pathname} />
        </SheetTab>

        {/* Account — auth-aware */}
        {isLoading ? (
          // Inert placeholder during auth hydration so the 5-tab grid doesn't
          // jump (grid stability — NOT parity with the top bar, whose UserMenu
          // shows a spinner while loading). aria-hidden: it looks tappable but
          // is deliberately inert until auth settles.
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
            description="Your account: notifications, library, profile, settings, and sign out."
          >
            <AccountSheetBody
              items={visibleNavItems(accountItems, user)}
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
