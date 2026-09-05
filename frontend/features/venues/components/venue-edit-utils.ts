import { isUnitedStatesCountry } from '@/lib/formatLocation'
import { isValidTimeZone } from '@/lib/utils/formatters'
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
 * Whether this venue may be saved without naming a state.
 *
 * `state` on a venue is a US-map concern: the app reads it to name a timezone
 * for a venue that has no resolved `timezone` of its own. A venue with no state
 * on file whose own IANA zone is nameable, or that names a country the state
 * map does not describe, is a complete record, and the editor must not demand
 * an invented state before it will save an address fix.
 *
 * Only a venue whose STORED state is EXACTLY blank qualifies. Clearing a state
 * that is on file keeps the requirement, because `PUT /venues/{id}` answers 422
 * to `state: ""` and `detectVenueChanges` sends the field exactly when it
 * changed: exempting a clear would trade an inline message for a server error.
 * A blank state that was blank on arrival is never sent at all. The test is
 * `!== ''` rather than a trim for that reason: a whitespace state is a value
 * the form can leave alone, and treating it as blank would make clearing it
 * legal here and rejected there.
 *
 * The country half is a weaker signal than the zone half. It says the state map
 * does not apply, not which zone does; such a venue still renders on
 * FALLBACK_SHOW_TIMEZONE. That is unchanged by this rule, which governs whether
 * the venue RECORD can be edited, not how its shows are read. `country` is free
 * text and `isUnitedStatesCountry` recognizes two spellings, so a US venue
 * recorded as "United States" reads as non-US here and has its state waived.
 *
 * `isValidTimeZone` rather than `isShowTimezoneResolved`: the state is already
 * known blank, so the only live question is whether the venue's own zone STRING
 * names a zone, which is the distinction `formatters.ts` exports
 * `isValidTimeZone` to answer.
 *
 * KNOWN GAP: `venues.timezone` is DERIVED from the same location this venue is
 * missing part of (`applyGeocoding` in the venue service), so for a US town
 * entered without its state the zone can be a same-name city abroad. This rule
 * reads that zone as evidence and stops prompting for the state, which is the
 * one edit that would re-derive it. The zone is already wrong for such a venue
 * before anyone opens this form; what is lost is an accidental repair, not a
 * correct value.
 */
export function venueMayOmitState(
  venue: Pick<Venue, 'state' | 'timezone' | 'country'>
): boolean {
  if (venue.state !== '') return false
  if (venue.timezone && isValidTimeZone(venue.timezone)) return true
  const country = venue.country?.trim() ?? ''
  return country !== '' && !isUnitedStatesCountry(country)
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
