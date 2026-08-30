import { MiddotSegments } from './MiddotSegments'
import { lastPlayedLabel } from './showTimelineCopy'
import type { ShowTimelineRecurrence } from '../types'

/** The bill fields this line needs: a name to say, keyed by id. */
export interface RecurrenceBillArtist {
  id: number
  name: string
}

export interface ShowBillRecurrenceProps {
  recurrence: ShowTimelineRecurrence[]
  /** The bill as rendered above, the source of every name on this line. */
  artists: RecurrenceBillArtist[]
}

/**
 * What this place already knows about tonight's bill, as one line under it:
 * `Modest Mouse last played Chicago: Nov 2023, Aragon Ballroom · Califone:
 * hometown show`.
 *
 * SELF-HIDING when the archive has nothing to say. An act with no history here
 * and no claim on the place contributes no segment, and a bill where that is
 * true of everyone renders nothing at all.
 *
 * HOMETOWN WINS over a prior date. "Califone last played Chicago" is true of a
 * Chicago band and says nothing: every local band last played their own city.
 * The interesting fact is that they live here, so that is the one stated.
 *
 * The city named is the one on the ENTRY, never the show's own. An entry can
 * name a neighbouring city, so naming the show's city would print "last played
 * Chicago" against a venue that is not in Chicago.
 *
 * Names come from the BILL, not from the payload, so this line and the heading
 * above it cannot disagree about what an act is called. An entry naming an act
 * that is not on the bill is dropped rather than rendered nameless.
 */
export function ShowBillRecurrence({
  recurrence,
  artists,
}: ShowBillRecurrenceProps) {
  const nameById = new Map(artists.map(artist => [artist.id, artist.name]))

  const segments = recurrence.flatMap(entry => {
    const name = nameById.get(entry.artist_id)
    if (!name) return []
    if (entry.is_hometown) {
      return [{ id: entry.artist_id, text: `${name}: hometown show` }]
    }
    if (!entry.last_played) return []
    // A room with no city on record leaves the sentence no place to name, so it
    // states what it still has: when, and which room. The colon goes with the
    // city, because a colon introduces the place clause and reads as a
    // rendering fault standing on its own.
    const city = entry.last_played.city?.trim()
    const when = lastPlayedLabel(entry.last_played)
    return [
      {
        id: entry.artist_id,
        text: city
          ? `${name} last played ${city}: ${when}`
          : `${name} last played ${when}`,
      },
    ]
  })

  // MiddotSegments owns the separator, including the spaces-outside-the-hidden
  // -span rule that keeps two facts from running together when read aloud. It
  // self-hides on an empty list, which is this module's empty state.
  return (
    <MiddotSegments
      data-testid="show-bill-recurrence"
      className="mt-2 text-sm text-muted-foreground"
      segments={segments.map(segment => segment.text)}
      keys={segments.map(segment => String(segment.id))}
    />
  )
}
