'use client'

import { useCallback, useMemo, useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { Command as CommandPrimitive } from 'cmdk'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import {
  Calendar, Mic2, MapPin, Disc3, Tag, Tags, Tent, BookOpen, Headphones, Send,
  Library, LayoutList, MessageSquarePlus, Settings, Search, Clock, X, Globe,
  TrendingUp, LayoutDashboard, Upload, BadgeCheck, Flag, ScrollText, Users, Workflow,
  ClipboardCheck, BarChart3, Music, Bell, HeartHandshake, ShieldCheck, Loader2, Trophy,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useCommandPalette } from '@/lib/hooks/common/useCommandPalette'
import { useEntitySearch } from '@/lib/hooks/common/useEntitySearch'
import type { EntitySearchResult } from '@/lib/hooks/common/useEntitySearch'

interface RouteItem {
  label: string
  href: string
  icon: LucideIcon
  keywords: string[]
  requireAuth?: boolean
  requireAdmin?: boolean
}

const routes: RouteItem[] = [
  {
    label: 'Shows',
    href: '/shows',
    icon: Calendar,
    keywords: ['shows', 'concerts', 'events', 'live', 'music', 'gigs'],
  },
  {
    label: 'Festivals',
    href: '/festivals',
    icon: Tent,
    keywords: ['festivals', 'fests', 'lineup', 'multi-day', 'outdoor', 'music festival'],
  },
  {
    label: 'Artists',
    href: '/artists',
    icon: Mic2,
    keywords: ['artists', 'bands', 'musicians', 'performers'],
  },
  {
    label: 'Venues',
    href: '/venues',
    icon: MapPin,
    keywords: ['venues', 'locations', 'places', 'bars', 'clubs'],
  },
  {
    label: 'Releases',
    href: '/releases',
    icon: Disc3,
    keywords: ['releases', 'albums', 'records', 'eps', 'singles', 'discography', 'music'],
  },
  {
    label: 'Labels',
    href: '/labels',
    icon: Tag,
    keywords: ['labels', 'record labels', 'imprints', 'roster', 'catalog'],
  },
  {
    label: 'Tags',
    href: '/tags',
    icon: Tags,
    keywords: ['tags', 'genres', 'moods', 'styles', 'categories', 'tagging'],
  },
  {
    label: 'Scenes',
    href: '/scenes',
    icon: Globe,
    keywords: ['scenes', 'cities', 'city', 'local', 'geographic', 'phoenix', 'music scene'],
  },
  {
    label: 'Crates',
    href: '/crates',
    icon: LayoutList,
    keywords: ['crates', 'curated', 'lists', 'playlists', 'collections'],
  },
  {
    label: 'Charts',
    href: '/charts',
    icon: TrendingUp,
    keywords: ['charts', 'trending', 'popular', 'top', 'hot', 'rankings', 'leaderboard'],
  },
  {
    label: 'Contribute',
    href: '/contribute',
    icon: HeartHandshake,
    keywords: ['contribute', 'help', 'data quality', 'missing', 'opportunities', 'improve'],
  },
  {
    label: 'Leaderboard',
    href: '/community/leaderboard',
    icon: Trophy,
    keywords: ['leaderboard', 'top', 'contributors', 'rankings', 'community', 'competition'],
  },
  {
    label: 'Requests',
    href: '/requests',
    icon: MessageSquarePlus,
    keywords: ['requests', 'request', 'wanted', 'missing', 'suggest', 'ask'],
  },
  {
    label: 'Blog',
    href: '/blog',
    icon: BookOpen,
    keywords: ['blog', 'posts', 'articles', 'writing', 'news'],
  },
  {
    label: 'DJ Sets',
    href: '/dj-sets',
    icon: Headphones,
    keywords: ['dj', 'sets', 'mixes', 'electronic'],
  },
  {
    label: 'Submissions',
    href: '/submissions',
    icon: Send,
    keywords: ['submissions', 'submit', 'add', 'new show'],
  },
  {
    label: 'Library',
    href: '/library',
    icon: Library,
    keywords: ['library', 'saved', 'bookmarks', 'favorites', 'following', 'my stuff', 'personal', 'my shows', 'going', 'interested', 'attending'],
    requireAuth: true,
  },
  {
    label: 'Collection',
    href: '/collection',
    icon: BookOpen,
    keywords: ['collection', 'saved', 'my list', 'favorites', 'bookmarks'],
    requireAuth: true,
  },
  {
    label: 'Notification Filters',
    href: '/settings/notifications',
    icon: Bell,
    keywords: ['notifications', 'notify', 'filters', 'alerts', 'bell', 'subscribe'],
    requireAuth: true,
  },
  {
    label: 'Settings',
    href: '/profile',
    icon: Settings,
    keywords: ['settings', 'profile', 'account', 'preferences', 'email'],
    requireAuth: true,
  },
]

const adminRoutes: RouteItem[] = [
  {
    label: 'Admin: Dashboard',
    href: '/admin',
    icon: LayoutDashboard,
    keywords: ['admin', 'dashboard', 'overview', 'stats'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Moderation Queue',
    href: '/admin?tab=moderation',
    icon: ShieldCheck,
    keywords: ['admin', 'moderation', 'queue', 'review', 'pending', 'edits', 'reports', 'moderate'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Pending Shows',
    href: '/admin?tab=pending-shows',
    icon: Clock,
    keywords: ['admin', 'pending', 'shows', 'approve', 'review', 'moderate'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Venue Edits',
    href: '/admin?tab=pending-venue-edits',
    icon: MapPin,
    keywords: ['admin', 'venue', 'edits', 'pending', 'approve'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Unverified Venues',
    href: '/admin?tab=unverified-venues',
    icon: BadgeCheck,
    keywords: ['admin', 'unverified', 'venues', 'verify'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Reports',
    href: '/admin?tab=reports',
    icon: Flag,
    keywords: ['admin', 'reports', 'flags', 'flagged', 'issues'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Import Show',
    href: '/admin?tab=import-show',
    icon: Upload,
    keywords: ['admin', 'import', 'show', 'add', 'upload'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Releases',
    href: '/admin?tab=releases',
    icon: Disc3,
    keywords: ['admin', 'releases', 'albums', 'manage'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Labels',
    href: '/admin?tab=labels',
    icon: Tag,
    keywords: ['admin', 'labels', 'record labels', 'manage'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Festivals',
    href: '/admin?tab=festivals',
    icon: Tent,
    keywords: ['admin', 'festivals', 'manage'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Data Pipeline',
    href: '/admin?tab=pipeline',
    icon: Workflow,
    keywords: ['admin', 'pipeline', 'extraction', 'scraping', 'venues', 'data', 'import'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Crates',
    href: '/admin?tab=crates',
    icon: Library,
    keywords: ['admin', 'crates', 'manage', 'featured', 'collections'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Tags',
    href: '/admin?tab=tags',
    icon: Tags,
    keywords: ['admin', 'tags', 'manage', 'genres'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Data Quality',
    href: '/admin?tab=data-quality',
    icon: ClipboardCheck,
    keywords: ['admin', 'data', 'quality', 'health', 'issues'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Analytics',
    href: '/admin?tab=analytics',
    icon: BarChart3,
    keywords: ['admin', 'analytics', 'metrics', 'growth', 'engagement'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Artists',
    href: '/admin?tab=artists-admin',
    icon: Music,
    keywords: ['admin', 'artists', 'manage', 'merge', 'aliases'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Users',
    href: '/admin?tab=users',
    icon: Users,
    keywords: ['admin', 'users', 'accounts', 'manage'],
    requireAdmin: true,
  },
  {
    label: 'Admin: Audit Log',
    href: '/admin?tab=audit-log',
    icon: ScrollText,
    keywords: ['admin', 'audit', 'log', 'history', 'actions'],
    requireAdmin: true,
  },
]

const allRoutes = [...routes, ...adminRoutes]

/** Map entity type to an icon */
const entityTypeIcons: Record<EntitySearchResult['entityType'], LucideIcon> = {
  artist: Mic2,
  venue: MapPin,
  release: Disc3,
  label: Tag,
  festival: Tent,
}

/** Map entity type to display label for grouping */
const entityTypeLabels: Record<EntitySearchResult['entityType'], string> = {
  artist: 'Artists',
  venue: 'Venues',
  release: 'Releases',
  label: 'Labels',
  festival: 'Festivals',
}

export function CommandPalette() {
  const router = useRouter()
  const { user, isAuthenticated } = useAuthContext()
  const {
    open,
    setOpen,
    getRecentSearches,
    addRecentSearch,
    clearRecentSearches,
  } = useCommandPalette()

  const [recentSearches, setRecentSearches] = useState<string[]>([])
  const [search, setSearch] = useState('')

  // Entity search — only active when palette is open and query is 2+ chars
  const { data: entityResults, isSearching, totalResults } = useEntitySearch({
    query: search,
    enabled: open,
  })

  useEffect(() => {
    if (open) {
      setRecentSearches(getRecentSearches())
      setSearch('')
    }
  }, [open, getRecentSearches])

  const availableRoutes = useMemo(() => {
    return routes.filter(route => {
      if (route.requireAuth) return isAuthenticated
      return true
    })
  }, [isAuthenticated])

  const availableAdminRoutes = useMemo(() => {
    if (!user?.is_admin) return []
    return adminRoutes
  }, [user?.is_admin])

  const handleSelect = useCallback(
    (href: string, label: string) => {
      addRecentSearch(label)
      setOpen(false)
      router.push(href)
    },
    [router, setOpen, addRecentSearch]
  )

  const handleEntitySelect = useCallback(
    (result: EntitySearchResult) => {
      addRecentSearch(result.name)
      setOpen(false)
      router.push(result.href)
    },
    [router, setOpen, addRecentSearch]
  )

  const handleRecentSelect = useCallback(
    (label: string) => {
      const route = allRoutes.find(
        r => r.label.toLowerCase() === label.toLowerCase()
      )
      if (route) {
        handleSelect(route.href, route.label)
      }
    },
    [handleSelect]
  )

  const handleClearRecent = useCallback(() => {
    clearRecentSearches()
    setRecentSearches([])
  }, [clearRecentSearches])

  const showRecent = !search && recentSearches.length > 0
  const hasEntityResults = totalResults > 0
  const showEntityResults = search.trim().length >= 2

  // Collect entity result groups that have results, in display order
  const entityGroups = useMemo(() => {
    if (!entityResults) return []
    const types = ['artist', 'venue', 'release', 'label', 'festival'] as const
    const groups: { type: EntitySearchResult['entityType']; results: EntitySearchResult[] }[] = []
    for (const type of types) {
      const key = `${type}s` as keyof typeof entityResults
      const results = entityResults[key]
      if (results && results.length > 0) {
        groups.push({ type, results })
      }
    }
    return groups
  }, [entityResults])

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <div className="flex items-center border-b border-border/50 px-3">
        <Search className="mr-2 h-4 w-4 shrink-0 opacity-50" />
        <CommandPrimitive.Input
          placeholder="Search entities or go to page..."
          className="flex h-11 w-full bg-transparent py-3 text-sm outline-none placeholder:text-muted-foreground"
          value={search}
          onValueChange={setSearch}
        />
        {isSearching && (
          <Loader2 className="ml-2 h-3.5 w-3.5 animate-spin text-muted-foreground" />
        )}
        {search && !isSearching && (
          <button
            onClick={() => setSearch('')}
            className="ml-2 rounded-sm p-1 text-muted-foreground hover:text-foreground"
            aria-label="Clear search"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      <CommandList className="max-h-[400px] p-2">
        <CommandEmpty>
          {showEntityResults && !hasEntityResults && !isSearching
            ? 'No matching entities or pages.'
            : 'No matching pages.'}
        </CommandEmpty>

        {showRecent && (
          <CommandGroup
            heading={
              <div className="flex items-center justify-between">
                <span>Recent</span>
                <button
                  onClick={handleClearRecent}
                  className="text-[10px] font-normal text-muted-foreground hover:text-foreground"
                >
                  Clear
                </button>
              </div>
            }
          >
            {recentSearches.map(label => {
              const route = allRoutes.find(
                r => r.label.toLowerCase() === label.toLowerCase()
              )
              return (
                <CommandItem
                  key={`recent-${label}`}
                  value={`recent-${label}`}
                  onSelect={() => handleRecentSelect(label)}
                  className="cursor-pointer gap-3 rounded-lg px-2 py-2.5"
                  keywords={[label]}
                >
                  <Clock className="h-4 w-4 text-muted-foreground" />
                  <span>{label}</span>
                  {route && (
                    <span className="ml-auto text-xs text-muted-foreground">
                      {route.href}
                    </span>
                  )}
                </CommandItem>
              )
            })}
          </CommandGroup>
        )}

        {showRecent && <CommandSeparator className="mx-2 my-1" />}

        {/* Entity search results — shown when user types 2+ characters */}
        {showEntityResults && entityGroups.map(({ type, results }) => {
          const Icon = entityTypeIcons[type]
          const groupLabel = entityTypeLabels[type]
          return (
            <CommandGroup key={type} heading={groupLabel}>
              {results.map(result => (
                <CommandItem
                  key={`entity-${type}-${result.id}`}
                  value={`entity-${type}-${result.id}-${result.name}`}
                  onSelect={() => handleEntitySelect(result)}
                  className="cursor-pointer gap-3 rounded-lg px-2 py-2.5"
                  keywords={[result.name]}
                >
                  <Icon className="h-4 w-4 text-muted-foreground" />
                  <div className="flex flex-col gap-0.5 min-w-0">
                    <span className="truncate">{result.name}</span>
                    {result.subtitle && (
                      <span className="text-xs text-muted-foreground truncate">
                        {result.subtitle}
                      </span>
                    )}
                  </div>
                  <span className="ml-auto shrink-0 text-xs text-muted-foreground">
                    {result.href}
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          )
        })}

        {showEntityResults && hasEntityResults && (
          <CommandSeparator className="mx-2 my-1" />
        )}

        <CommandGroup heading="Pages">
          {availableRoutes.map(route => {
            const Icon = route.icon
            return (
              <CommandItem
                key={route.href}
                value={route.label}
                onSelect={() => handleSelect(route.href, route.label)}
                keywords={route.keywords}
                className="cursor-pointer gap-3 rounded-lg px-2 py-2.5"
              >
                <Icon className="h-4 w-4 text-muted-foreground" />
                <span>{route.label}</span>
                <span className="ml-auto text-xs text-muted-foreground">
                  {route.href}
                </span>
              </CommandItem>
            )
          })}
        </CommandGroup>

        {availableAdminRoutes.length > 0 && (
          <>
            <CommandSeparator className="mx-2 my-1" />
            <CommandGroup heading="Admin">
              {availableAdminRoutes.map(route => {
                const Icon = route.icon
                return (
                  <CommandItem
                    key={route.href}
                    value={route.label}
                    onSelect={() => handleSelect(route.href, route.label)}
                    keywords={route.keywords}
                    className="cursor-pointer gap-3 rounded-lg px-2 py-2.5"
                  >
                    <Icon className="h-4 w-4 text-muted-foreground" />
                    <span>{route.label}</span>
                    <span className="ml-auto text-xs text-muted-foreground">
                      /admin
                    </span>
                  </CommandItem>
                )
              })}
            </CommandGroup>
          </>
        )}
      </CommandList>

      <div className="flex items-center justify-between border-t border-border/50 px-3 py-2">
        <div className="flex items-center gap-3 text-[11px] text-muted-foreground">
          <span className="flex items-center gap-1">
            <kbd className="rounded border border-border bg-muted px-1 py-0.5 font-mono text-[10px]">
              &uarr;
            </kbd>
            <kbd className="rounded border border-border bg-muted px-1 py-0.5 font-mono text-[10px]">
              &darr;
            </kbd>
            navigate
          </span>
          <span className="flex items-center gap-1">
            <kbd className="rounded border border-border bg-muted px-1 py-0.5 font-mono text-[10px]">
              &crarr;
            </kbd>
            select
          </span>
          <span className="flex items-center gap-1">
            <kbd className="rounded border border-border bg-muted px-1 py-0.5 font-mono text-[10px]">
              esc
            </kbd>
            close
          </span>
        </div>
      </div>
    </CommandDialog>
  )
}
