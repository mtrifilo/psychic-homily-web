/**
 * Google Maps URL builders, shared by every surface that points a reader at a
 * venue on a map (the venue page's location card, the show page's
 * [Directions] affordance).
 *
 * The query is the venue's NAME plus whatever address parts exist, so it
 * degrades honestly: an unverified venue whose street address is redacted
 * still resolves to a name + city search rather than a broken pin.
 */

export interface MapsVenueQuery {
  name: string
  address?: string | null
  city: string
  state: string
  zipcode?: string | null
}

function mapQuery(venue: MapsVenueQuery): string {
  // Every part is filtered: city and state are non-nullable in the type but
  // arrive over the wire and can be empty, and "The Venue, Berlin, " is a
  // worse search query than "The Venue, Berlin".
  return [
    venue.name,
    venue.address,
    venue.city,
    venue.state,
    venue.zipcode,
  ]
    .map(part => part?.trim())
    .filter(Boolean)
    .join(', ')
}

/** Search URL for a normal browser tab. */
export function googleMapsSearchUrl(venue: MapsVenueQuery): string {
  const query = encodeURIComponent(mapQuery(venue))
  return `https://www.google.com/maps/search/?api=1&query=${query}`
}

/** Legacy embed-format URL that works without an API key. */
export function googleMapsEmbedUrl(venue: MapsVenueQuery): string {
  const query = encodeURIComponent(mapQuery(venue))
  return `https://maps.google.com/maps?q=${query}&t=&z=15&ie=UTF8&iwloc=&output=embed`
}
