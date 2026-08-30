'use client'

import Link from 'next/link'
import { FollowButton } from '@/components/shared/FollowButton'
import { BracketLink } from '@/components/shared/BracketLink'
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
  // VERIFIED venues only. The venue's own page refuses to map an unverified
  // venue at all (city/state text, no directions, no embed) because a
  // name + city map search narrows a house show to a door — server-side
  // address redaction is only half that policy, and this affordance is the
  // other half. Two surfaces, one rule. Also requires something to actually
  // search for: blank venue names are a modeled case in this codebase, and
  // a [Directions] bracket pointing at an empty maps query is a dead verb.
  const hasLocatableQuery = Boolean(
    venue.name.trim() || venueAddress || cityState
  )
  const mapsUrl =
    venue.verified && hasLocatableQuery
      ? googleMapsSearchUrl({
          name: venue.name,
          address: venueAddress,
          city: venue.city,
          state: venue.state,
        })
      : null

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
        {mapsUrl && (
          <BracketLink
            label="Directions ↗"
            href={mapsUrl}
            external
            // Names both the venue and the destination app: "[Directions ↗]"
            // alone is ambiguous out of context, and which app it hands you to
            // is something BracketLink's generic suffix cannot say.
            ariaLabel={`Directions to ${venue.name} on Google Maps`}
          />
        )}
        {/* One bracket, not two. This module used to pair [Follow venue] with
            [Notify me], which did different things: the first wrote a bookmark
            row that delivered nothing, the second created the filter that
            actually subscribed. Following a venue now IS subscribing
            (PSY-1893), so the second bracket is gone (PSY-1905). The alert
            on/off axis lives on the venue's own page, not in a show's sidebar
            module. */}
        <FollowButton
          entityType="venues"
          entityId={venue.id}
          variant="bracket"
          bracketLabel="Follow venue"
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
