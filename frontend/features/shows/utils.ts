/**
 * Render-time dedup helpers for show lists shown on artist + venue
 * detail pages (PSY-559).
 *
 * The dedup key MUST include time (full ISO event_date), not just
 * the date — matinee + evening sets at the same venue on the same
 * day are NOT duplicates and must remain visible.
 *
 * Lowest-ID wins. Used as a stopgap so the visual fix lands even if
 * the backend dedup cmd (`cmd/dedup-shows`) hasn't been run on the
 * target environment yet.
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

// Pick a stable headliner artist id for a show. Prefer set_type, then
// is_headliner, then position 0, then the first artist. Returns null
// when the show has no artists.
function headlinerArtistId(show: ShowWithArtists): number | null {
  if (!show.artists || show.artists.length === 0) {
    return null
  }
  return (
    show.artists.find(a => a.set_type === 'headliner')?.id ??
    show.artists.find(a => a.is_headliner === true)?.id ??
    show.artists.find(a => a.position === 0)?.id ??
    show.artists[0].id
  )
}

// Filter input to one row per dedup key, preserving input order.
// Among collisions on the same key, the show with the lowest id wins.
//
// Map iteration order alone is not enough: a later row can replace an
// earlier winner inside the Map and that mutation would shuffle the
// rendered order relative to the API response. We collect winner ids
// into a Set and re-filter the original array instead.
function pickWinners<T extends ShowLike>(
  shows: T[],
  keyFor: (show: T) => string,
): T[] {
  const winnersByKey = new Map<string, T>()
  for (const show of shows) {
    const key = keyFor(show)
    const existing = winnersByKey.get(key)
    if (!existing || show.id < existing.id) {
      winnersByKey.set(key, show)
    }
  }
  const winnerIds = new Set<number>()
  for (const v of winnersByKey.values()) winnerIds.add(v.id)
  return shows.filter(s => winnerIds.has(s.id))
}

/**
 * Dedup shows on an ARTIST detail page. Each show carries its own
 * `venue.id`, so the key is `(venue.id, event_date)`.
 */
export function dedupArtistShows<T extends ShowWithVenueAndArtists>(shows: T[]): T[] {
  return pickWinners(shows, show => `${show.venue?.id ?? 0}|${show.event_date}`)
}

/**
 * Dedup shows on a VENUE detail page. The list is already scoped to
 * ONE venue, so the key is `(headliner_artist_id, event_date)`.
 * Shows without artists keep their own bucket (key uses 0).
 */
export function dedupVenueShows<T extends ShowWithArtists>(shows: T[]): T[] {
  return pickWinners(shows, show => `${headlinerArtistId(show) ?? 0}|${show.event_date}`)
}
