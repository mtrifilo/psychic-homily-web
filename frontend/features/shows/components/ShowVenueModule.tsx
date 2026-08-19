'use client'

import Link from 'next/link'
import { FollowButton } from '@/components/shared/FollowButton'
import { BracketLink } from '@/components/shared/BracketLink'
import { NotifyMeButton } from '@/features/notifications'
import { formatLocation, LOCATION_UNKNOWN } from '@/lib/formatLocation'
import { googleMapsSearchUrl } from '@/lib/maps'
import { MiddotSegments } from './MiddotSegments'
import { venueFactSegments } from './showVenueFacts'
import type { ShowResponse } from '../types'

interface ShowVenueModuleProps {
  show: ShowResponse
}

/**
 * The venue module of the show page: the co-primary entity's own block,
 * deliberately ABOVE the ticket module — the locked mock's scan order is
 * who → where/when → how much → social, and the venue must not be buried
 * under commerce.
 *
 * Three rows: name + address, the facts line, and the venue's verbs
 * (`[Directions ↗] [Follow venue] [Notify me] More at {venue} →`). Every row
 * degrades to omission when its data is absent; an unverified venue's street
 * address arrives redacted server-side and the module simply says less.
 *
 * The venue is derived HERE from `show.venues[0]` rather than accepted as a
 * prop, so the facts line (whose age/doors segments read the SHOW) can never
 * be printed beside a venue the show does not belong to. Renders nothing for
 * a venue-less show.
 */
export function ShowVenueModule({ show }: ShowVenueModuleProps) {
  const venue = show.venues[0]
  if (!venue) return null

  // Trimmed once: the API can hand back a whitespace-only address, which
  // would otherwise pass the truthiness check and render a stray comma.
  const venueAddress = venue.address?.trim()
  // Composed INTO a line, so the helper's stand-alone placeholder must be
  // recognised and dropped — "1357 N Elston Ave, Location Unknown" states
  // something this line was not asked to state.
  const formattedCityState = formatLocation({
    city: venue.city,
    state: venue.state,
  })
  const cityState =
    formattedCityState === LOCATION_UNKNOWN ? null : formattedCityState
  const factSegments = venueFactSegments(show, venue)
  // Directions works from whatever location facts exist — the query is
  // name-first, so a redacted street address degrades to a name + city
  // search rather than disappearing the affordance.
  const mapsUrl = googleMapsSearchUrl({
    name: venue.name,
    address: venueAddress,
    city: venue.city,
    state: venue.state,
  })

  return (
    <div className="mt-4" data-testid="show-venue-module">
      {/* Name + address, one line as the mock sets it. `venue-location` and
          `venue-address` testids survive from the pre-module markup: the bill
          above prints each act's hometown, so city/state text is not unique
          on this page and tests address the slots, not the strings. */}
      <div className="flex flex-wrap items-baseline gap-x-3">
        {venue.slug ? (
          <Link
            href={`/venues/${venue.slug}`}
            className="text-lg text-primary/80 hover:text-primary font-medium transition-colors"
          >
            {venue.name}
          </Link>
        ) : (
          <span className="text-lg text-primary/80 font-medium">
            {venue.name}
          </span>
        )}
        {(venueAddress || cityState) && (
          <span
            data-testid="venue-location"
            className="text-sm text-muted-foreground"
          >
            {venueAddress && (
              <span data-testid="venue-address">{venueAddress}</span>
            )}
            {venueAddress && cityState && ', '}
            {cityState}
          </span>
        )}
      </div>

      <MiddotSegments
        segments={factSegments}
        data-testid="venue-facts"
        className="mt-1.5 font-mono text-sm tabular-nums text-muted-foreground"
      />

      <div className="mt-2 flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <BracketLink
          label="Directions ↗"
          href={mapsUrl}
          external
          ariaLabel={`Directions to ${venue.name} (opens Google Maps in a new tab)`}
        />
        {/* Follow routes are PLURAL path segments ("venues"); the notify
            vocabulary is SINGULAR ("venue"). Adjacent on purpose so the next
            editor sees both spellings are deliberate. */}
        <FollowButton
          entityType="venues"
          entityId={venue.id}
          variant="bracket"
          bracketLabel="Follow venue"
        />
        <NotifyMeButton
          entityType="venue"
          entityId={venue.id}
          entityName={venue.name}
          variant="bracket"
        />
        {venue.slug && (
          <Link
            href={`/venues/${venue.slug}`}
            className="text-sm text-muted-foreground hover:text-primary transition-colors"
          >
            More at {venue.name} &rarr;
          </Link>
        )}
      </div>
    </div>
  )
}
