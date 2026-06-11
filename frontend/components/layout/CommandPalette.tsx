'use client'

import { useCallback, useMemo, useState } from 'react'
import { useRouter, usePathname } from 'next/navigation'
import { Command as CommandPrimitive } from 'cmdk'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
// PSY-1019: rows are icon-less in the editorial re-skin (Figma 539:5) — the
// uppercase mono group headings carry the wayfinding, so names sit flush-left.
// The Recent clock is the one deliberate exception: in the no-query state it
// distinguishes "something you searched before" from a live result.
import { Search, Clock, X, Loader2 } from 'lucide-react'
import { adminNavGroups, adminTabHref } from '@/components/layout/adminNav'
import type { AdminTab } from '@/components/layout/adminNav'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useCommandPalette } from '@/lib/hooks/common/useCommandPalette'
import {
  useEntitySearch,
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE,
} from '@/lib/hooks/common/useEntitySearch'
import type { EntitySearchResult } from '@/lib/hooks/common/useEntitySearch'
import { GRAPH_HASH } from '@/lib/hooks/common/useUrlHash'
import { TagOfficialIndicator } from '@/features/tags'
import { InlineErrorBanner } from '@/components/shared'

interface RouteItem {
  label: string
  href: string
  keywords: string[]
  requireAuth?: boolean
  requireAdmin?: boolean
}

const routes: RouteItem[] = [
  {
    label: 'Shows',
    href: '/shows',
    keywords: ['shows', 'concerts', 'events', 'live', 'music', 'gigs'],
  },
  {
    label: 'Festivals',
    href: '/festivals',
    keywords: ['festivals', 'fests', 'lineup', 'multi-day', 'outdoor', 'music festival'],
  },
  {
    label: 'Artists',
    href: '/artists',
    keywords: ['artists', 'bands', 'musicians', 'performers', 'graph', 'explore', 'network', 'visualize', 'related', 'similar'],
  },
  {
    label: 'Venues',
    href: '/venues',
    keywords: ['venues', 'locations', 'places', 'bars', 'clubs', 'graph', 'explore', 'network', 'visualize', 'co-bill'],
  },
  {
    label: 'Releases',
    href: '/releases',
    keywords: ['releases', 'albums', 'records', 'eps', 'singles', 'discography', 'music'],
  },
  {
    label: 'Labels',
    href: '/labels',
    keywords: ['labels', 'record labels', 'imprints', 'roster', 'catalog'],
  },
  {
    label: 'Tags',
    href: '/tags',
    keywords: ['tags', 'genres', 'moods', 'styles', 'categories', 'tagging'],
  },
  {
    label: 'Scenes',
    href: '/scenes',
    keywords: ['scenes', 'cities', 'city', 'local', 'geographic', 'phoenix', 'music scene', 'graph', 'explore', 'network', 'visualize'],
  },
  {
    label: 'Collections',
    href: '/collections',
    keywords: ['collections', 'curated', 'lists', 'playlists', 'graph', 'explore', 'network', 'visualize'],
  },
  {
    // PSY-366: Phoenix scene graph deep-link. Phoenix is the only scene with
    // dense-enough data for the graph viz today (memory: ≤11 artists in every
    // other scene). Re-evaluate when a second city hits scene-scale density.
    label: 'Phoenix scene graph',
    href: `/scenes/phoenix-az${GRAPH_HASH}`,
    keywords: ['graph', 'explore', 'network', 'visualize', 'phoenix', 'scene', 'arizona', 'az'],
  },
  {
    label: 'Charts',
    href: '/charts',
    keywords: ['charts', 'trending', 'popular', 'top', 'hot', 'rankings', 'leaderboard'],
  },
  {
    label: 'Radio',
    href: '/radio',
    keywords: ['radio', 'stations', 'shows', 'playlists', 'kexp', 'broadcast', 'fm', 'stream'],
  },
  {
    label: 'Contribute',
    href: '/contribute',
    keywords: ['contribute', 'help', 'data quality', 'missing', 'opportunities', 'improve'],
  },
  {
    label: 'Leaderboard',
    href: '/community/leaderboard',
    keywords: ['leaderboard', 'top', 'contributors', 'rankings', 'community', 'competition'],
  },
  {
    label: 'Requests',
    href: '/requests',
    keywords: ['requests', 'request', 'wanted', 'missing', 'suggest', 'ask'],
  },
  {
    label: 'Blog',
    href: '/blog',
    keywords: ['blog', 'posts', 'articles', 'writing', 'news'],
  },
  {
    label: 'DJ Sets',
    href: '/dj-sets',
    keywords: ['dj', 'sets', 'mixes', 'electronic'],
  },
  {
    label: 'Submit a Show',
    href: '/shows/submit',
    keywords: ['submit', 'add', 'new show', 'submission', 'submit a show'],
  },
  {
    label: 'My Submissions',
    href: '/submissions',
    keywords: ['submissions', 'pending', 'edits', 'my submissions', 'my edits', 'my pending edits'],
  },
  {
    label: 'Library',
    href: '/library',
    keywords: ['library', 'saved', 'bookmarks', 'favorites', 'following', 'my stuff', 'personal', 'my shows', 'going', 'interested', 'attending', 'my collection', 'submissions', 'my submissions'],
    requireAuth: true,
  },
  {
    label: 'Notifications',
    href: '/notifications',
    keywords: ['notifications', 'inbox', 'bell', 'replies', 'mentions', 'unread'],
    requireAuth: true,
  },
  {
    label: 'Notification Filters',
    href: '/settings/notification-filters',
    keywords: ['notification filters', 'notify', 'filters', 'alerts', 'subscribe', 'show alerts'],
    requireAuth: true,
  },
  {
    label: 'Settings',
    href: '/profile',
    keywords: ['settings', 'profile', 'account', 'preferences', 'email'],
    requireAuth: true,
  },
  {
    // /users/me redirects to /users/<username> when one is set, and renders
    // the claim-username self view otherwise — so this static entry works for
    // every authed user without needing the username here (PSY-1045).
    label: 'My Profile',
    href: '/users/me',
    keywords: ['my profile', 'profile', 'public profile', 'identity', 'me'],
    requireAuth: true,
  },
]

