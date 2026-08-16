import {
  Mic2, MapPin, Disc3, Tag, Tent, LayoutList, TrendingUp, Tags, Globe, Trophy,
  MessageSquarePlus, Music, Send, ClipboardList, HeartHandshake, BookOpen, Headphones, Newspaper,
  Map as MapIcon, Home, Calendar, Radio, Orbit, Bell, Library, UserCircle, Settings,
  Palette, Shield,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

// Shared link data + styling for the app's navigation surfaces (PSY-1013).
// Originally the top-bar Browse/Contribute tables (which absorbed the 2025
// left sidebar's ~20 destinations); since PSY-1821 this module also owns the
// live side-nav rail's tables (sidebarGroups, sidebarAccountItems) and the
// account destination set. The menu *presentation* stays in each component;
// the destinations themselves live here.

export interface NavLink {
  href: string
  label: string
  icon?: LucideIcon
  external?: boolean
  authOnly?: boolean
  /**
   * Marks the Contribute menu's call-to-action ("+ Submit a show"), which the
   * Figma design (frame 460:3) renders in the primary color. Submit lives in
   * this menu rather than as a standalone top-bar CTA (OQ-2, resolved).
   */
  submitPrimary?: boolean
  /** Rendered only for admin viewers — filter with visibleNavItems. */
  adminOnly?: boolean
}

export interface NavGroup {
  label: string
  items: NavLink[]
}

// Browse ▾ — the full catalog, faceted. PSY-1014 expands this into the
// three-column mega-menu (+ optional Fresh rail); the grouping here already
// mirrors that planned structure.
export const browseGroups: NavGroup[] = [
  {
    label: 'Catalog',
    items: [
      { href: '/artists', label: 'Artists', icon: Mic2 },
      { href: '/venues', label: 'Venues', icon: MapPin },
      { href: '/releases', label: 'Releases', icon: Disc3 },
      { href: '/labels', label: 'Labels', icon: Tag },
      { href: '/festivals', label: 'Festivals', icon: Tent },
    ],
  },
  {
    label: 'Curation',
    items: [
      { href: '/collections', label: 'Collections', icon: LayoutList },
      { href: '/charts', label: 'Charts', icon: TrendingUp },
      { href: '/tags', label: 'Tags', icon: Tags },
      { href: '/community/leaderboard', label: 'Leaderboard', icon: Trophy },
    ],
  },
  {
    // Scene pages are DERIVED from verified-venue location data, not stored
    // entities; a `<city>` slug is `<city-lowercased,-spaces→dashes>-<state>`
    // (backend `buildSceneSlug`, e.g. Phoenix/AZ → `phoenix-az`). PSY-1030
    // reconciled this list against stage (2026-07-22): API+UI 200 for
    // phoenix-az, tucson-az, los-angeles-ca, denver-co. Denver sat at the
    // sceneMinVenues floor (2) — kept because it resolved; re-check before
    // adding more. "All scenes" is the index. Do not add a city that 404s
    // on stage.
    label: 'Scenes',
    items: [
      { href: '/scenes/phoenix-az', label: 'Phoenix', icon: MapPin },
      { href: '/scenes/tucson-az', label: 'Tucson', icon: MapPin },
      { href: '/scenes/los-angeles-ca', label: 'Los Angeles', icon: MapPin },
      { href: '/scenes/denver-co', label: 'Denver', icon: MapPin },
      { href: '/scenes', label: 'All scenes', icon: Globe },
    ],
  },
]

// Contribute ▾ — the What.cd "request system as call-to-action". PSY-1015
// lays this out as the two-column Participate / Editorial panel from Figma
// (frame 460:3). Leaderboard appears in Participate as a contributor-standing
// destination; it is ALSO reachable from Browse → Curation (it's a plain link,
// so two entry points is fine). The Editorial sub-group is consume-not-
// contribute (resolved to live here per OQ-6).
export const contributeItems: NavLink[] = [
  { href: '/shows/submit', label: 'Submit a Show', icon: Music, submitPrimary: true },
  { href: '/requests', label: 'Requests', icon: MessageSquarePlus },
  { href: '/contribute/submissions', label: 'Show Submissions', icon: ClipboardList, authOnly: true },
  { href: '/submissions', label: 'My Submissions', icon: Send, authOnly: true },
  { href: '/community/leaderboard', label: 'Leaderboard', icon: Trophy },
  { href: '/contribute', label: 'Contribute hub', icon: HeartHandshake },
]

export const editorialItems: NavLink[] = [
  { href: '/blog', label: 'Blog', icon: BookOpen },
  { href: '/dj-sets', label: 'DJ Sets', icon: Headphones },
  {
    href: 'https://psychichomily.substack.com/',
    label: 'Substack',
    icon: Newspaper,
    external: true,
  },
]

// ---------------------------------------------------------------------------
// Single-source destination tables (PSY-1821). A destination's href, label,
// and icon are decided ONCE here; the composed surfaces (PrimaryNav + its
// menus, side-nav rail, mobile tab bar/sheets, account menus) build their own
// chrome and ordering from these entries, so a destination can no longer
// drift between them. Two surfaces still fork their own destination lists —
// CommandPalette's routes and the Footer's link columns (both pre-date this
// module and have already drifted) — folding them in is follow-up work.

/** A destination whose icon is guaranteed — rails and tab bars render it. */
export interface NavDestination extends NavLink {
  icon: LucideIcon
}

/** A labelled group of icon-guaranteed destinations (rail/sheet chrome). */
export interface NavDestinationGroup extends NavGroup {
  items: NavDestination[]
}

/**
 * Apply BOTH viewer gates one way everywhere: adminOnly needs an admin
 * viewer, authOnly needs any signed-in viewer. A null/undefined viewer is
 * anonymous (AuthContext's user is non-null exactly when authenticated) —
 * `email` is required in the type so an empty object can't accidentally
 * stand in for "some signed-in viewer". Every surface that renders a
 * composed table filters through this — a hand-rolled filter is the drift
 * this module exists to prevent.
 */
export function visibleNavItems<T extends NavLink>(
  items: readonly T[],
  viewer: { email: string; is_admin?: boolean } | null | undefined
): T[] {
  return items.filter(
    item =>
      (!item.adminOnly || !!viewer?.is_admin) && (!item.authOnly || viewer != null)
  )
}

// The mobile bar's plain-link tabs — this list IS the mobile-tab membership
// and order decision, nothing else's (PrimaryNav names its own destinations).
// Its length is pinned: BottomTabBar renders these three + Browse + Account
// into a literal grid-cols-5 — a guard test in BottomTabBar.test.tsx fails
// if the two drift apart.
export const primaryTabs: ReadonlyArray<NavDestination> = [
  { href: '/', label: 'Home', icon: Home },
  { href: '/shows', label: 'Shows', icon: Calendar },
  { href: '/radio', label: 'Radio', icon: Radio },
]

// Graph/Atlas are desktop *primary* links with no home in the Browse/
// Contribute menus; these are their canonical entries. One icon per
// destination: Orbit reads as a node graph, Map is literal for Atlas — this
// retires both the side rail's old Graph/Atlas double-Orbit and the mobile
// sheet's Compass.
export const graphItem: NavDestination = { href: '/graph', label: 'Graph', icon: Orbit }
export const atlasItem: NavDestination = { href: '/atlas', label: 'Atlas', icon: MapIcon }

/**
 * The claim-username self view (PSY-1045). Always an account route for
 * active-state purposes, even when Profile deep-links past it.
 */
export const PROFILE_CLAIM_HREF = '/users/me'

/**
 * The notification inbox, named so the table entry below and the mobile Account
 * sheet — which finds that row to hang the unread badge on (PSY-1819) — agree
 * by identity rather than by matching string literals. A change of route moves
 * the badge with it. Other surfaces still inline the path (CommandPalette's
 * fork, the bell popover's "View all"); this covers the nav table only.
 */
export const NOTIFICATIONS_HREF = '/notifications'

/**
 * The PSY-1045 username-or-claim routing rule, in one place: with a username
 * the Profile destination deep-links to the public identity view
 * (`/users/<username>` — the same dense page visitors see, PSY-1025); without
 * one it routes to the claim-username self view.
 */
export function profileHref(user: { username?: string | null } | null | undefined): string {
  return user?.username ? `/users/${user.username}` : PROFILE_CLAIM_HREF
}

/** The Profile destination with its username-aware href resolved. */
function profileNavItem(
  user: { username?: string | null } | null | undefined
): NavDestination {
  return { href: profileHref(user), label: 'Profile', icon: UserCircle }
}

/**
 * The canonical account destination set. The desktop UserMenu dropdown and
 * the mobile Account sheet both render it in this order, with one chrome
 * difference: UserMenu renders the adminOnly partition after a separator in
 * its own group, while the Account sheet renders it inline — so a non-admin
 * entry added AFTER Admin would land above the separator on desktop but
 * below Admin on mobile. Keep Admin last. The side-nav rail's authed block
 * keeps its own deliberate list (composed via navDestination below) —
 * overlapping but neither a subset nor a superset: it drops
 * Notifications/Settings (the top bar's bell and UserMenu cover those in
 * side-nav mode) and adds Show Submissions and the desktop-only
 * Notification Filters.
 *
 * Admin stays in the list flagged `adminOnly` (surfaces filter it) so
 * active-state derivations see every account route regardless of viewer tier.
 * Profile → the public identity view; Settings → the /profile editor
 * (PSY-1486 split).
 */
export function accountNavItems(
  user: { username?: string | null } | null | undefined
): NavDestination[] {
  return [
    { href: NOTIFICATIONS_HREF, label: 'Notifications', icon: Bell },
    { href: '/library', label: 'My Library', icon: Library },
    profileNavItem(user),
    { href: '/profile', label: 'Settings', icon: Settings },
    // Reachable from the account menus in the DEFAULT top-bar mode — the side
    // rail's own Appearance entry only renders once already in side-nav mode.
    { href: '/settings/appearance', label: 'Appearance', icon: Palette },
    { href: '/admin', label: 'Admin', icon: Shield, adminOnly: true },
  ]
}

// Desktop-only by decision (PSY-1821): among the composed surfaces only the
// side-nav rail lists it (the command palette still forks its own copy — see
// the header note). On phones it stays reachable through the Account sheet →
// Notifications → the filters link on that page (app/notifications/page.tsx).
// Deliberately NOT in accountNavItems.
const notificationFiltersItem: NavDestination = {
  href: '/settings/notification-filters',
  label: 'Notification Filters',
  icon: Bell,
}

/**
 * Index destinations by href for composition lookups. Equal duplicates
 * collapse (Leaderboard legitimately appears in two desktop menus with one
 * definition). CONFLICTING duplicates — same href differing on ANY rendered
 * or behavioral field, the gating flags included — are the drift this module
 * exists to prevent, so they fail loudly at module load instead of
 * last-wins. Exported for its unit test only.
 */
export function buildDestinationIndex(
  sources: ReadonlyArray<readonly NavLink[]>
): Map<string, NavLink> {
  const index = new Map<string, NavLink>()
  for (const item of sources.flat()) {
    const existing = index.get(item.href)
    if (existing) {
      // Structural comparison over the key union, not an enumerated field
      // list — a field added to NavLink later is compared automatically
      // instead of silently reverting to last-wins. `?? false` folds the
      // undefined-vs-absent boolean flag case into one value.
      for (const key of new Set([...Object.keys(existing), ...Object.keys(item)])) {
        if (key === 'href') continue
        const a = existing[key as keyof NavLink]
        const b = item[key as keyof NavLink]
        if ((a ?? false) !== (b ?? false)) {
          throw new Error(
            `navData: conflicting definitions for destination "${item.href}" (${key})`
          )
        }
      }
    }
    index.set(item.href, item)
  }
  return index
}

const destinationByHref = buildDestinationIndex([
  primaryTabs,
  [graphItem, atlasItem],
  ...browseGroups.map(g => g.items),
  contributeItems,
  editorialItems,
  accountNavItems(null),
  [notificationFiltersItem],
])

/**
 * Resolve a destination from the canonical tables, optionally overriding
 * per-surface copy. Throws on an href the canonical tables don't define, so
 * a typo fails at module load (any test importing a composed table). Note
 * the guard is table membership only — it does not verify the route exists
 * in app/, so a canonical href pointing at a renamed route still ships.
 */
export function navDestination(
  href: string,
  overrides?: { label?: string }
): NavDestination {
  const item = destinationByHref.get(href)
  if (!item) throw new Error(`navData: unknown nav destination "${href}"`)
  if (!item.icon) throw new Error(`navData: destination "${href}" has no icon`)
  return { ...(item as NavDestination), ...overrides }
}

// The side-nav rail's tables (PSY-1821: folded in from Sidebar.tsx). The rail
// keeps its own curated order and grouping; every entry resolves through the
// canonical tables, and the two label overrides are the rail's flat-chrome
// copy, decided here rather than forked in the component.
export const sidebarGroups: NavDestinationGroup[] = [
  {
    label: 'Discover',
    items: [
      navDestination('/shows'),
      navDestination('/festivals'),
      navDestination('/artists'),
      navDestination('/venues'),
      navDestination('/graph'),
      navDestination('/releases'),
      navDestination('/labels'),
      navDestination('/tags'),
      // The rail links the scenes INDEX; "All scenes" is Browse-menu copy for
      // a list that sits under a "Scenes" group label.
      navDestination('/scenes', { label: 'Scenes' }),
      navDestination('/atlas'),
      navDestination('/collections'),
      navDestination('/charts'),
      navDestination('/radio'),
    ],
  },
  {
    label: 'Community',
    items: [
      // "Contribute hub" is Contribute-menu copy; the rail keeps the short label.
      navDestination('/contribute', { label: 'Contribute' }),
      navDestination('/community/leaderboard'),
      navDestination('/requests'),
      navDestination('/blog'),
      navDestination('/dj-sets'),
      navDestination('https://psychichomily.substack.com/'),
      // The rail renders this plain — SidebarNavLink ignores submitPrimary;
      // the CTA color treatment is menu/sheet chrome.
      navDestination('/shows/submit'),
      navDestination('/submissions'),
    ],
  },
]

// The rail's authed block is mostly user-invariant — resolve those entries
// once at module load (which also makes navDestination's fail-at-load
// guarantee hold for them).
const sidebarAccountStatic: NavDestination[] = [
  navDestination('/library'),
  navDestination('/contribute/submissions'),
  navDestination('/settings/notification-filters'),
  navDestination('/settings/appearance'),
]
const sidebarAdminItem = navDestination('/admin') // carries adminOnly from the account table

/**
 * The side-nav rail's authed block, in rail order. Its membership is
 * deliberately NOT accountNavItems (see that function's doc): it adds Show
 * Submissions (borrowed from contributeItems, authOnly and all) and the
 * desktop-only Notification Filters, and drops Notifications/Settings. Same
 * contract as accountNavItems: Admin stays in the list flagged adminOnly —
 * filter with visibleNavItems at the render site.
 */
export function sidebarAccountItems(
  user: { username?: string | null } | null | undefined
): NavDestination[] {
  return [...sidebarAccountStatic, profileNavItem(user), sidebarAdminItem]
}

// All destinations a single nav menu links to — used to light up its trigger as
// active when the current route lives inside it.
export const browseHrefs = browseGroups.flatMap(g => g.items.map(i => i.href))
export const contributeHrefs = [...contributeItems, ...editorialItems]
  .filter(i => !i.external)
  .map(i => i.href)

/** Active when the path is the link or a descendant of it ("/" matches only "/"). */
export function isNavActive(pathname: string, href: string): boolean {
  if (href === '/') return pathname === '/'
  return pathname === href || pathname.startsWith(href + '/')
}

/**
 * Shared style for top-bar nav items and menu triggers, matching the Figma
 * Navigation design: medium-weight muted by default, semibold foreground when
 * active. Reused by plain links (PrimaryNav) and menu triggers so the row reads
 * as one consistent set.
 */
export function navItemClassName(active?: boolean): string {
  return cn(
    'inline-flex items-center gap-1 whitespace-nowrap rounded-sm text-[15px] outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/50',
    active
      ? 'font-semibold text-foreground'
      : 'font-medium text-muted-foreground hover:text-foreground'
  )
}

// PSY-1020 — mobile bottom tab bar (Option A, Figma Navigation 540:8).
// The Browse tab's long-tail sheet: every desktop menu destination, composed
// from the canonical tables above (no forked destination lists). Graph and
// Atlas are desktop *primary* links (PrimaryNav) with no home in the
// Browse/Contribute menus, so they get a leading group here — without it those
// destinations would be unreachable on mobile, where the primary tabs only
// carry Home/Shows/Radio.
// Deduped at composition: Leaderboard legitimately appears in both Curation
// and Contribute on desktop (two separate menus), but this is ONE sheet — a
// destination renders once, under the first group that claims it. (IIFE keeps
// the seen-set out of module scope so nothing can consume it half-drained.)
export const mobileBrowseGroups: NavGroup[] = (() => {
  const seen = new Set<string>()
  return [
    {
      label: 'Discover',
      items: [graphItem, atlasItem],
    },
    ...browseGroups,
    { label: 'Contribute', items: contributeItems },
    { label: 'Editorial', items: editorialItems },
  ].map(group => ({
    label: group.label,
    items: group.items.filter(item => {
      if (seen.has(item.href)) return false
      seen.add(item.href)
      return true
    }),
  }))
})()

// Lights the Browse tab as active when the route lives in its sheet (external
// links excluded; they never match a pathname).
export const mobileBrowseHrefs = mobileBrowseGroups.flatMap(g =>
  g.items.filter(i => !i.external).map(i => i.href)
)

/**
 * Row style for links inside bottom sheets and drawers (BottomTabBar's
 * Browse/Account sheets, AdminDrawerNav) — one definition so the phone-chrome
 * surfaces can't drift apart (PSY-1020 /simplify).
 */
export function sheetLinkClassName(active: boolean): string {
  return cn(
    'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
    active
      ? 'bg-accent text-accent-foreground'
      : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
  )
}

/**
 * The mono uppercase group-label token shared by the desktop menus
 * (BrowseMenu/ContributeMenu) and the mobile Browse sheet. Callers add their
 * own layout classes (margins/padding) on top.
 */
export const navGroupLabelClassName =
  'font-mono text-[11px] font-bold uppercase tracking-[1.2px] text-muted-foreground'
