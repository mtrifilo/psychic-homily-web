'use client'

import { useState } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { ExternalLink, LayoutGrid, LogOut, Moon, Sun, User } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { Button } from '@/components/ui/button'
import { useThemeToggle } from '../mode-toggle'
import {
  Sheet, SheetClose, SheetContent, SheetDescription, SheetHeader, SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import {
  UnreadCountBadge,
  withUnreadLabel,
} from '@/components/shared/UnreadCountBadge'
import { useUnreadNotificationCount } from '@/features/notifications'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  NOTIFICATIONS_HREF, PROFILE_CLAIM_HREF, accountNavItems, isNavActive,
  mobileBrowseGroups, mobileBrowseHrefs, primaryTabs, sheetLinkClassName,
  navGroupLabelClassName, visibleNavItems,
} from './navData'
import type { NavDestination, NavLink } from './navData'

// The persistent mobile bottom tab bar (PSY-1020, Figma Navigation 540:8 —
// Option A, the user-approved pattern; the hamburger-sheet Option B 542:6 is the
// fallback record only). Five tabs: Home · Shows · Radio · Browse · Account.
// Home/Shows/Radio are plain links (Radio became a plain /radio link in
// PSY-1057, so the static mock and shipped reality agree here). Browse opens the
// long-tail bottom sheet (every desktop Browse/Contribute/Editorial destination,
// composed in navData's mobileBrowseGroups — one source of truth, no forked
// lists). Account is auth-aware: a /auth link for every viewer not settled as
// signed in (anonymous, and unsettled too), an account sheet mirroring the
// UserMenu entries once signed in, carrying the unread-notification badge on
// both the tab and the sheet's Notifications row
// (PSY-1819 — below `sm` the top bar's bell is hidden, so without this there is
// no unread affordance on a phone at all).
//
// Rendered by AppShell below `xl` on every page — matching PrimaryNav's
// xl:flex, so the lg–xl band (tablets) keeps a primary nav; AppShell adds the
// matching bottom padding so fixed-bar content is never covered.
//
// GEOMETRY (PSY-1820). The bar's height is pinned to
// `--bottom-tab-bar-height + env(safe-area-inset-bottom)` rather than left to
// grow from its contents, so the element and every surface that subtracts it
// measure the same thing. The var already includes the 1px border-t, and the
// box is border-box, so the grid's h-full still resolves to the designed
// tab row — see the --bottom-tab-bar-height comment in globals.css,
// which lists every surface that must subtract the bar. The env() terms resolve
// to the device's reported insets as of PSY-1820 (app/layout.tsx sets
// viewport-fit=cover): the bottom inset is what lifts the tabs off a home
// indicator, and the left/right insets keep the outer tabs off the notch and
// rounded corners in landscape while the background stays full-bleed to both
// screen edges. Note safe-area-inset-TOP is deliberately unhandled here and
// app-wide: in a Safari tab (this app ships no web-app manifest and no
// standalone mode) the browser chrome already occupies that band. If the app
// ever goes installable, the top inset goes live and TopBar needs it — that is
// tracked with the rest of the uncovered surfaces in PSY-1824.
//
// The bar sits at z-40 — under sheets/dialogs and the z-50 top bar. The z-50
// cookie banner offsets itself ABOVE the bar below `xl` (CookieConsentBanner.tsx
// carries the other end of that contract): the two must never co-occupy the
// bottom band, or every pre-consent visitor loses the primary nav.

function tabClassName(active: boolean): string {
  return cn(
    'flex h-full flex-col items-center justify-center gap-1 text-[11px] font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/50',
    active ? 'text-primary' : 'text-muted-foreground hover:text-foreground'
  )
}