/**
 * Cmd-K search keywords per admin section, keyed by AdminTab so a new section
 * (added to VALID_TABS in adminNav.ts) is a compile error here until it gets
 * search terms — the palette can never silently ship a keyword-less admin entry.
 * 'admin' is prepended to every entry by the derivation below, so it's omitted here.
 */
const ADMIN_ROUTE_KEYWORDS: Record<AdminTab, string[]> = {
  dashboard: ['dashboard', 'overview', 'stats'],
  moderation: ['moderation', 'queue', 'review', 'pending', 'edits', 'reports', 'moderate', 'venue'],
  'pending-shows': ['pending', 'shows', 'approve', 'review', 'moderate'],
  'unverified-venues': ['unverified', 'venues', 'verify'],
  reports: ['reports', 'flags', 'flagged', 'issues'],
  'import-show': ['import', 'show', 'add', 'upload'],
  releases: ['releases', 'albums', 'manage'],
  labels: ['labels', 'record labels', 'manage'],
  festivals: ['festivals', 'manage'],
  pipeline: ['pipeline', 'extraction', 'scraping', 'venues', 'data', 'import'],
  collections: ['collections', 'manage', 'featured'],
  tags: ['tags', 'manage', 'genres'],
  'data-quality': ['data', 'quality', 'health', 'issues'],
  analytics: ['analytics', 'metrics', 'growth', 'engagement'],
  'artists-admin': ['artists', 'manage', 'merge', 'aliases'],
  radio: ['radio', 'stations', 'matching', 'kexp', 'wfmu', 'nts', 'manage'],
  users: ['users', 'accounts', 'manage'],
  'audit-log': ['audit', 'log', 'history', 'actions'],
}

// PSY-934: derive the admin palette entries from the nav SSOT (adminNavGroups +
// adminTabHref in adminNav.ts) rather than maintaining a third hardcoded copy of
// the admin route list. The prior copy carried a dead `?tab=pending-venue-edits`
// link (not a valid section — pending edits live under Moderation) and was
// MISSING the Radio section. Deriving from the SSOT makes a stale/missing tab a
// compile error and keeps coverage in lockstep with the sidebar.
// adminNav.test.ts asserts the derived hrefs ⊆ VALID_TABS.
// Exported for the parity test (adminNav hrefs ⊆ VALID_TABS) in
// CommandPalette.test.tsx — not consumed elsewhere.
export const adminRoutes: RouteItem[] = adminNavGroups.flatMap(group =>
  group.items.map(item => ({
    label: `Admin: ${item.label}`,
    href: adminTabHref(item.tab),
    keywords: ['admin', ...ADMIN_ROUTE_KEYWORDS[item.tab]],
    requireAdmin: true,
  }))
)

const allRoutes = [...routes, ...adminRoutes]

/** Map entity type to display label for grouping */
const entityTypeLabels: Record<EntitySearchResult['entityType'], string> = {
  artist: 'Artists',
  venue: 'Venues',
  show: 'Shows',
  release: 'Releases',
  label: 'Labels',
  festival: 'Festivals',
  tag: 'Tags',
}

// PSY-1019 editorial group headings (Figma 539:5): Space Mono, uppercase,
// letter-spaced — mirrors ContributeMenu's group labels. Applied per-group so
// tailwind-merge resolves it against CommandGroup's default heading styles on
// the same element (deterministic; padding + muted color come from those
// defaults). The shared CommandGroup primitive stays untouched for CityFilters.
const groupClassName =
  '[&_[cmdk-group-heading]]:font-mono [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:font-bold [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-[1.2px]'

