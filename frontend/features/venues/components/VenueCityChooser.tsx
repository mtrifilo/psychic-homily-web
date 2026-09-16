'use client'

import Link from 'next/link'
import type { CityWithCount } from '@/components/filters'
import { countLabel, venuesCityHref } from '../venuesListNavigation'

/** How many busiest cities the state offers as one-press shortcuts. */
const BUSIEST_CITY_COUNT = 10

export interface VenueCityChooserProps {
  /** Every city with rooms, in any order; this picks and orders its own. */
  cities: CityWithCount[]
  /** The params on screen, which every chip href carries but for the city. */
  params: { toString: () => string }
}

/**
 * What the directory shows a visitor it cannot place: a sentence and the
 * busiest cities, each a real link to that city's page.
 *
 * The chips are links rather than filter presses, so every city this state
 * offers is an address a reader can share and a crawler can follow, the same
 * way the rest of the directory is.
 *
 * EVERY chip carries its unit, not just the first: a reader who meets the
 * fourth chip first is otherwise left to name the bare number themselves.
 */
export function VenueCityChooser({ cities, params }: VenueCityChooserProps) {
  const busiest = [...cities]
    .sort((a, b) => b.count - a.count || a.city.localeCompare(b.city))
    .slice(0, BUSIEST_CITY_COUNT)

  return (
    <div data-testid="venues-city-chooser">
      <p className="mb-3 text-sm">Choose a city to see its rooms.</p>

      {busiest.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No cities to choose from yet.
        </p>
      )}

      {busiest.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <span
            id="venues-busiest-label"
            className="text-xs font-semibold uppercase tracking-wider text-muted-foreground"
          >
            Busiest
          </span>
          <ul
            aria-labelledby="venues-busiest-label"
            className="flex list-none flex-wrap gap-2"
          >
            {busiest.map(city => (
              <li key={`${city.city}-${city.state}`}>
                <Link
                  href={venuesCityHref(params, city.city, city.state)}
                  // The same words the chip shows, with the separator spoken
                  // as a pause rather than as a character: the visible text is
                  // hidden from assistive tech below, so these two are one
                  // label in two renderings and have to say the same thing.
                  aria-label={`${city.city}, ${city.state}, ${countLabel(city.count, 'room')}`}
                  data-testid={`venues-busiest-${city.city}-${city.state}`
                    .toLowerCase()
                    .replace(/\s+/g, '-')}
                  className="inline-flex min-h-11 items-center rounded-md border border-border/50 bg-muted/30 px-3 text-sm transition-colors hover:border-border hover:bg-muted"
                >
                  <span aria-hidden="true">
                    {city.city}, {city.state} &middot;{' '}
                    {countLabel(city.count, 'room')}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
