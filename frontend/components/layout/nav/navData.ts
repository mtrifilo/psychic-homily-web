import {
  Mic2, MapPin, Disc3, Tag, Tent, LayoutList, TrendingUp, Tags, Globe, Trophy,
  MessageSquarePlus, Music, Send, HeartHandshake, BookOpen, Headphones, Newspaper,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

// Shared link data + styling for the top-bar primary nav (PSY-1013). The retired
// left sidebar exposed ~20 destinations; these tables keep every one of them
// reachable from the new top bar's Browse / Contribute menus so retiring the
// sidebar doesn't regress desktop discoverability. The menu *presentation* is
// refined by follow-up tickets (Browse mega-menu → PSY-1014, Contribute →
// PSY-1015, Radio D2 panel → PSY-1016); the destinations themselves live here.

export interface NavLink {
  href: string
  label: string
  icon?: LucideIcon
  external?: boolean
  authOnly?: boolean
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
    label: 'Scenes',
    items: [{ href: '/scenes', label: 'All scenes', icon: Globe }],
  },
]

// Contribute ▾ — the What.cd "request system as call-to-action". PSY-1015
// refines presentation; Editorial (consume-not-contribute) is a labelled
// sub-group here pending its final home (Contribute vs. Browse → Curation).
export const contributeItems: NavLink[] = [
  { href: '/shows/submit', label: 'Submit a Show', icon: Music },
  { href: '/requests', label: 'Requests', icon: MessageSquarePlus },
  { href: '/submissions', label: 'My Submissions', icon: Send, authOnly: true },
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