export function CommandPalette() {
  const router = useRouter()
  const pathname = usePathname()
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

  // Entity search — only active when palette is open and query is 2+ chars.
  // PSY-372: `totalResults` from the hook now includes shows (which the
  // collection Add Items panel surfaces). The palette intentionally does not
  // render shows, so we derive a palette-local `hasEntityResults` from the
  // visible groups below instead of using the hook's total directly.
  // `searchError` is true only when every backing endpoint failed in the
  // latest fetch — distinct from "no results" so we can swap copy.
  const { data: entityResults, isSearching, searchError } = useEntitySearch({
    query: search,
    enabled: open,
  })

  // Re-seed recent searches and clear the query each time the palette opens.
  // React 19.2: adjust state during render on the open false→true transition
  // via the canonical previous-value-guard idiom instead of a cascading
  // effect. `getRecentSearches` is a stable useCallback, so guarding on `open`
  // alone preserves the prior effect's semantics.
  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    if (open) {
      setRecentSearches(getRecentSearches())
      setSearch('')
    }
  }

  const availableRoutes = useMemo(() => {
    return routes.filter(route => {
      if (route.requireAuth) return isAuthenticated
      return true
    })
  }, [isAuthenticated])

  // PSY-366: context-aware "Explore graph" entries based on current page.
  // When the palette opens on an entity page that has a graph view, surface
  // a deep-link to that entity's graph at the top of the list. Uses #graph
  // anchor so deep-links work without new routes — the destination page is
  // expected to have an `id="graph"` wrapper.
  const contextualRoutes = useMemo<RouteItem[]>(() => {
    if (!pathname) return []
    const items: RouteItem[] = []
    const artistMatch = pathname.match(/^\/artists\/([^/]+)/)
    if (artistMatch && artistMatch[1] !== '') {
      items.push({
        label: 'Explore graph for this artist',
        href: `/artists/${artistMatch[1]}${GRAPH_HASH}`,
        keywords: ['graph', 'explore', 'network', 'visualize', 'related', 'similar'],
      })
    }
    const collectionMatch = pathname.match(/^\/collections\/([^/]+)/)
    if (collectionMatch && collectionMatch[1] !== '') {
      items.push({
        label: 'Explore graph for this collection',
        href: `/collections/${collectionMatch[1]}${GRAPH_HASH}`,
        keywords: ['graph', 'explore', 'network', 'visualize'],
      })
    }
    const sceneMatch = pathname.match(/^\/scenes\/([^/]+)/)
    if (sceneMatch && sceneMatch[1] !== '') {
      items.push({
        label: 'Explore graph for this scene',
        href: `/scenes/${sceneMatch[1]}${GRAPH_HASH}`,
        keywords: ['graph', 'explore', 'network', 'visualize'],
      })
    }
    const venueMatch = pathname.match(/^\/venues\/([^/]+)/)
    if (venueMatch && venueMatch[1] !== '') {
      items.push({
        label: 'Explore graph for this venue',
        href: `/venues/${venueMatch[1]}${GRAPH_HASH}`,
        keywords: ['graph', 'explore', 'network', 'visualize', 'co-bill'],
      })
    }
    return items
  }, [pathname])

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
  const showEntityResults = search.trim().length >= 2

  // Collect entity result groups that have results, in display order.
  // PSY-372: `show` is intentionally excluded — the palette does not surface
  // shows beyond the static `/shows` route entry. Show results from the
  // shared hook are simply ignored here.
  const entityGroups = useMemo(() => {
    if (!entityResults) return []
    const types = ['artist', 'venue', 'release', 'label', 'festival', 'tag'] as const
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

  // Derived from the visible groups so excluded entity types (shows) don't
  // flip the empty-state copy when they're the only thing that matched.
  const hasEntityResults = entityGroups.length > 0

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <div className="flex items-center border-b border-border/50 px-3">
        <Search className="mr-2 h-4 w-4 shrink-0 opacity-50" />
        <CommandPrimitive.Input
          placeholder="Search entities or go to page..."
          className="flex h-11 w-full bg-transparent py-3 text-[15px] outline-none placeholder:text-muted-foreground"
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
          {showEntityResults && !hasEntityResults && !isSearching && !searchError
            ? 'No matching entities or pages.'
            : 'No matching pages.'}
        </CommandEmpty>

        {/* Total search outage — every backing endpoint rejected. Show
            before any other groups so users see why their results
            collapsed instead of retyping against a dead backend. */}
        {showEntityResults && searchError && (
          <InlineErrorBanner
            className="mx-2 my-2"
            testId="entity-search-error-banner"
          >
            {ENTITY_SEARCH_UNAVAILABLE_MESSAGE}
          </InlineErrorBanner>
        )}

        {showRecent && (
          <CommandGroup
            className={groupClassName}
            heading={
              <div className="flex items-center justify-between">
                <span>Recent</span>
                <button
                  onClick={handleClearRecent}
                  // font-sans + normal-case break the inheritance from the
                  // mono/uppercase group-heading styles — this is an action,
                  // not a heading.
                  className="font-sans text-[10px] font-normal normal-case tracking-normal text-muted-foreground hover:text-foreground"
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
                  className="cursor-pointer gap-3 rounded-sm px-2 py-2.5 text-[15px]"
                  keywords={[label]}
                >
                  <Clock className="h-4 w-4 text-muted-foreground" />
                  <span>{label}</span>
                  {route && (
                    <span className="ml-auto font-mono text-xs text-muted-foreground">
                      {route.href}
                    </span>
                  )}
                </CommandItem>
              )
            })}
          </CommandGroup>
        )}

        {/* Entity search results — shown when user types 2+ characters.
            PSY-1019: name flush-left, subtitle right-aligned per Figma 539:5
            (the href is gone — the subtitle is the scent, the row navigates). */}
        {showEntityResults && entityGroups.map(({ type, results }) => {
          const groupLabel = entityTypeLabels[type]
          return (
            <CommandGroup key={type} className={groupClassName} heading={groupLabel}>
              {results.map(result => (
                <CommandItem
                  key={`entity-${type}-${result.id}`}
                  value={`entity-${type}-${result.id}-${result.name}`}
                  onSelect={() => handleEntitySelect(result)}
                  className="cursor-pointer gap-3 rounded-sm px-2 py-2.5 text-[15px]"
                  keywords={[result.name]}
                >
                  <span className="inline-flex min-w-0 items-center gap-1.5">
                    <span className="truncate">{result.name}</span>
                    {result.entityType === 'tag' && result.isOfficial && (
                      <TagOfficialIndicator size="sm" tagName={result.name} />
                    )}
                  </span>
                  {result.subtitle && (
                    <span className="ml-auto shrink-0 text-[13px] text-muted-foreground">
                      {result.subtitle}
                    </span>
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          )
        })}

        {/* PSY-366: context-aware graph entry points. Only rendered when the
            palette opens on an entity page that has a graph view. */}
        {contextualRoutes.length > 0 && (
          <CommandGroup className={groupClassName} heading="Explore">
            {contextualRoutes.map(route => (
              <CommandItem
                key={route.href}
                value={route.label}
                onSelect={() => handleSelect(route.href, route.label)}
                keywords={route.keywords}
                className="cursor-pointer gap-3 rounded-sm px-2 py-2.5 text-[15px]"
              >
                <span>{route.label}</span>
                <span className="ml-auto font-mono text-xs text-muted-foreground">
                  {route.href}
                </span>
              </CommandItem>
            ))}
          </CommandGroup>
        )}

        <CommandGroup className={groupClassName} heading="Pages">
          {availableRoutes.map(route => (
            <CommandItem
              key={route.href}
              value={route.label}
              onSelect={() => handleSelect(route.href, route.label)}
              keywords={route.keywords}
              className="cursor-pointer gap-3 rounded-sm px-2 py-2.5 text-[15px]"
            >
              <span>{route.label}</span>
              <span className="ml-auto font-mono text-xs text-muted-foreground">
                {route.href}
              </span>
            </CommandItem>
          ))}
        </CommandGroup>

        {availableAdminRoutes.length > 0 && (
          <CommandGroup className={groupClassName} heading="Admin">
            {availableAdminRoutes.map(route => (
              <CommandItem
                key={route.href}
                value={route.label}
                onSelect={() => handleSelect(route.href, route.label)}
                keywords={route.keywords}
                className="cursor-pointer gap-3 rounded-sm px-2 py-2.5 text-[15px]"
              >
                <span>{route.label}</span>
                <span className="ml-auto font-mono text-xs text-muted-foreground">
                  /admin
                </span>
              </CommandItem>
            ))}
          </CommandGroup>
        )}
      </CommandList>

      {/* PSY-1019: keyboard hints as plain Space Mono text — the boxed-kbd
          chrome is gone per Figma 539:5. <kbd> elements stay for semantics. */}
      <div className="flex items-center gap-4 border-t border-border/50 px-4 py-2.5 font-mono text-[11px] text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <kbd>&uarr;&darr;</kbd>
          navigate
        </span>
        <span className="flex items-center gap-1.5">
          <kbd>&crarr;</kbd>
          select
        </span>
        <span className="flex items-center gap-1.5">
          <kbd>esc</kbd>
          close
        </span>
      </div>
    </CommandDialog>
  )
}
