import {
  LayoutDashboard, ShieldCheck, Clock, BadgeCheck, Flag,
  Disc3, Tag, Tent, Music, Radio, Library, Tags,
  Upload, ClipboardCheck, BarChart3, Users, ScrollText,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { AdminNavCounts } from '@/lib/hooks/admin/useAdminNavCounts'

/**
 * Admin navigation model (PSY-933). The admin area swaps the global Sidebar's
 * public Discover/Community groups for these 6 grouped admin sections (the
 * context-aware-sidebar decision — see the PSY-933 Figma mock + ticket). Items
 * link to the existing `/admin?tab=` model rather than nested routes, so this is
 * a nav-chrome change only; `app/admin/page.tsx` still owns the tab content.
 */

/**
 * The admin section tabs — the `?tab=` values on /admin. Single source of truth
 * for valid sections: app/admin/page.tsx imports isValidTab to resolve the
 * active section, and adminNavGroups below must cover exactly these (enforced by
 * adminNav.test.ts). Adding a section = add it here + its TabsContent in
 * page.tsx + a nav item below. Lives here (not page.tsx) so the always-mounted
 * Sidebar can type its nav items against it without importing the page module.
 */
export const VALID_TABS = [
  'dashboard', 'moderation', 'pending-shows', 'unverified-venues',
  'reports', 'import-show', 'releases', 'labels', 'festivals',
  'collections', 'tags', 'data-quality', 'analytics', 'artists-admin', 'radio',
  'users', 'audit-log',
] as const

export type AdminTab = (typeof VALID_TABS)[number]

export function isValidTab(value: string | null): value is AdminTab {
  return value !== null && (VALID_TABS as readonly string[]).includes(value)
}

/** Sections whose tab carries an attention-count badge. Keys match AdminNavCounts. */
export type AdminBadgeKey = keyof AdminNavCounts

export interface AdminNavItem {
  /** The `?tab=` value on /admin. Typed against VALID_TABS so a typo or a
   *  renamed section is a compile error, not a silent dead nav link. */
  tab: AdminTab
  label: string
  icon: LucideIcon
  /** When set, the item shows the matching count from useAdminNavCounts. */
  badgeKey?: AdminBadgeKey
}

export interface AdminNavGroup {
  label: string
  items: AdminNavItem[]
}

export const adminNavGroups: AdminNavGroup[] = [
  {
    label: 'Overview',
    items: [{ tab: 'dashboard', label: 'Dashboard', icon: LayoutDashboard }],
  },
  {
    label: 'Moderation & Queues',
    items: [
      { tab: 'moderation', label: 'Moderation', icon: ShieldCheck, badgeKey: 'moderation' },
      { tab: 'pending-shows', label: 'Pending Shows', icon: Clock, badgeKey: 'pendingShows' },
      { tab: 'unverified-venues', label: 'Unverified Venues', icon: BadgeCheck, badgeKey: 'unverifiedVenues' },
      { tab: 'reports', label: 'Reports', icon: Flag, badgeKey: 'reports' },
    ],
  },
  {
    label: 'Catalog',
    items: [
      { tab: 'releases', label: 'Releases', icon: Disc3 },
      { tab: 'labels', label: 'Labels', icon: Tag },
      { tab: 'festivals', label: 'Festivals', icon: Tent },
      { tab: 'artists-admin', label: 'Artists', icon: Music },
      { tab: 'radio', label: 'Radio Stations', icon: Radio },
    ],
  },
  {
    label: 'Curation & Taxonomy',
    items: [
      { tab: 'collections', label: 'Collections', icon: Library },
      { tab: 'tags', label: 'Tags', icon: Tags },
    ],
  },
  {
    label: 'Tools',
    items: [
      { tab: 'import-show', label: 'Import Show', icon: Upload },
      { tab: 'data-quality', label: 'Data Quality', icon: ClipboardCheck },
    ],
  },
  {
    label: 'Insights & System',
    items: [
      { tab: 'analytics', label: 'Analytics', icon: BarChart3 },
      { tab: 'users', label: 'Users', icon: Users },
      { tab: 'audit-log', label: 'Audit Log', icon: ScrollText },
    ],
  },
]

/** The /admin URL for a section. Dashboard is the bare /admin (no tab param). */
export function adminTabHref(tab: string): string {
  return tab === 'dashboard' ? '/admin' : `/admin?tab=${tab}`
}

/**
 * Active-state test for an admin item against the current location. Mirrors
 * page.tsx's `isValidTab(tabParam) ? tabParam : 'dashboard'` resolution exactly:
 * a non-section tab param (missing, `dashboard`, or anything invalid like a
 * stale `pending-venue-edits` link) falls back to Dashboard, so Dashboard is the
 * active item there too — never a highlight/content mismatch. Other sections
 * match their exact tab param.
 */
export function isAdminTabActive(
  tab: AdminTab,
  pathname: string,
  tabParam: string | null
): boolean {
  if (!pathname.startsWith('/admin')) return false
  if (tab === 'dashboard') return tabParam === 'dashboard' || !isValidTab(tabParam)
  return tabParam === tab
}

/**
 * Per-queue badge color. Kept as today's ad-hoc Tailwind tints (purple / amber /
 * orange / red) to preserve the current information scent; tokenizing these to
 * the DS palette is the PSY-908 cohesion decision (the PSY-933 mock showed a
 * single illustrative token).
 */
export const ADMIN_BADGE_CLASS: Record<AdminBadgeKey, string> = {
  moderation: 'bg-purple-500',
  pendingShows: 'bg-amber-500',
  unverifiedVenues: 'bg-orange-500',
  reports: 'bg-red-500',
}
