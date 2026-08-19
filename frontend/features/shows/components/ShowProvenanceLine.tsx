'use client'

import { BracketLink, UserAttribution } from '@/components/shared'
import { useEntityAttribution } from '@/features/contributions'
import { ensureUTC } from '@/lib/ensureUTC'
import { MiddotSegments } from './MiddotSegments'
import { sourceVenueName } from './showFlyer'
import { ReportShowButton } from './ReportShowButton'
import type { ShowResponse } from '../types'

/**
 * "Jul 31", or "Jul 31, 2025" once the year is no longer the current one.
 * The mock's register is the short absolute date — a listing is a record,
 * and "2 weeks ago" ages out from under a record.
 *
 * Pinned to UTC on BOTH sides of the comparison, for determinism rather
 * than out of viewer courtesy: this string server-renders (Vercel runs UTC)
 * and re-renders on the viewer's machine, and a formatter left on the
 * ambient zone names different calendar days on the two passes — a
 * hydration mismatch plus a wrong date, invisible on a dev box where both
 * zones agree. Parsing goes through {@link ensureUTC} because a timestamp
 * that arrives without a zone suffix would otherwise be read as LOCAL time,
 * shifting the record's day by the viewer's offset.
 */
function shortDate(iso: string): string | null {
  const date = ensureUTC(iso)
  if (!Number.isFinite(date.getTime())) return null
  const formatted = date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    timeZone: 'UTC',
  })
  return date.getUTCFullYear() === new Date().getUTCFullYear()
    ? formatted
    : `${formatted}, ${date.getUTCFullYear()}`
}

interface ShowProvenanceLineProps {
  show: ShowResponse
  /** Display name for the show, threaded to the report dialog. */
  showTitle: string
  /**
   * Whether the viewer can open the edit drawer (admin OR submitter — shows
   * are direct-save only, with no suggest path to offer anyone else, so the
   * affordance is omitted rather than stubbed).
   */
  canEdit: boolean
  /** Opens the edit drawer. */
  onEdit: () => void
}

/**
 * The provenance footer of the show page, per the locked mock:
 * `Listing from Salt Shed calendar · added Jul 12 · updated Jul 31 by
 * mtrifilo · 3 edits · [Edit] · [Report issue]`.
 *
 * Every fragment degrades to omission:
 * - The listing credit resolves `source_venue` the same way the flyer
 *   caption does ({@link sourceVenueName}) and says nothing for
 *   user-submitted shows — the row carries no submitter display name, so
 *   the mock's "added by mtrifilo" has no honest data to render from.
 * - "updated … by …" and the edit count come from the revisions read the
 *   old attribution line already made on this page; zero revisions renders
 *   neither.
 * - `[Edit]` renders only when the drawer can actually open (the mock says
 *   "Suggest an edit", but shows have no suggest pipeline — user decision:
 *   honest label, no dead button).
 *
 * This supersedes `AttributionLine` on the show page only; the other five
 * detail pages keep it. Same deliberate asymmetry as the footer position
 * itself — see the ShowDetail comment above the footer.
 */
export function ShowProvenanceLine({
  show,
  showTitle,
  canEdit,
  onEdit,
}: ShowProvenanceLineProps) {
  const { data: attribution } = useEntityAttribution('show', show.id)

  const credit = sourceVenueName(show)
  const added = shortDate(show.created_at)
  const updated = attribution ? shortDate(attribution.created_at) : null

  // Parallel arrays of node + STABLE key, because the list's shape changes
  // at runtime: the revision fragments splice in when their async read
  // resolves, and a position-keyed list would remount everything after the
  // insertion point — including the report button, whose open dialog would
  // be thrown away mid-interaction.
  const fragments: React.ReactNode[] = []
  const fragmentKeys: string[] = []
  const push = (key: string, node: React.ReactNode) => {
    fragments.push(node)
    fragmentKeys.push(key)
  }

  if (credit) {
    push('credit', <span>Listing from {credit} calendar</span>)
  }
  if (added) {
    push('added', <span>added {added}</span>)
  }
  if (attribution && updated) {
    push(
      'updated',
      <span>
        updated {updated} by{' '}
        <UserAttribution
          name={attribution.user_name}
          username={attribution.user_username}
          className="hover:underline"
        />
      </span>
    )
    push(
      'edits',
      <span>
        {attribution.total} {attribution.total === 1 ? 'edit' : 'edits'}
      </span>
    )
  }
  if (canEdit) {
    // text-xs: BracketLink defaults to the header-linkbox text-sm, which
    // reads a size larger than the byline it sits in.
    push(
      'edit',
      <BracketLink label="Edit" onClick={onEdit} className="text-xs" />
    )
  }
  push(
    'report',
    <ReportShowButton
      showId={show.id}
      showTitle={showTitle}
      variant="bracket"
      className="text-xs"
    />
  )

  return (
    <MiddotSegments
      segments={fragments}
      keys={fragmentKeys}
      data-testid="show-provenance-line"
      className="mt-2 text-xs text-muted-foreground"
    />
  )
}
