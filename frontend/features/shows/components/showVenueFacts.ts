import { doorsMusicFactSegment } from './showStatusStripeCopy'
import { governingAgeRequirement } from '../showAge'
import { showTimingInput } from '../utils'
import type { ShowResponse, VenueResponse } from '../types'

/**
 * The age fragment of the venue facts line, or null when nothing is known.
 *
 * The show's `age_requirement` is the PER-EVENT OVERRIDE and the venue's
 * `age_policy` is the HOUSE DEFAULT. When both exist and disagree,
 * the line says so in the mock's register — `17+ (event override; house
 * default all ages)` — because a reader who knows the room is all-ages would
 * otherwise read a bare `17+` as a data error. When they agree, or only one
 * exists, one value is the whole statement.
 */
function ageFactSegment(
  show: ShowResponse,
  venue: VenueResponse
): string | null {
  const override = show.age_requirement?.trim()
  const house = venue.age_policy?.trim()
  // WHICH value governs is `governingAgeRequirement`, shared with the show
  // page's discovery rails so the two cannot name different door policies for
  // one night. What follows is this line's own REGISTER: it has room to name a
  // disagreement between the halves, which a ledger cell does not.
  const governing = governingAgeRequirement(override, house)
  if (!governing) return null
  if (!override || !house || override.toLowerCase() === house.toLowerCase()) {
    return governing
  }
  return `${governing} (event override; house default ${house})`
}

/**
 * The facts a reader plans around, as ordered segments for one
 * middot-separated line: `CAP ~3,600 · 17+ · DOORS 7PM / MUSIC 8PM`.
 *
 * Every segment degrades to omission (locked mock: no placeholders), and the
 * caller renders nothing when no segment survives. Capacity is prefixed `~`
 * because it is community-backfilled (a locked decision) — a claim of
 * roughly, not exactly. The doors/music segment reuses the status stripe's
 * copy rules — see {@link doorsMusicFactSegment} for why it can be null even
 * when times exist.
 */
export function venueFactSegments(
  show: ShowResponse,
  venue: VenueResponse
): string[] {
  const segments: string[] = []
  if (venue.capacity != null && venue.capacity > 0) {
    segments.push(`CAP ~${venue.capacity.toLocaleString('en-US')}`)
  }
  const age = ageFactSegment(show, venue)
  if (age) {
    segments.push(age)
  }
  const timing = showTimingInput(show)
  const times = doorsMusicFactSegment({
    doorsAt: show.doors_at,
    musicAt: show.music_at,
    state: timing.state,
    timezone: timing.timezone,
  })
  if (times) {
    segments.push(times)
  }
  return segments
}
