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
  Calendar, Mic2, MapPin, Disc3, BookOpen, Headphones, Send,
  Library, Settings, Shield, Search, Clock, X,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useCommandPalette } from '@/lib/hooks/useCommandPalette'

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
    label: 'Collection',
    href: '/collection',
    icon: Library,
    keywords: ['collection', 'saved', 'my list', 'favorites', 'bookmarks'],
    requireAuth: true,
  },
  {
    label: 'Settings',
    href: '/profile',
    icon: Settings,
    keywords: ['settings', 'profile', 'account', 'preferences', 'email'],
    requireAuth: true,
  },
  {
    label: 'Admin',
    href: '/admin',
    icon: Shield,
    keywords: ['admin', 'dashboard', 'manage', 'moderate'],
    requireAdmin: true,
  },
]

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

  useEffect(() => {
    if (open) {
      setRecentSearches(getRecentSearches())
      setSearch('')
    }
  }, [open, getRecentSearches])

  const availableRoutes = useMemo(() => {
    return routes.filter(route => {
      if (route.requireAdmin) return user?.is_admin
      if (route.requireAuth) return isAuthenticated
      return true
    })
  }, [isAuthenticated, user?.is_admin])

  const handleSelect = useCallback(
    (href: string, label: string) => {
      addRecentSearch(label)
      setOpen(false)
      router.push(href)
    },
    [router, setOpen, addRecentSearch]
  )

  const handleRecentSelect = useCallback(
    (label: string) => {
      const route = routes.find(
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

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <div className="flex items-center border-b border-border/50 px-3">
        <Search className="mr-2 h-4 w-4 shrink-0 opacity-50" />
        <CommandPrimitive.Input
          placeholder="Search pages..."
          className="flex h-11 w-full bg-transparent py-3 text-sm outline-none placeholder:text-muted-foreground"
          value={search}
          onValueChange={setSearch}
        />
        {search && (
          <button
            onClick={() => setSearch('')}
            className="ml-2 rounded-sm p-1 text-muted-foreground hover:text-foreground"
            aria-label="Clear search"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      <CommandList className="max-h-[320px] p-2">
        <CommandEmpty>No results found.</CommandEmpty>

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
              const route = routes.find(
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
