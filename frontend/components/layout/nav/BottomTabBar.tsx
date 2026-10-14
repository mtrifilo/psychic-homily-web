'use client'

import { useState } from 'react'
import { ChromeLink } from '@/components/layout/nav/ChromeLink'
import { useLinkStatus } from 'next/link'
import { usePathname } from 'next/navigation'
import { AUTH_PATH, buildAuthHref } from '@/lib/auth-href'
import {
  ChevronRight, ExternalLink, LayoutGrid, LogIn, LogOut, Moon, Sun, User,
} from 'lucide-react'
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
import { useUnreadNotificationCount } from '@/features/notifications/hooks'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useCookieConsent } from '@/lib/context/CookieConsentContext'
import {
  NOTIFICATIONS_HREF, PROFILE_CLAIM_HREF, accountNavItems, isNavActive,
  legalSheetRows, mobileBrowseGroups, mobileBrowseHrefs, primaryTabs, sheetLinkClassName,
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
// lists). Account opens a sheet for every viewer, ending in the legal rows
// (Cookie preferences, Privacy, Terms): for a viewer not settled as signed in
// (anonymous, and unsettled too) a Sign in link back to the current page; once
// signed in, the UserMenu entries and Sign out, with the unread-notification
// badge on both the tab and the sheet's Notifications row (below `sm` the top
// bar's bell is hidden, so without this there is no unread affordance on a
// phone at all).
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

// While a tab's navigation is pending (TabLinkBody marks it data-tab-pending),
// that tab reads as selected and every other tab, the active one included,
// reads as unselected, until the destination commits and `active` moves.
// Product Designs XakQQ0nYGqnt77PrHKO9IE, boards 1955:90 and 1955:118.
// scripts/chrome-perf.mjs times the pending paint by the same marker.
const pendingTabClassName =
  'has-[[data-tab-pending]]:text-primary group-has-[[data-tab-pending]]/tabbar:not-has-[[data-tab-pending]]:text-muted-foreground'

function tabClassName(active: boolean): string {
  return cn(
    'flex h-full flex-col items-center justify-center gap-1 text-[11px] font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/50',
    active ? 'text-primary' : 'text-muted-foreground hover:text-foreground',
    pendingTabClassName
  )
}

// A 16px three-quarter arc with a 2px stroke, in the tab's colour.
function TabPendingSpinner() {
  return (
    <svg viewBox="0 0 16 16" fill="none" aria-hidden className="size-4 animate-spin">
      <path d="M15 8A7 7 0 1 1 8 1" stroke="currentColor" strokeWidth="2" />
    </svg>
  )
}

// A tab link's icon and label. useLinkStatus reads the ChromeLink this renders
// inside: from the tap until the destination commits, the spinner takes the
// icon's 20px box, so the label does not move.
function TabLinkBody({ icon: Icon, label }: { icon: LucideIcon; label: string }) {
  const { pending } = useLinkStatus()
  return (
    <>
      {pending ? (
        <span data-tab-pending="" className="flex size-5 items-center justify-center">
          <TabPendingSpinner />
        </span>
      ) : (
        <Icon className="size-5" aria-hidden />
      )}
      {label}
    </>
  )
}

// A row inside a bottom sheet. SheetClose closes the (controlled) sheet on tap
// immediately, without waiting for the route change that SheetTab also reacts to.
//
// `unreadCount` trails a badge on the row and folds the number into its
// accessible name (the badge itself is decorative). At 0 the row is
// byte-for-byte what it was — see withUnreadLabel for the guard convention.
//
// viewportPrefetch={false}: opening the Browse sheet mounts every one of its
// rows at once, and the visitor came to pick ONE destination, so no row
// prefetches on viewport entry. On a coarse pointer (ChromeLink's
// 'on-touch' policy) a row still prefetches its route when a touch lands on
// it: one request for the one navigation. On a fine pointer below xl the row
// renders Next's prefetch={false}, which turns off its hover and touchstart
// prefetch too.
//
// This applies to the Account sheet's rows too, a deliberate divergence from
// the desktop UserMenu those rows mirror: UserMenu opts out only its /admin
// links, because a short desktop dropdown is not the same trade as a sheet of
// every destination. The fan-out is the reason, not the route. The admin
// drawer (AdminDrawerNav) stays on the default: it is a small, admin-only
// route set behind a login, not a public sheet.
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
      <ChromeLink
        href={item.href}
        viewportPrefetch={false}
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
      </ChromeLink>
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

const legalRowClassName =
  'flex h-9 w-full items-center justify-between rounded-md px-3 text-left text-sm text-muted-foreground transition-colors hover:bg-accent/50 hover:text-accent-foreground'

// The Account sheet's legal block (navData's legalSheetRows): quiet rows with
// a trailing chevron and no icons, so they read as legal links rather than
// destinations. Every row is a SheetClose, so the sheet closes on the same tap.
// For Cookie preferences that matters beyond tidiness: the preferences dialog
// opens in that same render, and the sheet must not stay open over it. The
// links render ChromeLink's prefetch={false}: no prefetch on any pointer, a
// touch included, stricter than SheetNavLink's viewport-only opt-out.
function LegalSheetRows() {
  const { openPreferences } = useCookieConsent()
  return (
    <>
      <div className="mx-3 my-2 border-t border-border/30" />
      {legalSheetRows.map(row => {
        const content = (
          <>
            <span>{row.label}</span>
            <ChevronRight className="size-3.5 opacity-70" aria-hidden />
          </>
        )
        return (
          <SheetClose asChild key={row.label}>
            {row.kind === 'cookie-preferences' ? (
              <button type="button" onClick={openPreferences} className={legalRowClassName}>
                {content}
              </button>
            ) : (
              <ChromeLink href={row.href} prefetch={false} className={legalRowClassName}>
                {content}
              </ChromeLink>
            )}
          </SheetClose>
        )
      })}
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
      <LegalSheetRows />
    </>
  )
}

// The Account sheet of a viewer not settled as signed in: a Sign in link back
// to the page the viewer is on, then the legal rows.
function SignedOutAccountSheetBody({ pathname }: { pathname: string }) {
  return (
    <>
      <SheetNavLink
        item={{
          // The render-time grade of the destination: the pathname alone.
          // See `currentLocationReturnTo` for why a query string cannot be
          // read here.
          href: buildAuthHref(pathname),
          label: 'Sign in',
          icon: LogIn,
        }}
        active={isNavActive(pathname, AUTH_PATH)}
      />
      <LegalSheetRows />
    </>
  )
}

export function BottomTabBar() {
  const pathname = usePathname()
  const { user, authStatus, logout } = useAuthContext()

  // Costs no request and self-gates to 0 when signed out — see the hook.
  const unreadCount = useUnreadNotificationCount()

  const isActive = (href: string) => isNavActive(pathname, href)

  // The one signal every viewer-derived part of the Account cell reads, so
  // the sheet's contents, its active set and its badge cannot disagree about
  // who is looking. `user !== null` narrows the type for `user.email` below
  // and adds nothing else: AuthProvider derives 'authenticated' from a
  // non-null user, so each implies the other.
  //
  // What this cell must NOT read is `isLoading`, which is false both before
  // the profile fetch starts and after it fails, and so has windows where it
  // falls through to a claim the context has not settled. See the AuthStatus
  // type for the rule.
  const isSettledAuthenticated = authStatus === 'authenticated' && user !== null

  // The canonical account list (navData, PSY-1821) with the username-aware
  // Profile href already resolved. Admin stays in the list (adminOnly) so the
  // tab's active state covers every account route; rendering filters it.
  const accountItems = isSettledAuthenticated ? accountNavItems(user) : []

  // At most one tab lights up (off-map routes like /help light none). Primary
  // tabs win on shared prefixes (e.g.
  // /shows/submit is both a Shows descendant and a Browse-sheet destination —
  // Shows takes it); Account owns its own routes (the legal rows' routes light
  // no tab); Browse takes the rest of its sheet's destinations. The claim-view alias stays in the active set even
  // when Profile deep-links to /users/<username>.
  const primaryActive = primaryTabs.some(t => isActive(t.href))
  const accountActive = isSettledAuthenticated
    ? accountItems.some(i => isActive(i.href)) || isActive(PROFILE_CLAIM_HREF)
    : isActive(AUTH_PATH)
  const browseActive =
    !primaryActive && !accountActive && mobileBrowseHrefs.some(isActive)

  return (
    <nav
      aria-label="Mobile navigation"
      // Measured by floating overlays (the first-save hint) that must stay
      // clear of this bar; see the geometry contract in globals.css.
      data-bottom-tab-bar=""
      className="group/tabbar fixed inset-x-0 bottom-0 z-40 h-[calc(var(--bottom-tab-bar-height)+env(safe-area-inset-bottom))] border-t border-border/50 bg-background/95 pb-[env(safe-area-inset-bottom)] pl-[env(safe-area-inset-left)] pr-[env(safe-area-inset-right)] backdrop-blur-sm supports-[backdrop-filter]:bg-background/80 xl:hidden"
    >
      <div className="grid h-full grid-cols-5">
        {primaryTabs.map(tab => {
          const active = isActive(tab.href)
          return (
            <ChromeLink
              key={tab.href}
              href={tab.href}
              atlasPrefetch="after-map"
              aria-current={active ? 'page' : undefined}
              className={tabClassName(active)}
            >
              <TabLinkBody icon={tab.icon} label={tab.label} />
            </ChromeLink>
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

        {/* The auth-aware Account cell. The signed-in sheet names a viewer,
            so it is what a settled 'authenticated' buys; every other status
            gets the signed-out sheet, which names none. That is the rule
            UserMenu applies in the top bar, and the two are on screen
            together from `sm` up to this bar's own `xl:hidden`, so they have
            to agree. Both arms are the same SheetTab in the same one of five
            equal grid columns, so the cell does not jump on settle. Their keys
            differ, so a settle remounts the cell and closes a sheet that is
            open across it, rather than moving different rows under a finger
            already on its way to one. */}
        {isSettledAuthenticated ? (
          <SheetTab
            key="account-signed-in"
            label="Account"
            icon={User}
            active={accountActive}
            subtitle={user.email}
            description="Your account: notifications, library, profile, settings, sign out, cookie preferences, privacy, and terms."
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
          <SheetTab
            key="account-signed-out"
            label="Account"
            icon={User}
            active={accountActive}
            description="Sign in, cookie preferences, privacy, and terms."
          >
            <SignedOutAccountSheetBody pathname={pathname} />
          </SheetTab>
        )}
      </div>
    </nav>
  )
}