// A row inside a bottom sheet. SheetClose closes the (controlled) sheet on tap
// immediately, without waiting for the route change that SheetTab also reacts to.
//
// `unreadCount` trails a badge on the row and folds the number into its
// accessible name (the badge itself is decorative). At 0 the row is
// byte-for-byte what it was — see withUnreadLabel for the guard convention.
//
// prefetch={false} (decided in PSY-1820): opening the Browse sheet mounts 24 of
// these at once (22 for anonymous visitors), so the default fires 24 viewport
// prefetches on exactly the clients least able to afford them — phones on
// mobile data, which is the only place these sheets exist (xl:hidden). The
// visitor came here to pick ONE destination; paying for two dozen to save
// latency on one is a bad trade at this fan-out.
//
// Sizing the trade honestly: the default is FetchStrategy.PPR, so what we skip
// is 24 PARTIAL prefetches, not 24 full route payloads. The win is smaller than
// "a burst of pages" — it is still 24 requests a phone did not ask for.
//
// And the cost is bigger than it looks: `prefetch={false}` is the strong
// opt-out. In the App Router it disables prefetching on hover AND on
// touchstart (link.js returns early on !prefetchEnabled in both handlers), so
// the tapped row loses the ~100-300ms intent head start touchstart would have
// given it and navigates cold. Accepted deliberately: 24 speculative fetches
// for one real navigation is the worse end of the trade on a metered phone.
//
// This applies to the Account sheet's rows too, a DELIBERATE divergence from
// the desktop UserMenu those rows mirror: UserMenu opts out only its /admin
// links, because a dropdown of ~6 items on desktop bandwidth is not the same
// trade as a phone sheet. The fan-out is the reason, not the route. The admin
// drawer (AdminDrawerNav) is deliberately left on the default: it is a small,
// admin-only route set behind a login, not a 24-row public surface.
function SheetNavLink({
  item,
  active,
  unreadCount = 0,
}: {
  item: NavLink
  active: boolean
  unreadCount?: number
}) {
  const Icon = item.icon
  return (
    <SheetClose asChild>
      <Link
        href={item.href}
        prefetch={false}
        target={item.external ? '_blank' : undefined}
        rel={item.external ? 'noopener noreferrer' : undefined}
        aria-label={
          unreadCount > 0 ? withUnreadLabel(item.label, unreadCount) : undefined
        }
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
        <UnreadCountBadge count={unreadCount} className="ml-auto" />
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
  unreadCount = 0,
  children,
}: {
  label: string
  icon: LucideIcon
  active: boolean
  subtitle?: string
  description: string
  unreadCount?: number
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
        aria-label={
          unreadCount > 0 ? withUnreadLabel(label, unreadCount) : undefined
        }
      >
        {/* The offsets pull the badge off the 20px icon box without widening
            the tab's centred column — the grid cell is fixed-width, so
            anything that affects layout here shifts the label under it. */}
        <span className="relative">
          <Icon className="size-5" aria-hidden />
          <UnreadCountBadge
            count={unreadCount}
            className="absolute -right-2 -top-1"
          />
        </span>
        {label}
      </SheetTrigger>
      <SheetContent
        side="bottom"
        // The home-indicator inset comes from sheet.tsx's shared `bottom`
        // variant; only the landscape gutters are this sheet's own business.
        className="max-h-[80dvh] gap-0 pl-[env(safe-area-inset-left)] pr-[env(safe-area-inset-right)]"
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
  const { toggle: toggleTheme, label: themeLabel } = useThemeToggle()
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
          SheetClose: flipping the theme should show the result in place. */}
      <div className="mx-3 my-2 border-t border-border/30" />
      {/* Icon tracks the ACTION like the label does (Sun + "Light mode" in
          dark), not the current state — an icon meaning "you are in dark"
          beside a label meaning "switch to light" reads as two controls. */}
      <button
        onClick={toggleTheme}
        className="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm font-medium text-foreground/70 transition-colors hover:bg-accent/50 hover:text-accent-foreground"
      >
        <Sun className="hidden size-4 dark:block" aria-hidden />
        <Moon className="size-4 dark:hidden" aria-hidden />
        {themeLabel}
      </button>
    </>
  )
}

function AccountSheetBody({
  items,
  pathname,
  logout,
  unreadCount,
}: {
  items: NavDestination[]
  pathname: string
  logout: () => void
  unreadCount: number
}) {
  return (
    <>
      {items.map(item => (
        <SheetNavLink
          key={item.href}
          item={item}
          active={isNavActive(pathname, item.href)}
          unreadCount={item.href === NOTIFICATIONS_HREF ? unreadCount : 0}
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
  const { user, authStatus, logout } = useAuthContext()

  // Costs no request and self-gates to 0 when signed out — see the hook.
  const unreadCount = useUnreadNotificationCount()

  const isActive = (href: string) => isNavActive(pathname, href)

  // Everything the Account cell derives from the viewer turns on this one
  // signal, so the tab's destination, its active set and its badge cannot
  // disagree about who is looking. It is `authStatus`, not `isLoading`, per
  // the rule on the AuthStatus type: `isLoading` is false both before the
  // profile fetch starts and after it fails, and in either window an
  // `isLoading` gate falls through to a claim the context has not settled.
  const isSettledAuthenticated = authStatus === 'authenticated' && user !== null

  // The canonical account list (navData, PSY-1821) with the username-aware
  // Profile href already resolved. Admin stays in the list (adminOnly) so the
  // tab's active state covers every account route; rendering filters it.
  // Viewers who never read it (anonymous, unsettled) don't get it built.
  const accountItems = isSettledAuthenticated ? accountNavItems(user) : []

  // At most one tab lights up (off-map routes like /help light none). Primary
  // tabs win on shared prefixes (e.g.
  // /shows/submit is both a Shows descendant and a Browse-sheet destination —
  // Shows takes it); Account owns its own routes; Browse takes the rest of its
  // sheet's destinations. The claim-view alias stays in the active set even
  // when Profile deep-links to /users/<username>.
  const primaryActive = primaryTabs.some(t => isActive(t.href))
  const accountActive = isSettledAuthenticated
    ? accountItems.some(i => isActive(i.href)) || isActive(PROFILE_CLAIM_HREF)
    : isActive('/auth')
  const browseActive =
    !primaryActive && !accountActive && mobileBrowseHrefs.some(isActive)

  return (
    <nav
      aria-label="Mobile navigation"
      className="fixed inset-x-0 bottom-0 z-40 h-[calc(var(--bottom-tab-bar-height)+env(safe-area-inset-bottom))] border-t border-border/50 bg-background/95 pb-[env(safe-area-inset-bottom)] pl-[env(safe-area-inset-left)] pr-[env(safe-area-inset-right)] backdrop-blur-sm supports-[backdrop-filter]:bg-background/80 xl:hidden"
    >
      <div className="grid h-full grid-cols-5">
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

        {/* The auth-aware Account cell. The sheet asserts an identity (the viewer's
            email as its subtitle, their account destinations, their unread
            count, sign out), so it is what a settled 'authenticated' buys;
            every other status gets the anonymous /auth link, which names no
            viewer and is the tab's only route to sign-in. That is the rule
            UserMenu applies to the desktop bar, and both bars are on screen
            together at tablet widths, so they have to agree. Either arm fills
            the same one of five equal grid columns, so the cell does not jump
            when auth settles. */}
        {isSettledAuthenticated ? (
          <SheetTab
            label="Account"
            icon={User}
            active={accountActive}
            subtitle={user.email}
            description="Your account: notifications, library, profile, settings, and sign out."
            unreadCount={unreadCount}
          >
            <AccountSheetBody
              items={visibleNavItems(accountItems, user)}
              pathname={pathname}
              logout={logout}
              unreadCount={unreadCount}
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
