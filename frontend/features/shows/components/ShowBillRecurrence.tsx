import { MiddotSegments } from './MiddotSegments'
import { billRecurrenceSegments } from './showTimelineCopy'
import type { RecurrenceBillArtist } from './showTimelineCopy'
import type { ShowTimelineRecurrence } from '../types'

export type { RecurrenceBillArtist }

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
 * SELF-HIDING when the archive has nothing to say. `billRecurrenceSegments`
 * owns which acts get a clause and whether the line is worth printing at all,
 * including the all-local rule; this component only renders what it returns.
 */
export function ShowBillRecurrence({
  recurrence,
  artists,
}: ShowBillRecurrenceProps) {
  const segments = billRecurrenceSegments(recurrence, artists)

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
