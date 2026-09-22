'use client'

import { createContext, useContext } from 'react'
import Link from 'next/link'
import { buildCitiesParam, cityLabel } from '@/components/filters/cityParams'
import { cn } from '@/lib/utils'
import type { CityState } from '@/components/filters'
import { useHomeShowCitySelection } from '@/features/shows/hooks/useHomeShowCitySelection'
import type { HomeCityLinkSlot } from '../sections'

/**
 * Which surface owns the "All upcoming shows in {city} →" link this render.
 *
 * The link belongs to the nearby section's header. Hiding that section would
 * otherwise take the home page's only route to the full show list with it, so
 * ownership falls back to the saved-shows footer and then to the toolbar row.
 * Defaults to 'nearby', which is where it lives on an untouched layout and on
 * any surface that mounts these sections without the layout runtime.
 */
const HomeCityLinkSlotContext = createContext<HomeCityLinkSlot>('nearby')

export const HomeCityLinkSlotProvider = HomeCityLinkSlotContext.Provider

export function useHomeCityLinkSlot(): HomeCityLinkSlot {
  return useContext(HomeCityLinkSlotContext)
}

/**
 * One city is the common case and the only one the approved copy names; a
 * multi-city selection links to all of them under the unqualified label rather
 * than picking one of the viewer's cities to speak for the rest.
 */
export function homeCityShowsLink(cities: readonly CityState[]): {
  href: string
  label: string
} {
  if (cities.length === 0) {
    return { href: '/shows', label: 'All upcoming shows →' }
  }
  const href = `/shows?cities=${encodeURIComponent(buildCitiesParam([...cities]))}`
  const label =
    cities.length === 1
      ? `All upcoming shows in ${cityLabel(cities[0])} →`
      : 'All upcoming shows →'
  return { href, label }
}

/** The home surfaces' primary-link chrome, shared so the relocated city link
 *  and the popover's "All settings" link cannot drift apart. */
export const PRIMARY_LINK_CLASS =
  'text-sm font-medium text-primary transition-colors hover:underline underline-offset-4'

/** Presentational form, for a caller that already owns a city selection. */
export function HomeCityShowsLink({
  cities,
  className,
}: {
  cities: readonly CityState[]
  className?: string
}) {
  const { href, label } = homeCityShowsLink(cities)
  return (
    <Link href={href} className={cn(PRIMARY_LINK_CLASS, className)}>
      {label}
    </Link>
  )
}

/**
 * Self-resolving form, for the fallback slots. Mounted only when the nearby
 * section is hidden, so it never doubles that section's own city resolution.
 */
export function ResolvedHomeCityShowsLink({ className }: { className?: string }) {
  const { effectiveCities, source, isResolving } = useHomeShowCitySelection({
    resolveCityForCopy: true,
  })
  // Held while the city is still being decided, the same way the nearby
  // section holds its subline. Painting the unqualified link first and
  // swapping in a city one moment later reflows the row it shares with the
  // toolbar kicker, and a click in that window loses the city filter.
  if (isResolving && source === 'none') return null
  return <HomeCityShowsLink cities={effectiveCities} className={className} />
}
