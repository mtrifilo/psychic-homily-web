/**
 * dedupShowsByArtistVenueDateTime collapses literal-duplicate show
 * rows that share the same `(artist_id, venue_id, event_date+time)`
 * tuple. PSY-559.
 *
 * The dedup key MUST include time (full ISO event_date), not just the
 * date — matinee + evening sets at the same venue on the same day are
 * NOT duplicates and must remain visible.
 *
 * Lowest-ID wins. Used as a render-time stopgap on artist + venue
 * detail pages so the visual fix lands even if the backend dedup cmd
 * (cmd/dedup-shows) hasn't been run on the target environment yet.
 *
 * Generic over T to work with both ArtistShowResponse (has top-level
 * `venue.id`) and VenueShowResponse (no venue field — venue is
 * implicit since the list is already scoped to one venue, so we key
 * on the headliner artist instead).
 */

interface ShowLike {
  id: number
  event_date: string
}

interface ArtistInList {
  id: number
  is_headliner?: boolean | null
  set_type?: string
  position?: number
}

interface ShowWithVenueAndArtists extends ShowLike {
  venue?: { id: number } | null
  artists: ArtistInList[]
}

interface ShowWithArtists extends ShowLike {
  artists: ArtistInList[]
}

/**
 * Pick a stable headliner artist id for a show. Prefer
 * `set_type === 'headliner'`, then `is_headliner`, then position 0,
 * then the first artist. Returns null if the show has no artists.
 */
function headlinerArtistId(show: ShowWithArtists): number | null {
  if (!show.artists || show.artists.length === 0) {
    return null
  }
  const byHeadlinerSetType = show.artists.find(a => a.set_type === 'headliner')
  if (byHeadlinerSetType) return byHeadlinerSetType.id
  const byHeadlinerFlag = show.artists.find(a => a.is_headliner === true)
  if (byHeadlinerFlag) return byHeadlinerFlag.id
  const byPosition = show.artists.find(a => a.position === 0)
  if (byPosition) return byPosition.id
  return show.artists[0].id
}

/**
 * Dedup shows on an ARTIST detail page. Each show carries its own
 * `venue.id`, so the key is `(venue.id, event_date)`. Lowest show.id
 * wins on collision.
 */
export function dedupArtistShows<T extends ShowWithVenueAndArtists>(shows: T[]): T[] {
  const seen = new Map<string, T>()
  for (const show of shows) {
    const venueId = show.venue?.id ?? 0
    const key = `${venueId}|${show.event_date}`
    const existing = seen.get(key)
    if (!existing || show.id < existing.id) {
      seen.set(key, show)
    }
  }
  // Preserve input order: walk the original list and only keep entries
  // whose id matches the winner for their key. (Map iteration order is
  // insertion order, but a later collision can replace an earlier
  // winner — that would shuffle the ordering relative to the API
  // response. Filter from the original instead.)
  const winners = new Set<number>()
  for (const v of seen.values()) winners.add(v.id)
  return shows.filter(s => winners.has(s.id))
}

/**
 * Dedup shows on a VENUE detail page. The list is already scoped to
 * ONE venue, so the key is `(headliner_artist_id, event_date)`.
 * Shows without artists keep their own bucket (key uses 0).
 */
export function dedupVenueShows<T extends ShowWithArtists>(shows: T[]): T[] {
  const seen = new Map<string, T>()
  for (const show of shows) {
    const artistId = headlinerArtistId(show) ?? 0
    const key = `${artistId}|${show.event_date}`
    const existing = seen.get(key)
    if (!existing || show.id < existing.id) {
      seen.set(key, show)
    }
  }
  const winners = new Set<number>()
  for (const v of seen.values()) winners.add(v.id)
  return shows.filter(s => winners.has(s.id))
}
