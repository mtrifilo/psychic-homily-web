import { Fragment } from 'react'
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
  /**
   * The show's own city, as the place these facts are about. Empty for a
   * venue-less show, which drops the "last played {city}" phrasing.
   */
  city: string
}

/**
 * What this city already knows about tonight's bill, as one line under it:
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
 * Names come from the BILL, not from the payload, so this line and the heading
 * above it cannot disagree about what an act is called. An entry naming an act
 * that is not on the bill is dropped rather than rendered nameless.
 */
export function ShowBillRecurrence({
  recurrence,
  artists,
  city,
}: ShowBillRecurrenceProps) {
  const nameById = new Map(artists.map(artist => [artist.id, artist.name]))

  const segments = recurrence.flatMap(entry => {
    const name = nameById.get(entry.artist_id)
    if (!name) return []
    if (entry.is_hometown) {
      return [{ id: entry.artist_id, text: `${name}: hometown show` }]
    }
    if (!entry.last_played) return []
    // Without a city the sentence has no place to name, so it states the fact
    // it still has: when and where they were last seen.
    const where = city ? ` ${city}` : ''
    return [
      {
        id: entry.artist_id,
        text: `${name} last played${where}: ${lastPlayedLabel(entry.last_played)}`,
      },
    ]
  })

  if (segments.length === 0) return null

  return (
    <p
      data-testid="show-bill-recurrence"
      className="mt-2 text-sm text-muted-foreground"
    >
      {segments.map((segment, index) => (
        <Fragment key={segment.id}>
          {/* The glyph is decoration and is hidden; the spaces around it are
              real text nodes OUTSIDE the hidden span, so the two facts stay
              separated when the line is read aloud rather than running
              together as one sentence. */}
          {index > 0 && (
            <>
              {' '}
              <span aria-hidden="true" className="text-muted-foreground/60">
                &middot;
              </span>{' '}
            </>
          )}
          {segment.text}
        </Fragment>
      ))}
    </p>
  )
}
