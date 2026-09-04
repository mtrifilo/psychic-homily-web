import { isShowTimezoneResolved } from '@/lib/utils/formatters'
import type { Venue, VenueWithShowCount, VenueEditRequest } from '../types'

export interface VenueEditFormValues {
  name: string
  address: string
  city: string
  state: string
  zipcode: string
  instagram: string
  facebook: string
  twitter: string
  youtube: string
  spotify: string
  soundcloud: string
  bandcamp: string
  website: string
}

/**
 * Spellings of the United States that `venues.country` holds. The column is
 * free text written by several sources and is not canonicalized, so the set is
 * matched case-insensitively after trimming. Anything else non-empty names a
 * country that is not the one the state map describes.
 */
const US_COUNTRY_NAMES = new Set([
  'US',
  'U.S.',
  'U.S.A.',
  'USA',
  'UNITED STATES',
  'UNITED STATES OF AMERICA',
  'AMERICA',
])

/**
 * Whether this venue may be saved without naming a state.
 *
 * `state` on a venue is a US-map concern: the app reads it to name a timezone
 * for a venue that has no resolved `timezone` of its own. A venue with no state
 * on file whose zone is nameable anyway, or that sits in a country the state
 * map does not describe, is a complete record, and the editor must not demand
 * an invented state before it will save an address fix.
 *
 * Only a venue whose STORED state is already blank qualifies. Clearing a state
 * that is on file keeps the requirement, because `PUT /venues/{id}` answers 422
 * to `state: ""` and `detectVenueChanges` sends the field exactly when it
 * changed: exempting a clear would trade an inline message for a server error.
 * A blank state that was blank on arrival is never sent at all.
 *
 * The country half is a weaker signal than the timezone half. It says the state
 * map does not apply, not which zone does; a venue with `country` set and no
 * `timezone` still renders on FALLBACK_SHOW_TIMEZONE. That is unchanged by this
 * rule, which governs whether the venue record can be edited, not how its shows
 * are read.
 */
export function venueMayOmitState(
  venue: Pick<Venue, 'state' | 'timezone' | 'country'>
): boolean {
  if (venue.state.trim() !== '') return false
  if (isShowTimezoneResolved(venue.state, venue.timezone)) return true
  const country = venue.country?.trim() ?? ''
  return country !== '' && !US_COUNTRY_NAMES.has(country.toUpperCase())
}

/**
 * Build a VenueEditRequest containing only the fields that changed.
 * Returns null if no changes were detected.
 */
export function detectVenueChanges(
  value: VenueEditFormValues,
  venue: VenueWithShowCount | Venue
): VenueEditRequest | null {
  const changes: VenueEditRequest = {}

  if (value.name !== venue.name) changes.name = value.name
  if (value.address !== (venue.address || ''))
    changes.address = value.address || undefined
  if (value.city !== venue.city) changes.city = value.city
  if (value.state !== venue.state) changes.state = value.state
  if (value.zipcode !== (venue.zipcode || ''))
    changes.zipcode = value.zipcode || undefined
  if (value.instagram !== (venue.social?.instagram || ''))
    changes.instagram = value.instagram || undefined
  if (value.facebook !== (venue.social?.facebook || ''))
    changes.facebook = value.facebook || undefined
  if (value.twitter !== (venue.social?.twitter || ''))
    changes.twitter = value.twitter || undefined
  if (value.youtube !== (venue.social?.youtube || ''))
    changes.youtube = value.youtube || undefined
  if (value.spotify !== (venue.social?.spotify || ''))
    changes.spotify = value.spotify || undefined
  if (value.soundcloud !== (venue.social?.soundcloud || ''))
    changes.soundcloud = value.soundcloud || undefined
  if (value.bandcamp !== (venue.social?.bandcamp || ''))
    changes.bandcamp = value.bandcamp || undefined
  if (value.website !== (venue.social?.website || ''))
    changes.website = value.website || undefined

  return Object.keys(changes).length > 0 ? changes : null
}
