import {
  Mic2, MapPin, Disc3, Tag, Tent, LayoutList, TrendingUp, Tags, Globe, Trophy,
  MessageSquarePlus, Music, Send, ClipboardList, HeartHandshake, BookOpen, Headphones, Newspaper,
  Compass, Map as MapIcon,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

// Shared link data + styling for the top-bar primary nav (PSY-1013). The retired
// left sidebar exposed ~20 destinations; these tables keep every one of them
// reachable from the new top bar's Browse / Contribute menus so retiring the
// sidebar doesn't regress desktop discoverability. The menu *presentation* is
// refined by follow-up tickets (Browse mega-menu → PSY-1014, Contribute →
// PSY-1015); Radio became a plain /radio link in PSY-1057. The destinations
// themselves live here.

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
// destination renders once, under the first group that claims it.
const seenMobileHrefs = new Set<string>()
export const mobileBrowseGroups: NavGroup[] = [
  {
    label: 'Discover',
    items: [
      { href: '/graph', label: 'Graph', icon: Compass },
      { href: '/atlas', label: 'Atlas', icon: MapIcon },
    ],
  },
  ...browseGroups,
  { label: 'Contribute', items: contributeItems },
  { label: 'Editorial', items: editorialItems },
].map(group => ({
  label: group.label,
  items: group.items.filter(item => {
    if (seenMobileHrefs.has(item.href)) return false
    seenMobileHrefs.add(item.href)
    return true
  }),
}))

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
