import type { Venue, VenueWithShowCount, VenueEditRequest } from '@/lib/types/venue'

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
